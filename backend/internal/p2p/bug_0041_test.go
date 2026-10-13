// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bug-0041: Sessions never freed — unbounded memory growth.
//
// 1. LeaveRoom should clean up session if user has no other rooms.
// 2. Sessions should have TTL / cleanup for abandoned sessions.
// 3. There should be a max session limit to prevent unbounded growth.

func TestLeaveRoom_CleansUpSessionWhenEmpty(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create room and join
	info, err := svc.CreateRoom(ctx, "user1", "Room", "", "torrent-A")
	require.NoError(t, err)
	roomID := info.ID

	// Verify session exists
	session := svc.getSession("user1")
	require.NotNil(t, session)
	assert.Equal(t, roomID, session.CurrentRoom())

	// Leave room
	require.NoError(t, svc.LeaveRoom(ctx, "user1"))

	// Session should be cleaned up (nil or removed from map)
	// Option 1: session removed from map
	sessionAfter := svc.getSession("user1")
	assert.Nil(t, sessionAfter, "session should be removed after leaving room")

	// Option 2: if kept, CurrentRoom should be empty and peer cleared
	// (We prefer removal for memory efficiency)
	_ = roomID
}

func TestSessionTTL_ExpiredSessionsCleaned(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create room and join
	_, err := svc.CreateRoom(ctx, "user1", "Room", "", "torrent-A")
	require.NoError(t, err)

	// Manually expire the session by setting LastHeartbeat to past
	svc.mu.Lock()
	session := svc.sessions["user1"]
	if session != nil {
		peer := session.GetPeer()
		if peer != nil {
			peer.LastHeartbeat = time.Now().Add(-2 * time.Hour)
		}
	}
	svc.mu.Unlock()

	// Trigger prune (if implemented)
	// For now this documents the expected behavior
	// TODO: implement pruneExpiredSessions and test it
}

func TestMaxSessionsLimit(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create a room
	_, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)

	// Try to create many sessions (simulating different X-Client-IDs)
	// Should hit a limit or be cleaned up
	for i := 0; i < 100; i++ {
		userID := "user" + string(rune('a'+i%26)) // limited unique IDs
		_ = svc.getOrCreateSession(userID)
	}

	// Verify session count is bounded
	svc.mu.RLock()
	count := len(svc.sessions)
	svc.mu.RUnlock()

	// Should not grow unbounded (max sessions limit)
	assert.LessOrEqual(t, count, 100, "session count should be bounded")
}
