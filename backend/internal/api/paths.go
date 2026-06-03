// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api предоставляет HTTP API для сервера.
// Содержит константы путей API для использования в роутере и тестах.
package api

// ── API Path Constants ─────────────────────────────────────────────────

const (
	// HealthCheck путь для проверки здоровья (публичный)
	APIPathHealth = "/health"

	// HealthCheckDetailed путь для детальной проверки здоровья
	APIPathHealthDetailed = "/health/detailed"

	// Version путь для получения версии сервера
	APIPathVersion = "/api/v1/version"

	// Metrics путь для метрик Prometheus
	APIPathMetrics = "/metrics"

	// CSRFToken путь для получения CSRF токена
	APIPathCSRFToken = "/api/v1/csrf-token"

	// ── Auth endpoints ─────────────────────────────────────────────────

	// AuthRegister путь для регистрации
	APIPathAuthRegister = "/api/v1/auth/register"

	// AuthLogin путь для входа
	APIPathAuthLogin = "/api/v1/auth/login"

	// AuthLogout путь для выхода
	APIPathAuthLogout = "/api/v1/auth/logout"

	// ── Torrent endpoints ──────────────────────────────────────────────

	// Torrents путь для списка торрентов
	APIPathTorrents = "/api/v1/torrents"

	// TorrentFiles путь для файлов торрента (использовать с {id})
	APIPathTorrentFiles = "/api/v1/torrents/{id}/files"

	// TorrentSelect путь для выбора файла (использовать с {id})
	APIPathTorrentSelect = "/api/v1/torrents/{id}/select"

	// TorrentStream путь для стриминга (использовать с {id})
	APIPathTorrentStream = "/api/v1/torrents/{id}/stream"

	// TorrentRemove путь для удаления торрента (использовать с {id})
	APIPathTorrentRemove = "/api/v1/torrents/{id}"

	// ── Room endpoints ─────────────────────────────────────────────────

	// Rooms путь для создания комнаты
	APIPathRooms = "/api/v1/rooms"

	// RoomJoin путь для присоединения к комнате
	APIPathRoomJoin = "/api/v1/rooms/join"

	// RoomLeave путь для выхода из комнаты
	APIPathRoomLeave = "/api/v1/rooms/leave"

	// RoomSignal путь для отправки сигнала
	APIPathRoomSignal = "/api/v1/rooms/signal"

	// RoomEvents путь для SSE событий комнаты
	APIPathRoomEvents = "/api/v1/rooms/events"

	// ── Sync endpoints ─────────────────────────────────────────────────

	// SyncPlay путь для синхронизации воспроизведения
	APIPathSyncPlay = "/api/v1/sync/play"

	// SyncPause путь для синхронизации паузы
	APIPathSyncPause = "/api/v1/sync/pause"

	// SyncSeek путь для синхронизации перемотки
	APIPathSyncSeek = "/api/v1/sync/seek"

	// SyncStatus путь для получения статуса синхронизации
	APIPathSyncStatus = "/api/v1/sync/status"
)
