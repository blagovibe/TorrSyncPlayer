// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api provides HTTP API for the server.
// Contains API path constants for use in router and tests.
package api

// ── API Path Constants ─────────────────────────────────────────────────

const (
	// HealthCheck health check path (public)
	APIPathHealth = "/health"

	// HealthCheckDetailed detailed health check path
	APIPathHealthDetailed = "/health/detailed"

	// Version path for getting server version
	APIPathVersion = "/api/v1/version"

	// Metrics path for Prometheus metrics
	APIPathMetrics = "/metrics"

	// CSRFToken path for getting CSRF token
	APIPathCSRFToken = "/api/v1/csrf-token"

	// ── Auth endpoints ─────────────────────────────────────────────────

	// AuthRegister registration path
	APIPathAuthRegister = "/api/v1/auth/register"

	// AuthLogin login path
	APIPathAuthLogin = "/api/v1/auth/login"

	// AuthLogout logout path
	APIPathAuthLogout = "/api/v1/auth/logout"

	// ── Torrent endpoints ──────────────────────────────────────────────

	// Torrents torrent list path
	APIPathTorrents = "/api/v1/torrents"

	// TorrentFiles torrent files path (use with {id})
	APIPathTorrentFiles = "/api/v1/torrents/{id}/files"

	// TorrentSelect file selection path (use with {id})
	APIPathTorrentSelect = "/api/v1/torrents/{id}/select"

	// TorrentStream streaming path (use with {id})
	APIPathTorrentStream = "/api/v1/torrents/{id}/stream"

	// TorrentRemove torrent removal path (use with {id})
	APIPathTorrentRemove = "/api/v1/torrents/{id}"

	// ── Room endpoints ─────────────────────────────────────────────────

	// Rooms room creation path
	APIPathRooms = "/api/v1/rooms"

	// RoomJoin room join path
	APIPathRoomJoin = "/api/v1/rooms/join"

	// RoomLeave room leave path
	APIPathRoomLeave = "/api/v1/rooms/leave"

	// RoomSignal signal sending path
	APIPathRoomSignal = "/api/v1/rooms/signal"

	// RoomEvents room SSE events path
	APIPathRoomEvents = "/api/v1/rooms/events"

	// ── Sync endpoints ─────────────────────────────────────────────────

	// SyncPlay playback synchronization path
	APIPathSyncPlay = "/api/v1/sync/play"

	// SyncPause pause synchronization path
	APIPathSyncPause = "/api/v1/sync/pause"

	// SyncSeek seek synchronization path
	APIPathSyncSeek = "/api/v1/sync/seek"

	// SyncStatus sync status path
	APIPathSyncStatus = "/api/v1/sync/status"
)
