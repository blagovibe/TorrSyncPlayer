// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"

	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The room stops for the one who is behind, and the host is the one who stops
// it. For that the host needs to know a plain fact: is anyone in the room
// without data? The report is not a command, so nothing else may act on it.
//
// These tests pin that fact and its edges: the local peer is not in the
// roster (a peer does not hear its own report), a peer's recovery clears it,
// and a room with nobody buffering says so.

// TestTheRoomKnowsWhoIsBehind pins the fact the host reads.
func TestTheRoomKnowsWhoIsBehind(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := newFakeTransport("self", "guest")
	room := runRoom(ctx, t, tr, "self", "self-user", true, &Callbacks{})

	// The roster is built from hellos, so the guest has to say hello before
	// it can be in it.
	tr.feed("guest", NewHello("guest", "guest-user", "guest", false, nil))

	// Nobody is behind to begin with.
	waitFor(t, 5*time.Second, "the guest to be in the room", func() bool {
		return len(room.Peers()) == 1
	})
	assert.False(t, room.AnyoneBuffering(), "a room where nobody is behind must say so")

	// A guest reports that it ran out of data.
	tr.feed("guest", NewBuffering(true))
	waitFor(t, 5*time.Second, "the room to know the guest is behind", func() bool {
		return room.AnyoneBuffering()
	})

	// And the recovery clears it.
	tr.feed("guest", NewBuffering(false))
	waitFor(t, 5*time.Second, "the room to know the guest is back", func() bool {
		return !room.AnyoneBuffering()
	})
}

// TestTheLocalPeerIsNotInItsOwnRoster pins the edge the host's own stall
// depends on: a peer does not hear its own report, so the room's roster
// holds only the others. The host keeps its own stall beside itself, and
// this is why that is necessary.
func TestTheLocalPeerIsNotInItsOwnRoster(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := newFakeTransport("self")
	room := runRoom(ctx, t, tr, "self", "self-user", true, &Callbacks{})

	require.NoError(t, room.ReportBuffering(ctx, true))

	sent := tr.sentMessages()
	require.Len(t, sent, 1, "the report must have been sent")
	assert.False(t, room.AnyoneBuffering(),
		"a peer does not hear its own report, so the roster must not claim it is behind")
}
