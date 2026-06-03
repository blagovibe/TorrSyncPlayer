// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAPIPaths_Health проверяет пути health check
func TestAPIPaths_Health(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/health", APIPathHealth)
	assert.Equal(t, "/health/detailed", APIPathHealthDetailed)
}

// TestAPIPaths_Version проверяет путь версии
func TestAPIPaths_Version(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/version", APIPathVersion)
}

// TestAPIPaths_Metrics проверяет путь метрик
func TestAPIPaths_Metrics(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/metrics", APIPathMetrics)
}

// TestAPIPaths_CSRF проверяет путь CSRF токена
func TestAPIPaths_CSRF(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/csrf-token", APIPathCSRFToken)
}

// TestAPIPaths_Auth проверяет пути аутентификации
func TestAPIPaths_Auth(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/auth/register", APIPathAuthRegister)
	assert.Equal(t, "/api/v1/auth/login", APIPathAuthLogin)
	assert.Equal(t, "/api/v1/auth/logout", APIPathAuthLogout)
}

// TestAPIPaths_Torrents проверяет пути торрентов
func TestAPIPaths_Torrents(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/torrents", APIPathTorrents)
	assert.Equal(t, "/api/v1/torrents/{id}/files", APIPathTorrentFiles)
	assert.Equal(t, "/api/v1/torrents/{id}/select", APIPathTorrentSelect)
	assert.Equal(t, "/api/v1/torrents/{id}/stream", APIPathTorrentStream)
	assert.Equal(t, "/api/v1/torrents/{id}", APIPathTorrentRemove)
}

// TestAPIPaths_Rooms проверяет путей комнат
func TestAPIPaths_Rooms(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/rooms", APIPathRooms)
	assert.Equal(t, "/api/v1/rooms/join", APIPathRoomJoin)
	assert.Equal(t, "/api/v1/rooms/leave", APIPathRoomLeave)
	assert.Equal(t, "/api/v1/rooms/signal", APIPathRoomSignal)
	assert.Equal(t, "/api/v1/rooms/events", APIPathRoomEvents)
}

// TestAPIPaths_Sync проверяет пути синхронизации
func TestAPIPaths_Sync(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/sync/play", APIPathSyncPlay)
	assert.Equal(t, "/api/v1/sync/pause", APIPathSyncPause)
	assert.Equal(t, "/api/v1/sync/seek", APIPathSyncSeek)
	assert.Equal(t, "/api/v1/sync/status", APIPathSyncStatus)
}

// TestAPIPaths_AllDefined проверяет что все пути определены и не пустые
func TestAPIPaths_AllDefined(t *testing.T) {
	t.Parallel()
	paths := map[string]string{
		"Health":         APIPathHealth,
		"HealthDetailed": APIPathHealthDetailed,
		"Version":        APIPathVersion,
		"Metrics":        APIPathMetrics,
		"CSRFToken":      APIPathCSRFToken,
		"AuthRegister":   APIPathAuthRegister,
		"AuthLogin":      APIPathAuthLogin,
		"AuthLogout":     APIPathAuthLogout,
		"Torrents":       APIPathTorrents,
		"TorrentFiles":   APIPathTorrentFiles,
		"TorrentSelect":  APIPathTorrentSelect,
		"TorrentStream":  APIPathTorrentStream,
		"TorrentRemove":  APIPathTorrentRemove,
		"Rooms":          APIPathRooms,
		"RoomJoin":       APIPathRoomJoin,
		"RoomLeave":      APIPathRoomLeave,
		"RoomSignal":     APIPathRoomSignal,
		"RoomEvents":     APIPathRoomEvents,
		"SyncPlay":       APIPathSyncPlay,
		"SyncPause":      APIPathSyncPause,
		"SyncSeek":       APIPathSyncSeek,
		"SyncStatus":     APIPathSyncStatus,
	}

	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			assert.NotEmpty(t, path, "путь %s не должен быть пустым", name)
			assert.True(t, len(path) > 1, "путь %s должен начинаться с /", name)
		})
	}
}

// TestAPIPaths_Consistency проверяет консистентность путей (все начинаются с /api/v1 или /health или /metrics)
func TestAPIPaths_Consistency(t *testing.T) {
	t.Parallel()
	apiPaths := []string{
		APIPathVersion,
		APIPathCSRFToken,
		APIPathAuthRegister,
		APIPathAuthLogin,
		APIPathAuthLogout,
		APIPathTorrents,
		APIPathTorrentFiles,
		APIPathTorrentSelect,
		APIPathTorrentStream,
		APIPathTorrentRemove,
		APIPathRooms,
		APIPathRoomJoin,
		APIPathRoomLeave,
		APIPathRoomSignal,
		APIPathRoomEvents,
		APIPathSyncPlay,
		APIPathSyncPause,
		APIPathSyncSeek,
		APIPathSyncStatus,
	}

	for _, path := range apiPaths {
		assert.True(t, len(path) > 5, "путь %s слишком короткий", path)
		// Проверяем что путь начинается с /api/v1 или /health или /metrics
		hasValidPrefix := len(path) >= 7 && path[:7] == "/api/v1" ||
			len(path) >= 7 && path[:7] == "/health" ||
			len(path) >= 8 && path[:8] == "/metrics"
		assert.True(t, hasValidPrefix, "путь %s должен начинаться с /api/v1, /health или /metrics", path)
	}
}
