// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package constants содержит общие константы приложения.
// Все магические числа из различных пакетов вынесены сюда для удобства поддержки.
package constants

import "time"

// ── Server Constants ──────────────────────────────────────────────────────

const (
	// DefaultPort порт сервера по умолчанию
	DefaultPort = "8889"

	// ServerShutdownTimeout таймаут для graceful shutdown сервера
	ServerShutdownTimeout = 30 * time.Second

	// ServerReadTimeout таймаут чтения HTTP запроса
	ServerReadTimeout = 30 * time.Second

	// ServerWriteTimeout таймаут записи HTTP ответа
	ServerWriteTimeout = 30 * time.Second

	// ServerIdleTimeout таймаут простоя HTTP соединения
	ServerIdleTimeout = 120 * time.Second

	// PprofPort порт для pprof сервера
	PprofPort = ":6060"

	// SelfSignedCertValidity срок действия self-signed сертификата
	SelfSignedCertValidity = 365 * 24 * time.Hour // 1 год
)

// ── CSRF Constants ────────────────────────────────────────────────────────

const (
	// CSRFTokenTTL время жизни CSRF токена
	CSRFTokenTTL = 1 * time.Hour

	// CSRFTokenStoreMaxSize максимальное количество CSRF токенов в хранилище
	CSRFTokenStoreMaxSize = 10000

	// CSRFCleanupInterval интервал очистки истёкших CSRF токенов
	CSRFCleanupInterval = 5 * time.Minute

	// CSRFShutdownTimeout таймаут ожидания завершения CSRF cleanup горутины
	CSRFShutdownTimeout = 5 * time.Second

	// CSRFTokenBytes количество байт для генерации CSRF токена
	CSRFTokenBytes = 32

	// MinJWTTokenLength минимальная длина JWT токена для пропуска CSRF проверки
	MinJWTTokenLength = 10
)

// ── CORS Constants ────────────────────────────────────────────────────────

const (
	// CORSMaxAge время кэширования preflight запросов (24 часа)
	CORSMaxAge = "86400"

	// CORSAllowMethods разрешённые HTTP методы для CORS
	CORSAllowMethods = "GET, POST, PUT, DELETE, OPTIONS"

	// CORSAllowHeaders разрешённые заголовки для CORS
	CORSAllowHeaders = "Content-Type, Authorization, X-Requested-With, X-CSRF-Token, X-Session-ID"

	// CORSExposeHeaders заголовки, доступные клиенту
	CORSExposeHeaders = "X-CSRF-Token"

	// HSTSMaxAge срок действия HSTS заголовка (1 год в секундах)
	HSTSMaxAge = "max-age=31536000; includeSubDomains"
)

// ── Rate Limiting Constants ───────────────────────────────────────────────

const (
	// AuthRateLimit лимит запросов для auth endpoints (запросов в секунду)
	AuthRateLimit = 0.17 // ~10 запросов/минуту

	// AuthRateBurst размер burst для auth endpoints
	AuthRateBurst = 5

	// DefaultRateLimit лимит запросов для остальных endpoints (запросов в секунду)
	DefaultRateLimit = 1 // 60 запросов/минуту

	// DefaultRateBurst размер burst для остальных endpoints
	DefaultRateBurst = 10
)

// ── JWT Constants ─────────────────────────────────────────────────────────

const (
	// JWTTokenTTL время жизни JWT токена
	JWTTokenTTL = 24 * time.Hour

	// JWTSecretLength длина JWT секрета в байтах
	JWTSecretLength = 32

	// JTIBytes количество байт для генерации JWT ID
	JTIBytes = 16

	// BcryptCost стоимость bcrypt для хеширования паролей
	BcryptCost = 12

	// MaxPasswordLength максимальная длина пароля для bcrypt
	MaxPasswordLength = 72

	// RevocationStoreTTL время хранения отозванных токенов
	RevocationStoreTTL = 24 * time.Hour
)

// ── P2P Constants ─────────────────────────────────────────────────────────

const (
	// P2PEventChannelSize размер буфера канала P2P событий
	P2PEventChannelSize = 100

	// PeerIDLength длина идентификатора пира в байтах
	PeerIDLength = 16

	// P2PDefaultRoomAuth требовать аутентификацию по умолчанию
	P2PDefaultRoomAuth = true
)

// ── Torrent Constants ─────────────────────────────────────────────────────

const (
	// TorrentGracefulShutdownTimeout таймаут для graceful shutdown торрент-сервиса
	TorrentGracefulShutdownTimeout = 30 * time.Second
)

// ── Sync Constants ────────────────────────────────────────────────────────

const (
	// MaxPositionJump максимальный прыжок позиции в секундах для плавной подстройки
	MaxPositionJump = 2.0

	// SmoothAdjustmentRatio коэффициент плавной подстройки позиции
	SmoothAdjustmentRatio = 0.3

	// MsPerSecond количество миллисекунд в одной секунде
	MsPerSecond = 1000.0
)

// ── SSE Constants ─────────────────────────────────────────────────────────

const (
	// MaxSSEConnections максимальное количество одновременных SSE соединений на комнату
	MaxSSEConnections = 100

	// SSETimout таймаут для SSE соединения
	SSETimout = 30 * time.Minute

	// SSEPingInterval интервал отправки ping для поддержания SSE соединения
	SSEPingInterval = 30 * time.Second
)

// ── Pagination Constants ──────────────────────────────────────────────────

const (
	// DefaultPaginationLimit лимит пагинации по умолчанию
	DefaultPaginationLimit = 20

	// MaxPaginationLimit максимальный лимит пагинации
	MaxPaginationLimit = 100
)

// ── Request Constants ─────────────────────────────────────────────────────

const (
	// MaxRequestSize максимальный размер тела запроса (1 MB)
	MaxRequestSize = 1 << 20

	// TorrentIDLength длина ID торрента (SHA1 infohash в hex)
	TorrentIDLength = 40
)

// ── TLS Constants ─────────────────────────────────────────────────────────

const (
	// TLSMinVersion минимальная версия TLS
	TLSMinVersion = 0x0303 // TLS 1.2
)

// ── Buffer Constants ──────────────────────────────────────────────────────

const (
	// DefaultBufferPercent процент буферизации от размера файла
	DefaultBufferPercent = 10

	// DefaultBufferDuration длительность буфера в секундах
	DefaultBufferDuration = 60

	// DefaultMaxBufferSize максимальный размер буфера (512 МБ)
	DefaultMaxBufferSize = 512 * 1024 * 1024

	// DefaultPreBufferPercent процент предварительной буферизации перед стартом
	DefaultPreBufferPercent = 5

	// BufferUpdateInterval интервал обновления приоритетов
	BufferUpdateInterval = 1 * time.Second

	// PiecePriorityNow немедленная загрузка
	PiecePriorityNow = 4

	// PiecePriorityHigh высокий приоритет
	PiecePriorityHigh = 3

	// PiecePriorityNormal обычный приоритет
	PiecePriorityNormal = 2

	// PiecePriorityReadahead предзагрузка
	PiecePriorityReadahead = 1

	// PiecePriorityNone не загружать
	PiecePriorityNone = 0
)
