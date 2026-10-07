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

	// ServerWriteTimeout HTTP response write timeout
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
)

// ── JWT Constants ─────────────────────────────────────────────────────────

const (
	// JWTTokenTTL JWT token lifetime
	JWTTokenTTL = 24 * time.Hour

	// JWTSecretLength JWT secret length in bytes
	JWTSecretLength = 32

	// JTIBytes number of bytes for JWT ID generation
	JTIBytes = 16

	// JWTIssuer issuer claim (iss) embedded in and required by tokens.
	JWTIssuer = "torrsyncplayer"

	// JWTAudience audience claim (aud) embedded in and required by tokens.
	JWTAudience = "torrsyncplayer-api"

	// BcryptCost bcrypt cost for password hashing
	BcryptCost = 12

	// MaxPasswordLength maximum password length for bcrypt
	MaxPasswordLength = 72

	// RevocationStoreTTL revoked token storage duration
	RevocationStoreTTL = 24 * time.Hour

	// RevocationCleanupInterval interval for scanning expired revocation entries.
	// Shorter than RevocationStoreTTL so revoked tokens do not linger too long.
	RevocationCleanupInterval = 15 * time.Minute

	// MinTokenLength minimum JWT token length for validation
	MinTokenLength = 30

	// StreamTicketTTL lifetime of a short-lived, HMAC-signed stream ticket used
	// by the media player (libmpv) to authenticate /stream requests without a
	// JWT/CSR token, which it cannot attach to its own HTTP fetches.
	StreamTicketTTL = 5 * time.Minute

	// StreamTicketSecret domain-separation prefix prepended to the JWT secret
	// when deriving the HMAC key for stream tickets.
	StreamTicketSecret = "stream-ticket-v1"
)

// ── P2P Constants ─────────────────────────────────────────────────────────

const (
	// P2PEventChannelSize P2P event channel buffer size
	P2PEventChannelSize = 100

	// PeerIDLength peer identifier length in bytes
	PeerIDLength = 16

	// MaxSignalSize maximum room-signal payload size in bytes (64 KB).
	// Signals are relayed as opaque JSON through the server (no WebRTC), so this
	// is a DoS guard on the broker, not an SDP/ICE size limit.
	MaxSignalSize = 64 * 1024

	// MaxRooms maximum number of concurrent P2P rooms
	MaxRooms = 1000

	// P2PCloseTimeoutDefault timeout for graceful P2P service shutdown
	P2PCloseTimeoutDefault = 5 * time.Second

	// P2PDebounceInterval debounce interval for token revocation persistence
	P2PDebounceInterval = 5 * time.Second

	// PeerIdleTimeout maximum allowed time between heartbeats before a peer is
	// considered disconnected and pruned from its room. Clients refresh their
	// presence by rejoining; without pruning, abandoned sessions leak peers.
	PeerIdleTimeout = 5 * time.Minute

	// PeerPruneInterval how often the idle-peer sweep runs.
	PeerPruneInterval = 1 * time.Minute
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

// ── Sync Constants ────────────────────────────────────────────────────────

const (
	// MaxPositionJump maximum position jump in seconds for smooth adjustment
	MaxPositionJump = 2.0

	// SmoothAdjustmentRatio smooth position adjustment ratio
	SmoothAdjustmentRatio = 0.3
)

// ── SSE Constants ─────────────────────────────────────────────────────────

const (
	// MaxSSEConnections maximum number of concurrent SSE connections per room
	MaxSSEConnections = 100

	// MaxGlobalSSEConnections maximum total SSE connections across all rooms
	// Independent of room count to prevent DoS via many rooms
	MaxGlobalSSEConnections = 1000

	// SSETimeout SSE connection timeout
	SSETimeout = 30 * time.Minute

	// SSEPingInterval ping interval for keeping SSE connection alive
	SSEPingInterval = 30 * time.Second
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
