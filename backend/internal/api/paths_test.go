// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAPIPaths_Health tests health check paths
func TestAPIPaths_Health(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/health", APIPathHealth)
	assert.Equal(t, "/health/detailed", APIPathHealthDetailed)
}

// TestAPIPaths_Version tests version path
func TestAPIPaths_Version(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/version", APIPathVersion)
}

// TestAPIPaths_Metrics tests metrics path
func TestAPIPaths_Metrics(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/metrics", APIPathMetrics)
}

// TestAPIPaths_CSRF tests CSRF token path
func TestAPIPaths_CSRF(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/csrf-token", APIPathCSRFToken)
}

// TestAPIPaths_Auth tests authentication paths
func TestAPIPaths_Auth(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/auth/register", APIPathAuthRegister)
	assert.Equal(t, "/api/v1/auth/login", APIPathAuthLogin)
	assert.Equal(t, "/api/v1/auth/logout", APIPathAuthLogout)
}

// TestAPIPaths_Torrents tests torrent paths
func TestAPIPaths_Torrents(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/torrents", APIPathTorrents)
	assert.Equal(t, "/api/v1/torrents/{id}/files", APIPathTorrentFiles)
	assert.Equal(t, "/api/v1/torrents/{id}/select", APIPathTorrentSelect)
	assert.Equal(t, "/api/v1/torrents/{id}/stream", APIPathTorrentStream)
	assert.Equal(t, "/api/v1/torrents/{id}", APIPathTorrentRemove)
}

// TestAPIPaths_Rooms tests room paths
func TestAPIPaths_Rooms(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/rooms", APIPathRooms)
	assert.Equal(t, "/api/v1/rooms/join", APIPathRoomJoin)
	assert.Equal(t, "/api/v1/rooms/leave", APIPathRoomLeave)
	assert.Equal(t, "/api/v1/rooms/signal", APIPathRoomSignal)
	assert.Equal(t, "/api/v1/rooms/events", APIPathRoomEvents)
}

// TestAPIPaths_Sync tests sync paths
func TestAPIPaths_Sync(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/v1/sync/play", APIPathSyncPlay)
	assert.Equal(t, "/api/v1/sync/pause", APIPathSyncPause)
	assert.Equal(t, "/api/v1/sync/seek", APIPathSyncSeek)
	assert.Equal(t, "/api/v1/sync/status", APIPathSyncStatus)
}

// TestAPIPaths_AllDefined tests that all paths are defined and non-empty
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
			assert.NotEmpty(t, path, "path %s must not be empty", name)
			assert.True(t, len(path) > 1, "path %s should start with /", name)
		})
	}
}

// TestAPIPaths_Consistency tests path consistency (all start with /api/v1 or /health or /metrics)
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
		assert.True(t, len(path) > 5, "path %s is too short", path)
		// Check that path starts with /api/v1 or /health or /metrics
		hasValidPrefix := len(path) >= 7 && path[:7] == "/api/v1" ||
			len(path) >= 7 && path[:7] == "/health" ||
			len(path) >= 8 && path[:8] == "/metrics"
		assert.True(t, hasValidPrefix, "path %s must start with /api/v1, /health or /metrics", path)
	}
}
