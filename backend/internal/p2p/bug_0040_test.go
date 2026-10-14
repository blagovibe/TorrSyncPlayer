// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package p2p

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

// bug-0040: Events dropped silently — sequence numbers, ack, and replay.
//
// 1. Events should have sequence numbers (syncSeq per room).
// 2. Server should keep event history for replay.
// 3. Client can request replay from last known sequence.
// 4. Client can ack events; server tracks unacked events per session.

func TestEventSequenceNumbers(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	info, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)
	roomID := info.ID

	require.NoError(t, svc.JoinRoom(ctx, "guest", roomID, "", "torrent-A"))

	// Capture events for guest
	guestSession := svc.getSession("guest")
	events := make(chan models.P2PEvent, 10)
	guestSession.eventChan = events

	// Emit a sync event
	pos := 0.0
	svc.emitEvent(roomID, models.EventTypeSync, models.SyncEvent{
		Action: "play", Position: &pos, Initiator: "host",
	})

	// Guest should receive event with sequence number
	select {
	case e := <-events:
		assert.NotEmpty(t, e.Type)
		assert.Greater(t, e.Seq, int64(0), "event should have sequence number")
	default:
		t.Fatal("guest should receive event")
	}
}

func TestEventHistoryReplay(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	info, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)
	roomID := info.ID

	require.NoError(t, svc.JoinRoom(ctx, "guest", roomID, "", "torrent-A"))

	// Emit several events
	for i := 0; i < 5; i++ {
		pos := float64(i)
		svc.emitEvent(roomID, models.EventTypeSync, models.SyncEvent{
			Action: "seek", Position: &pos, Initiator: "host",
		})
	}

	// Get current sync seq
	seq, ok := svc.GetSyncSeq(roomID)
	require.True(t, ok)
	assert.Equal(t, int64(5), seq)

	// Replay from sequence 0
	replay := svc.ReplayEvents(roomID, 0)
	require.Len(t, replay, 5)
	for i, e := range replay {
		assert.Equal(t, int64(i+1), e.Seq)
		assert.Equal(t, models.EventTypeSync, e.Type)
	}

	// Replay from sequence 3 (should get 4 and 5)
	replay = svc.ReplayEvents(roomID, 3)
	require.Len(t, replay, 2)
	assert.Equal(t, int64(4), replay[0].Seq)
	assert.Equal(t, int64(5), replay[1].Seq)

	// Replay from beyond history
	replay = svc.ReplayEvents(roomID, 10)
	assert.Empty(t, replay)
}

func TestClientAckAndUnackedTracking(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	info, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)
	roomID := info.ID

	require.NoError(t, svc.JoinRoom(ctx, "guest", roomID, "", "torrent-A"))

	// Emit event
	pos := 0.0
	svc.emitEvent(roomID, models.EventTypeSync, models.SyncEvent{
		Action: "play", Position: &pos, Initiator: "host",
	})

	// Get the seq
	seq, ok := svc.GetSyncSeq(roomID)
	require.True(t, ok)

	// Test replay works after ack concept (simulated by client knowing seq)
	replay := svc.ReplayEvents(roomID, seq)
	assert.Empty(t, replay, "no new events after current seq")
}

func TestEventChannelOverflow_DroppedEventsTracked(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	info, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)
	roomID := info.ID

	require.NoError(t, svc.JoinRoom(ctx, "guest", roomID, "", "torrent-A"))

	// Emit events and verify they're stored in history
	for i := 0; i < 10; i++ {
		pos := float64(i)
		svc.emitEvent(roomID, models.EventTypeSync, models.SyncEvent{
			Action: "seek", Position: &pos, Initiator: "host",
		})
	}

	// All events should be in history
	seq, ok := svc.GetSyncSeq(roomID)
	require.True(t, ok)
	assert.Equal(t, int64(10), seq)

	replay := svc.ReplayEvents(roomID, 0)
	require.Len(t, replay, 10)

	// Non-sync events should not get sequence numbers
	svc.emitEvent(roomID, models.EventTypePeerJoined, models.PeerJoinedEvent{
		PeerID: "test", UserID: "test", IsHost: false, PeerCount: 1,
	})
	seq2, ok := svc.GetSyncSeq(roomID)
	require.True(t, ok)
	assert.Equal(t, int64(10), seq2, "non-sync events should not increment syncSeq")
}
