// Package api provides HTTP API for the server.
// Contains middleware for logging, CORS, recovery, rate limiting and security headers.
package api

import (
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// ── Content-Type Validation ──────────────────────────────────────────────

func ContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GET/OPTIONS/HEAD never carry a JSON body. DELETE is treated as a
		// mutating method by CSRF but the client sends it without a JSON
		// Content-Type and without a body, so we exempt it from this check to
		// avoid rejecting legitimate DELETE requests.
		if r.Method == http.MethodGet || r.Method == http.MethodOptions || r.Method == http.MethodHead || r.Method == http.MethodDelete {
			next.ServeHTTP(w, r)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "" {
			mediaType, _, err := mime.ParseMediaType(contentType)
			if err != nil || mediaType != "application/json" {
				logger.Warn("Content-Type: unsupported type", "contentType", contentType, "path", r.URL.Path, "method", r.Method)
				WriteError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				return
			}
		} else {
			logger.Warn("Content-Type: unsupported type", "contentType", contentType, "path", r.URL.Path, "method", r.Method)
			WriteError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ── Security Headers ────────────────────────────────────────────────────

// SecurityHeadersMiddleware adds security headers to all responses.
// Protects against XSS, clickjacking, MIME-sniffing and other attacks.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Remove default Server header
		w.Header().Del("Server")

		// Prevents MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Clickjacking protection - prevents embedding in iframe
		w.Header().Set("X-Frame-Options", "DENY")

		// XSS Protection for older browsers
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Referrer information control
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy - basic rule set
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self'; "+
				"img-src 'self' data:; "+
				"font-src 'self'; "+
				"connect-src 'self' ws: wss:; "+
				"media-src 'self' blob:; "+
				"object-src 'none'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'")

		// Strict Transport Security (HSTS) — only over TLS
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", constants.HSTSMaxAge)
		}

		// Permissions Policy - limit browser APIs
		w.Header().Set("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), payment=(), usb=(), magnetometer=(), gyroscope=(), midi=(), sync-xhr=()")

		next.ServeHTTP(w, r)
	})
}

// ── Existing Middleware ─────────────────────────────────────────────────

// Logger middleware for logging HTTP requests.
// Records method, path, status, duration and remote address.
// Uses slog for structured logging.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		metrics.GetInstance().RequestStarted()

		// Wrap ResponseWriter to capture status
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		if wrapped.statusCode >= 400 {
			metrics.GetInstance().RequestError()
		} else {
			metrics.GetInstance().RequestSuccess()
		}

		slog.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", duration.String(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

var (
	corsOriginsMu     sync.RWMutex
	cachedCORSOrigins map[string]bool
	corsLastLoad      time.Time
	corsTTL           = 5 * time.Minute
)

// getCORSOrigins returns the list of allowed CORS origins.
// Reads from the CORS_ORIGINS environment variable (comma-separated).
// Reloads periodically to allow runtime configuration changes.
func getCORSOrigins() map[string]bool {
	corsOriginsMu.RLock()
	cached := cachedCORSOrigins
	lastLoad := corsLastLoad
	corsOriginsMu.RUnlock()

	if cached != nil && time.Since(lastLoad) < corsTTL {
		return cached
	}

	corsOriginsMu.Lock()
	defer corsOriginsMu.Unlock()

	if time.Since(corsLastLoad) < corsTTL {
		return cachedCORSOrigins
	}

	cachedCORSOrigins = loadCORSOrigins()
	corsLastLoad = time.Now()
	return cachedCORSOrigins
}

func loadCORSOrigins() map[string]bool {
	origins := make(map[string]bool)

	if corsOrigins := os.Getenv("CORS_ORIGINS"); corsOrigins != "" {
		for _, origin := range strings.Split(corsOrigins, ",") {
			origin = normalizeOrigin(origin)
			if origin != "" {
				origins[origin] = true
			}
		}
		return origins
	}

	if os.Getenv("ENV") == "production" || os.Getenv("GO_ENV") == "production" {
		logger.Warn("CORS_ORIGINS not set in production — set the environment variable for cross-domain requests")
	}

	origins["http://localhost:8889"] = true
	origins["https://localhost:8889"] = true
	origins["http://127.0.0.1:8889"] = true
	origins["https://127.0.0.1:8889"] = true

	return origins
}

// normalizeOrigin normalizes origin for CORS validation.
// Removes trailing slash and converts to lowercase.
func normalizeOrigin(origin string) string {
	origin = strings.TrimSpace(origin)
	origin = strings.TrimSuffix(origin, "/")
	return strings.ToLower(origin)
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowedOrigins := getCORSOrigins()

		origin := r.Header.Get("Origin")
		normalizedOrigin := normalizeOrigin(origin)

		w.Header().Set("Vary", "Origin")

		if allowedOrigins[normalizedOrigin] {
			w.Header().Set("Access-Control-Allow-Origin", normalizedOrigin)
		}
		w.Header().Set("Access-Control-Allow-Methods", constants.CORSAllowMethods)
		w.Header().Set("Access-Control-Allow-Headers", constants.CORSAllowHeaders)
		w.Header().Set("Access-Control-Max-Age", constants.CORSMaxAge)
		w.Header().Set("Access-Control-Expose-Headers", constants.CORSExposeHeaders)
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Recovery middleware for handling panics in handlers.
// Catches panics and returns a 500 error instead of crashing the server.
// Logs the panic information for debugging.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("Panic in handler", "error", err, "path", r.URL.Path)
				WriteError(w, http.StatusInternalServerError, "Internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// clientLimiterEntry stores a rate limiter and last usage time
type clientLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// clientRateLimiter stores a rate limiter for each IP
type clientRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*clientLimiterEntry
	rate     rate.Limit
	burst    int
	cleanup  time.Duration
	stopChan chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// newClientRateLimiter creates a per-IP rate limiter
func newClientRateLimiter(r rate.Limit, b int) *clientRateLimiter {
	cri := &clientRateLimiter{
		limiters: make(map[string]*clientLimiterEntry),
		rate:     r,
		burst:    b,
		cleanup:  10 * time.Minute,
		stopChan: make(chan struct{}),
	}
	cri.wg.Add(1)
	go cri.cleanupLoop()
	return cri
}

func (cri *clientRateLimiter) Stop() {
	cri.stopOnce.Do(func() {
		close(cri.stopChan)
	})
	cri.wg.Wait()
}

// cleanupLoop periodically removes old rate limiter entries
func (cri *clientRateLimiter) cleanupLoop() {
	defer cri.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Error("RateLimiter: cleanup goroutine exited with panic", "error", r)
		}
	}()
	ticker := time.NewTicker(cri.cleanup)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cri.mu.Lock()
			now := time.Now()
			for ip, entry := range cri.limiters {
				if now.Sub(entry.lastSeen) > cri.cleanup {
					delete(cri.limiters, ip)
				}
			}
			cri.mu.Unlock()
		case <-cri.stopChan:
			return
		}
	}
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	if isTrustedProxy(r.RemoteAddr) {
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}

		xri := r.Header.Get("X-Real-IP")
		if xri != "" {
			return xri
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

var (
	trustedCIDRsOnce   sync.Once
	cachedTrustedCIDRs []*net.IPNet
	hasTrustedCIDRs    bool
)

func loadTrustedCIDRs() {
	if trustedCIDRs := os.Getenv("TRUSTED_PROXIES"); trustedCIDRs != "" {
		for _, cidr := range strings.Split(trustedCIDRs, ",") {
			cidr = strings.TrimSpace(cidr)
			if cidr == "" {
				continue
			}
			_, ipNet, err := net.ParseCIDR(cidr)
			if err == nil {
				cachedTrustedCIDRs = append(cachedTrustedCIDRs, ipNet)
				hasTrustedCIDRs = true
			}
		}
	}
}

// isTrustedProxy checks if the request comes from a trusted proxy.
// Reads from TRUSTED_PROXIES env var (comma-separated CIDRs).
// If not set, defaults to localhost and private network ranges.
// CIDR networks are cached after first parse for performance.
func isTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	trustedCIDRsOnce.Do(loadTrustedCIDRs)
	if hasTrustedCIDRs {
		for _, ipNet := range cachedTrustedCIDRs {
			if ipNet.Contains(ip) {
				return true
			}
		}
		return false
	}

	return isPrivateIP(ip) || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}

var (
	privateNetsOnce sync.Once
	private10       *net.IPNet
	private172      *net.IPNet
	private192      *net.IPNet
	loopbackNet     *net.IPNet
	linkLocalNet    *net.IPNet
	uniqueLocal6    *net.IPNet
	linkLocal6      *net.IPNet
	loopback6       *net.IPNet
)

func initPrivateNets() {
	_, private10, _ = net.ParseCIDR("10.0.0.0/8")
	_, private172, _ = net.ParseCIDR("172.16.0.0/12")
	_, private192, _ = net.ParseCIDR("192.168.0.0/16")
	_, loopbackNet, _ = net.ParseCIDR("127.0.0.0/8")
	_, linkLocalNet, _ = net.ParseCIDR("169.254.0.0/16")
	_, uniqueLocal6, _ = net.ParseCIDR("fc00::/7")
	_, linkLocal6, _ = net.ParseCIDR("fe80::/10")
	_, loopback6, _ = net.ParseCIDR("::1/128")
}

func isPrivateIP(ip net.IP) bool {
	privateNetsOnce.Do(initPrivateNets)
	if ip.To4() != nil {
		return private10.Contains(ip) || private172.Contains(ip) ||
			private192.Contains(ip) || loopbackNet.Contains(ip) || linkLocalNet.Contains(ip)
	}
	return uniqueLocal6.Contains(ip) || linkLocal6.Contains(ip) || loopback6.Contains(ip)
}

// getLimiter returns the rate limiter for the specified IP.
// Thread-safe with proper double-checked locking pattern.
func (cri *clientRateLimiter) getLimiter(ip string) *rate.Limiter {
	// First check with read lock
	cri.mu.RLock()
	_, exists := cri.limiters[ip]
	cri.mu.RUnlock()

	if exists {
		// Entry exists - update lastSeen under write lock
		cri.mu.Lock()
		// Re-check after acquiring write lock (double-checked locking)
		if current, ok := cri.limiters[ip]; ok {
			current.lastSeen = time.Now()
			cri.mu.Unlock()
			return current.limiter
		}
		cri.mu.Unlock()
		// Entry was removed, fall through to create new one
	}

	// Create new limiter with write lock
	cri.mu.Lock()
	defer cri.mu.Unlock()

	// Check again in case another goroutine created it while we waited
	if entry, exists := cri.limiters[ip]; exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	limiter := rate.NewLimiter(cri.rate, cri.burst)
	cri.limiters[ip] = &clientLimiterEntry{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

// globalClientRateLimiter global per-IP rate limiter for all endpoints.
//
// The rate and burst come from constants.APIRateLimit / constants.APIRateBurst
// (1 req/s, burst 10 — 60 requests/minute per address) unless the process
// environment raises them. That override exists only for load testing, where a
// single generator address would otherwise be throttled on every request; it
// raises the ceiling instead of removing the limiter, so the same token-bucket
// code still runs. See constants.EnvAPIRateLimit for the details, including why
// the values are never read from a request.
var globalClientRateLimiter = newClientRateLimiter(resolveAPIRateLimit(), resolveAPIRateBurst())

// resolveAPIRateLimit returns the per-IP request rate for the protected API
// group, falling back to the shipped default on anything malformed.
func resolveAPIRateLimit() rate.Limit {
	return rate.Limit(envFloat(constants.EnvAPIRateLimit, constants.APIRateLimit))
}

// resolveAPIRateBurst returns the per-IP burst for the protected API group.
func resolveAPIRateBurst() int {
	return envInt(constants.EnvAPIRateBurst, constants.APIRateBurst)
}

// RateLimitIsOverridden reports whether the per-IP API limit differs from the
// shipped defaults. main logs a warning when this is true so that a process
// running with a relaxed limiter announces it rather than doing so silently.
func RateLimitIsOverridden() bool {
	return resolveAPIRateLimit() != rate.Limit(constants.APIRateLimit) ||
		resolveAPIRateBurst() != constants.APIRateBurst
}

// envFloat reads a positive float from the environment, falling back to
// fallback for unset, unparsable or non-positive values.
func envFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return fallback
	}
	return f
}

// envInt reads a positive int from the environment, falling back to fallback
// for unset, unparsable or non-positive values.
func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// StopGlobalRateLimiters stops all background cleanup goroutines for rate limiters.
// Call during graceful shutdown.
func StopGlobalRateLimiters() {
	globalClientRateLimiter.Stop()
}

// responseWriter wrapper for capturing the response status.
// Allows middleware to get the HTTP response code after processing.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the response status.
// Saves the code for later use in middleware.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// NewRateLimiter creates a middleware for rate limiting requests.
// Parameter r - rate limit per second (rate.Limit).
// Parameter b - maximum burst (token bucket size).
// Returns 429 Too Many Requests when the limit is exceeded.
func NewRateLimiter(r rate.Limit, b int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(r, b)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !limiter.Allow() {
				WriteError(w, http.StatusTooManyRequests, "Too many requests. Please try again later.")
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// resetAll drops every per-address bucket.
//
// Production never calls this — the limiter is deliberately process-wide so
// the limit applies across all routes. Tests do, because every test in this
// package shares one global limiter and one loopback address: without a reset
// the bucket spent by an early test silently throttles later ones, and the
// suite fails on 429s that have nothing to do with the code under test.
func (cri *clientRateLimiter) resetAll() {
	cri.mu.Lock()
	defer cri.mu.Unlock()
	cri.limiters = make(map[string]*clientLimiterEntry)
}

// ResetClientRateLimiter clears every per-IP bucket in the process-wide
// limiter. Exported for test harnesses outside this package, most notably the
// pact provider verifier, which replays recorded interactions in a tight
// loop from one address and would otherwise throttle itself part-way through.
// Production never calls this.
func ResetClientRateLimiter() {
	globalClientRateLimiter.resetAll()
}

// PerIPRateLimiter creates middleware for per-IP rate limiting.
// Uses the global per-IP rate limiter.
func PerIPRateLimiter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)
		limiter := globalClientRateLimiter.getLimiter(ip)
		if !limiter.Allow() {
			WriteError(w, http.StatusTooManyRequests, "Too many requests. Please try again later.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
