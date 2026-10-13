// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package p2p

import (
	"context"
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bug-0050: host transfer on leave.
//
// When the host leaves a room, the room should survive with a new host.
// Without this, play/pause/seek commands become impossible because the
// original host's HostUserID still gates IsHost().

func TestHostTransfer_LeavingHostTransfersRights(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create room with host
	info, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)
	roomID := info.ID

	// Guest joins
	require.NoError(t, svc.JoinRoom(ctx, "guest", roomID, "", "torrent-A"))

	// Verify initial host
	isHost, err := svc.IsHost(ctx, "host", roomID)
	require.NoError(t, err)
	assert.True(t, isHost, "host should be host")

	isHost, err = svc.IsHost(ctx, "guest", roomID)
	require.NoError(t, err)
	assert.False(t, isHost, "guest should not be host")

	// Host leaves
	require.NoError(t, svc.LeaveRoom(ctx, "host"))

	// Guest should now be host
	isHost, err = svc.IsHost(ctx, "guest", roomID)
	require.NoError(t, err)
	assert.True(t, isHost, "guest should become host after host leaves")

	// Room should still exist
	roomInfo, err := svc.GetRoomInfo(ctx, "guest")
	require.NoError(t, err)
	assert.Equal(t, roomID, roomInfo.ID)
	assert.Equal(t, "guest", roomInfo.HostUserID)
}

// Host leaves, multiple guests — first joined becomes new host (deterministic).
func TestHostTransfer_MultipleGuestsFirstJoined(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	info, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)
	roomID := info.ID

	require.NoError(t, svc.JoinRoom(ctx, "guest1", roomID, "", "torrent-A"))
	require.NoError(t, svc.JoinRoom(ctx, "guest2", roomID, "", "torrent-A"))
	require.NoError(t, svc.JoinRoom(ctx, "guest3", roomID, "", "torrent-A"))

	// Host leaves
	require.NoError(t, svc.LeaveRoom(ctx, "host"))

	// guest1 (first joined) should be host
	isHost, err := svc.IsHost(ctx, "guest1", roomID)
	require.NoError(t, err)
	assert.True(t, isHost)

	isHost, err = svc.IsHost(ctx, "guest2", roomID)
	require.NoError(t, err)
	assert.False(t, isHost)

	isHost, err = svc.IsHost(ctx, "guest3", roomID)
	require.NoError(t, err)
	assert.False(t, isHost)

	roomInfo, err := svc.GetRoomInfo(ctx, "guest1")
	require.NoError(t, err)
	assert.Equal(t, "guest1", roomInfo.HostUserID)
}

// Host is the only participant — room is deleted, no transfer needed.
func TestHostTransfer_SoloHostDeletesRoom(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	info, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)
	roomID := info.ID

	require.NoError(t, svc.LeaveRoom(ctx, "host"))

	// Room should be gone: attempting to join should fail with room not found
	err = svc.JoinRoom(ctx, "another", roomID, "", "torrent-A")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// Host leaves — BroadcastSync should include new host in recipients.
func TestHostTransfer_BroadcastSyncIncludesNewHost(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	info, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)
	roomID := info.ID

	require.NoError(t, svc.JoinRoom(ctx, "guest", roomID, "", "torrent-A"))

	// Track events received by guest
	received := make(chan models.P2PEvent, 2)
	session := svc.getSession("guest")
	session.eventChan = received

	require.NoError(t, svc.LeaveRoom(ctx, "host"))

	// guest should get PeerLeft for host, then PeerJoined for itself as new host?
	// Actually the room remains, peer list updates with new host flag.
	// Check that at least one event arrives.
	select {
	case e := <-received:
		assert.NotEmpty(t, e.Type)
	default:
		t.Fatal("guest should receive room event after host leaves")
	}
}
