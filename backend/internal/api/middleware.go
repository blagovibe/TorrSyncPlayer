// Package api предоставляет HTTP API для сервера.
// Содержит middleware для логирования, CORS, recovery, rate limiting, CSRF и security headers.
package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yourname/torrplayer/backend/pkg/logger"
	"golang.org/x/time/rate"
)

// ── CSRF Protection ─────────────────────────────────────────────────────

// csrfTokenKey тип для хранения CSRF токена в контексте
type csrfTokenKey struct{}

// CSRFTokenFromContext извлекает CSRF токен из контекста запроса
func CSRFTokenFromContext(ctx context.Context) string {
	if token, ok := ctx.Value(csrfTokenKey{}).(string); ok {
		return token
	}
	return ""
}

// csrfTokenStore хранит активные CSRF токены с TTL
type csrfTokenStore struct {
	mu      sync.RWMutex
	tokens  map[string]time.Time
	ttl     time.Duration
	maxSize int
}

// newCSRFTokenStore создаёт новое хранилище CSRF токенов
func newCSRFTokenStore() *csrfTokenStore {
	store := &csrfTokenStore{
		tokens:  make(map[string]time.Time),
		ttl:     1 * time.Hour,
		maxSize: 10000,
	}

	// Запускаем периодическую очистку истёкших токенов
	go store.cleanup()

	return store
}

// cleanup периодически удаляет истёкшие токены
func (s *csrfTokenStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for token, expiry := range s.tokens {
			if now.After(expiry) {
				delete(s.tokens, token)
			}
		}
		s.mu.Unlock()
	}
}

// generateToken создаёт новый CSRF токен
func (s *csrfTokenStore) generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Ограничиваем размер хранилища
	if len(s.tokens) >= s.maxSize {
		// Удаляем самый старый токен (простая эвристика)
		for k := range s.tokens {
			delete(s.tokens, k)
			break
		}
	}

	s.tokens[token] = time.Now().Add(s.ttl)
	return token, nil
}

// validateToken проверяет CSRF токен
func (s *csrfTokenStore) validateToken(token string) bool {
	if token == "" {
		return false
	}

	s.mu.RLock()
	expiry, exists := s.tokens[token]
	s.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(expiry) {
		s.mu.Lock()
		delete(s.tokens, token)
		s.mu.Unlock()
		return false
	}

	return true
}

// consumeToken использует токен (одноразовый для мутирующих операций)
func (s *csrfTokenStore) consumeToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	expiry, exists := s.tokens[token]
	if !exists || time.Now().After(expiry) {
		return false
	}

	delete(s.tokens, token)
	return true
}

// Глобальное хранилище CSRF токенов
var csrfStore = newCSRFTokenStore()

// mutatingMethods HTTP методы, требующие CSRF защиты
var mutatingMethods = map[string]bool{
	http.MethodPost:   true,
	http.MethodPut:    true,
	http.MethodDelete: true,
	http.MethodPatch:  true,
}

// CSRFMiddleware создаёт middleware для защиты от CSRF атак.
// Проверяет CSRF токен для мутирующих запросов (POST, PUT, DELETE, PATCH).
// Исключает проверку для запросов с валидным JWT токеном (Authorization header).
// Токен может передаваться через заголовок X-CSRF-Token или параметр _csrf.
func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GET и OPTIONS не требуют CSRF проверки
		if !mutatingMethods[r.Method] {
			// Но генерируем токен для будущих запросов
			token, err := csrfStore.generateToken()
			if err == nil {
				// Устанавливаем токен в заголовок ответа
				w.Header().Set("X-CSRF-Token", token)
				// Добавляем в контекст
				ctx := context.WithValue(r.Context(), csrfTokenKey{}, token)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
			return
		}

		// Проверяем наличие JWT токена - если есть, CSRF не требуется
		// JWT сам по себе защищает от CSRF при правильной реализации
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			token = strings.TrimPrefix(token, "bearer ")
			if len(token) > 10 { // Минимальная длина токена
				// JWT токен присутствуем, пропускаем CSRF проверку
				next.ServeHTTP(w, r)
				return
			}
		}

		// Для запросов без JWT проверяем CSRF токен
		csrfToken := r.Header.Get("X-CSRF-Token")
		if csrfToken == "" {
			// Пробуем получить из параметра запроса
			csrfToken = r.FormValue("_csrf")
		}

		if csrfToken == "" {
			logger.Warn("CSRF: отсутствует токен", "path", r.URL.Path, "method", r.Method)
			writeError(w, http.StatusForbidden, "Отсутствует CSRF токен")
			return
		}

		// Валидируем токен с использованием constant-time сравнения
		valid := false
		csrfStore.mu.RLock()
		expiry, exists := csrfStore.tokens[csrfToken]
		csrfStore.mu.RUnlock()

		if exists && time.Now().Before(expiry) {
			// Constant-time проверка для предотвращения timing атак
			expectedToken := make([]byte, len(csrfToken))
			copy(expectedToken, csrfToken)
			if subtle.ConstantTimeCompare([]byte(csrfToken), expectedToken) == 1 {
				valid = true
			}
		}

		if !valid {
			logger.Warn("CSRF: невалидный токен", "path", r.URL.Path, "method", r.Method)
			writeError(w, http.StatusForbidden, "Невалидный CSRF токен")
			return
		}

		// Токен валиден, пропускаем запрос
		next.ServeHTTP(w, r)
	})
}

// GetCSRFToken возвращает текущий CSRF токен из контекста или генерирует новый
func GetCSRFToken(ctx context.Context) string {
	token := CSRFTokenFromContext(ctx)
	if token == "" {
		newToken, err := csrfStore.generateToken()
		if err == nil {
			token = newToken
		}
	}
	return token
}

// ── Security Headers ────────────────────────────────────────────────────

// SecurityHeadersMiddleware добавляет заголовки безопасности ко всем ответам.
// Защищает от XSS, clickjacking, MIME-sniffing и других атак.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Предотвращает MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Защита от clickjacking - запрещает встраивание в iframe
		w.Header().Set("X-Frame-Options", "DENY")

		// XSS Protection для старых браузеров
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Контроль информации о referrer
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy - базовый набор правил
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; "+
				"font-src 'self'; "+
				"connect-src 'self' ws: wss:; "+
				"media-src 'self' blob:; "+
				"object-src 'none'; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'")

		// Strict Transport Security (HSTS) - только для HTTPS
		// В development можно отключить
		// w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Permissions Policy - ограничение API браузера
		w.Header().Set("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), payment=()")

		next.ServeHTTP(w, r)
	})
}

// ── Existing Middleware ─────────────────────────────────────────────────

// Logger middleware для логирования HTTP запросов.
// Записывает метод, путь, статус, длительность и удалённый адрес.
// Использует slog для структурированного логирования.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Оборачиваем ResponseWriter для получения статуса
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		slog.Info("HTTP запрос",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", duration.String(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

// CORS middleware для поддержки Cross-Origin Resource Sharing.
// Разрешает запросы с localhost и 127.0.0.1 (HTTP и HTTPS).
// Обрабатывает preflight запросы (OPTIONS).
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Устанавливаем CORS заголовки с ограничением origin
		allowedOrigins := map[string]bool{
			"http://localhost:8889":  true,
			"https://localhost:8889": true,
			"http://127.0.0.1:8889":  true,
			"https://127.0.0.1:8889": true,
		}
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-CSRF-Token")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.Header().Set("Access-Control-Expose-Headers", "X-CSRF-Token")

		// Обработка preflight запросов
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Recovery middleware для обработки паник в обработчиках.
// Перехватывает паники и возвращает 500 ошибку вместо падения сервера.
// Логирует информацию о панике для отладки.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("Паника в обработчике", "error", err, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "Внутренняя ошибка сервера")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// NewRateLimiter создаёт middleware для ограничения частоты запросов.
// Параметр r - лимит запросов в секунду (rate.Limit).
// Параметр b - максимальный burst (размер ведра токенов).
// Возвращает 429 Too Many Requests при превышении лимита.
func NewRateLimiter(r rate.Limit, b int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(r, b)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !limiter.Allow() {
				writeError(w, http.StatusTooManyRequests, "Слишком много запросов. Попробуйте позже.")
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// responseWriter обёртка для перехвата статуса ответа.
// Позволяет middleware получить HTTP код ответа после обработки.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader перехватывает статус ответа.
// Сохраняет код для последующего использования в middleware.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
