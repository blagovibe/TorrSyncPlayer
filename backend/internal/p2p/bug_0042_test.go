// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package p2p

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bug-0042: SetPeerBuffering nil deref + emitEvent race on closed channel.
//
// 1. SetPeerBuffering must not panic when session.GetPeer() returns nil.
// 2. emitEvent must not panic when called concurrently with Close().

func TestSetPeerBuffering_NoPanicOnNilPeer(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create room and join as host
	_, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)

	// Explicitly set peer to nil to simulate GenerateID failure
	session := svc.getSession("host")
	require.NotNil(t, session)
	session.SetPeer(nil) // simulate nil peer

	// This should NOT panic, even with nil peer
	assert.NotPanics(t, func() {
		svc.SetPeerBuffering(ctx, "host", true)
	})
}

func TestEmitEvent_NoPanicOnConcurrentClose(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create room with host and guest
	info, err := svc.CreateRoom(ctx, "host", "Room", "", "torrent-A")
	require.NoError(t, err)
	roomID := info.ID

	require.NoError(t, svc.JoinRoom(ctx, "guest", roomID, "", "torrent-A"))

	// Emit an event to warm up
	pos := 0.0
	svc.emitEvent(roomID, models.EventTypeSync, models.SyncEvent{
		Action: "play", Position: &pos, Initiator: "host",
	})

	// Now call emitEvent concurrently with Close
	// This tests the race where emitEvent checks closed/doneChan,
	// but Close closes channels before emitEvent can send
	var panicOccurred atomic.Bool
	var wg sync.WaitGroup

	// Spawn many concurrent emitEvent calls
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicOccurred.Store(true)
				}
			}()
			svc.emitEvent(roomID, models.EventTypeSync, models.SyncEvent{
				Action: "play", Position: &pos, Initiator: "host",
			})
		}()
	}

	// Give goroutines time to start
	time.Sleep(10 * time.Millisecond)

	// Close the service while emits are in flight
	svc.Close()

	// Wait for all goroutines to finish (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// all goroutines finished
	case <-time.After(5 * time.Second):
		t.Fatal("goroutines did not finish in time - likely deadlock")
	}

	// No panic should have occurred
	assert.False(t, panicOccurred.Load(), "emitEvent should not panic when called concurrently with Close")
}
