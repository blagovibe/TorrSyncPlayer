// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package constants contains common application constants.
// All magic numbers from various packages are placed here for maintainability.
package constants

import "time"

// ── Server Constants ──────────────────────────────────────────────────────

const (
	// ServerReadTimeout HTTP request read timeout
	ServerReadTimeout = 30 * time.Second

	// ServerWriteTimeout bounds the total duration of a response body.
	//
	// Do NOT apply this to the main API server. net/http treats WriteTimeout as
	// an absolute deadline for the entire response, not as a per-write idle
	// timer, so it truncates exactly the responses this application depends on:
	// /stream dies mid-film and the SSE room-events feed drops. The main server
	// therefore leaves WriteTimeout unset — see cmd/server/main.go.
	//
	// It is still correct for short-lived, non-streaming endpoints, and is used
	// by the localhost pprof server.
	ServerWriteTimeout = 30 * time.Second

	// ServerIdleTimeout HTTP connection idle timeout
	ServerIdleTimeout = 120 * time.Second
)

// ── CSRF Constants ────────────────────────────────────────────────────────

const (
	// CSRFTokenTTL CSRF token lifetime
	CSRFTokenTTL = 1 * time.Hour

	// CSRFTokenStoreMaxSize maximum number of CSRF tokens in the store
	CSRFTokenStoreMaxSize = 10000

	// CSRFCleanupInterval interval for cleaning expired CSRF tokens
	CSRFCleanupInterval = 5 * time.Minute

	// CSRFShutdownTimeout timeout for waiting on CSRF cleanup goroutine
	CSRFShutdownTimeout = 5 * time.Second

	// CSRFTokenBytes number of bytes for CSRF token generation
	CSRFTokenBytes = 32

	// UserIDBytes number of bytes for user ID generation
	UserIDBytes = 16
)

// ── CORS Constants ────────────────────────────────────────────────────────

const (
	// CORSMaxAge preflight request cache time (24 hours)
	CORSMaxAge = "86400"

	// CORSAllowMethods allowed HTTP methods for CORS
	CORSAllowMethods = "GET, POST, PUT, DELETE, OPTIONS"

	// CORSAllowHeaders allowed headers for CORS
	CORSAllowHeaders = "Content-Type, Authorization, X-Requested-With, X-CSRF-Token, X-Session-ID"

	// CORSExposeHeaders headers exposed to the client
	CORSExposeHeaders = "X-CSRF-Token, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset"

	// HSTSMaxAge HSTS header lifetime (1 year in seconds)
	HSTSMaxAge = "max-age=31536000; includeSubDomains"

	// CORSCacheTTL time-to-live for cached CORS origins
	CORSCacheTTL = 5 * time.Minute
)

// ── Rate Limiting Constants ───────────────────────────────────────────────

const (
	// AuthRateLimit request limit for auth endpoints (requests per second)
	AuthRateLimit = 0.17 // ~10 requests/minute

	// AuthRateBurst burst size for auth endpoints
	AuthRateBurst = 5

	// APIRateLimit is the default per-IP limit for the protected /api/v1 group,
	// in requests per second: 60 requests/minute per client address.
	APIRateLimit = 1

	// APIRateBurst is the default burst for the protected /api/v1 group.
	APIRateBurst = 10

	// EnvAPIRateLimit / EnvAPIRateBurst override the two values above.
	//
	// They exist for load testing only: a load generator drives thousands of
	// requests per second from a single address and would otherwise be answered
	// with 429 on essentially every call. They raise the limit rather than
	// disabling it, so the limiter itself stays in the request path and keeps
	// running the same code it runs in production.
	//
	// These are read from the process environment once, at startup. They are
	// never derived from a request header, query parameter or body — changing
	// them requires the same host access as deploying a build. Server startup
	// logs a warning whenever they differ from the defaults, so a production
	// process that was accidentally relaxed is visible in the logs.
	EnvAPIRateLimit = "API_RATE_LIMIT"
	EnvAPIRateBurst = "API_RATE_BURST"

	// ClientRateLimiterCleanup cleanup interval for per-IP rate limiter
	ClientRateLimiterCleanup = 10 * time.Minute

	// CSRFRateLimit rate limit for CSRF endpoint (requests per second)
	// Very strict: ~5 requests/minute to prevent token flooding attacks
	CSRFRateLimit = 0.083 // ~5 requests/minute

	// CSRFRateBurst burst size for CSRF endpoint
	CSRFRateBurst = 2

	// MetricsRateLimit rate limit for metrics endpoint (requests per second)
	// Strict: ~30 requests/minute for Prometheus scraping
	MetricsRateLimit = 0.5 // ~30 requests/minute

	// MetricsRateBurst burst size for metrics endpoint
	MetricsRateBurst = 10

	// StreamRateLimit rate limit for stream endpoint (requests per second)
	// Moderate: ~120 requests/minute to prevent bandwidth exhaustion
	StreamRateLimit = 2.0 // ~120 requests/minute

	// StreamRateBurst burst size for stream endpoint
	StreamRateBurst = 20

	// BufferPositionRateLimit is the per-address limit for
	// POST /torrents/{id}/buffer/position, in requests per second.
	//
	// This endpoint is playback telemetry, not a control-plane call: the Qt
	// frontend reports the current position every 500 ms for as long as a file
	// is playing (MainWindow::m_bufferPositionTimer). Under APIRateLimit —
	// 1 req/s, burst 10 — a single player exhausted the bucket within seconds
	// of starting and every later report was answered 429. The client does not
	// retry a 4xx, so piece priorities silently froze a few seconds into
	// playback, which is exactly the defect bug-0028 set out to remove.
	//
	// The budget therefore covers what the topology actually produces: several
	// participants of one room may share one public address, each reporting
	// twice a second. 20 req/s carries eight such players with headroom.
	//
	// Unlike the API group this limit has no environment override, on purpose.
	// An override would let the test suite raise it away, and the suite runs
	// with API_RATE_LIMIT relaxed for unrelated reasons — which is precisely
	// why shipped limits went unnoticed there. See
	// buffer_position_limit_test.go, which exercises the shipped values.
	BufferPositionRateLimit = 20.0

	// BufferPositionRateBurst is the burst for the same endpoint. It is above
	// BufferPositionRateLimit so a seek that lands during a burst of reports is
	// not lost, while a runaway client still runs the bucket dry.
	BufferPositionRateBurst = 40
)

// ── Auth Constants ────────────────────────────────────────────────────────
//
// Здесь НЕТ JWT. Аутентификация — это один access-токен, который сервер
// генерирует при старте и печатает в консоль; клиент присылает его в
// заголовке X-Access-Token, а сравнение выполняется constant-time.
// Ни токенов с отзывом, ни JTI, ни issuer/audience, ни библиотеки JWT в
// проекте нет и не планируется (см. DEV.md, «Принципы разработки»).

const (
	// BcryptCost bcrypt cost for password hashing
	BcryptCost = 12

	// MaxPasswordLength maximum password length for bcrypt
	MaxPasswordLength = 72
)

// ── Torrent Constants ─────────────────────────────────────────────────────

const (
	// TorrentGracefulShutdownTimeout timeout for graceful torrent service shutdown
	TorrentGracefulShutdownTimeout = 30 * time.Second

	// MagnetMetadataTimeout timeout for waiting for magnet metadata
	MagnetMetadataTimeout = 60 * time.Second

	// MaxTorrents maximum number of concurrent torrents (DoS protection)
	MaxTorrents = 100

	// MaxTorrentFileSize maximum torrent file size (1 MB)
	// Torrent files are small metadata files, 1MB is more than enough
	MaxTorrentFileSize int64 = 1 * 1024 * 1024

	// MaxStreamFileSize maximum file size for streaming (100 GB)
	MaxStreamFileSize int64 = 100 * 1024 * 1024 * 1024
)

// ── Pagination Constants ──────────────────────────────────────────────────

const (
	// DefaultPaginationLimit default pagination limit
	DefaultPaginationLimit = 20

	// MaxPaginationLimit maximum pagination limit
	MaxPaginationLimit = 100

	// MaxPaginationOffset maximum pagination offset (DoS protection)
	MaxPaginationOffset = 10000
)

// ── Request Constants ─────────────────────────────────────────────────────

const (
	// MaxRequestSize maximum request body size (1 MB)
	MaxRequestSize = 1 << 20

	// TorrentIDLength torrent ID length (SHA1 infohash in hex)
	TorrentIDLength = 40
)

// ── Buffer Constants ──────────────────────────────────────────────────────

const (
	// DefaultBufferPercent buffer percentage of file size
	DefaultBufferPercent = 10

	// DefaultBufferDuration buffer duration in seconds
	DefaultBufferDuration = 60

	// DefaultMaxBufferSize maximum buffer size (512 MB)
	DefaultMaxBufferSize = 512 * 1024 * 1024

	// BufferUpdateInterval priority update interval
	BufferUpdateInterval = 1 * time.Second

	// BufferNowPieces number of pieces for immediate download
	BufferNowPieces = 10

	// BufferHighPieces number of pieces for high priority
	BufferHighPieces = 50

	// BufferReadAheadPieces number of pieces to read ahead for buffer extension
	BufferReadAheadPieces = 20

	// MaxRoomPasswordLength maximum room password length (bcrypt truncates at 72)
	MaxRoomPasswordLength = 72

	// DefaultMemoryStorageCapacity default in-memory storage size (4 GB, 0 = unlimited)
	DefaultMemoryStorageCapacity int64 = 4 * 1024 * 1024 * 1024

	// MaxMemoryStorageCapacity maximum in-memory storage size (256 GB)
	MaxMemoryStorageCapacity int64 = 256 * 1024 * 1024 * 1024
)
