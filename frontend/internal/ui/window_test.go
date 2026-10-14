// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/room"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The window holds every decision the application makes about the room, and
// until now none of it was exercised by anything. The failure mode is
// deceptive: a room event that is never applied looks exactly like a room where
// nothing happened.

func TestHumanSizeReadsLikeAPersonWould(t *testing.T) {
	// "1.0 MB" and "1048576 B" are both correct and one of them is unusable.
	for _, tc := range []struct {
		bytes int64
		want  string
	}{
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{276445467, "263.6 MB"},
		{1 << 30, "1.0 GB"},
	} {
		if got := humanSize(tc.bytes); got != tc.want {
			t.Errorf("humanSize(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestRosterLineNamesTheHostAndTheStalledPeer(t *testing.T) {
	// The roster line is how a person learns who they are watching with, and
	// who is stuck. Dropping either role makes it useless.
	var r room.Roster
	r.Apply([]room.Peer{
		{PeerID: "a", UserID: "host", IsHost: true},
		{PeerID: "b", UserID: "guest", Buffering: true},
	}, 1)

	peers := r.Peers()
	require.Len(t, peers, 2)

	labels := describeRoster(peers)
	require.Len(t, labels, 2)
	assert.Contains(t, labels[0], "host", "the host is not marked")
	assert.Contains(t, labels[1], "guest")
	assert.Contains(t, labels[1], "buffering", "a stalled peer is not marked")
}

func TestRosterLineIsEmptyWithoutPeers(t *testing.T) {
	assert.Empty(t, describeRoster(nil))
}

func TestSeekFollowsTheSmoothedPositionNotTheRequestedOne(t *testing.T) {
	// The server smooths jumps, so the position asked for and the position the
	// room actually moved to differ on purpose. Following the requested one
	// would put this client seconds ahead of everyone else — and it would do so
	// silently, because the numbers look plausible.
	position := positionFor(room.SyncEvent{
		Action:   "seek",
		Position: 120,
		Status:   &room.SyncStatus{IsPlaying: true, Position: 36, Duration: 7200},
	})
	assert.Equal(t, 36.0, position,
		"клиент пошёл по запрошенной позиции, а не по той, куда комната встала")

	// With no status there is nothing to follow but the request.
	position = positionFor(room.SyncEvent{Action: "seek", Position: 120})
	assert.Equal(t, 120.0, position)
}

func TestAZeroSmoothedPositionFallsBackToTheRequest(t *testing.T) {
	// A room that has not moved yet reports zero. Treating that as the truth
	// would yank the player back to the start on every seek into an unstarted
	// room.
	position := positionFor(room.SyncEvent{
		Action:   "seek",
		Position: 120,
		Status:   &room.SyncStatus{Position: 0},
	})
	assert.Equal(t, 120.0, position)
}

func TestTheHostDoesNotAddTheRoomTorrentItAlreadyHas(t *testing.T) {
	// The room broadcasts its torrent to everyone on every join. A host that
	// re-adds its own torrent each time would start a fresh download of a film
	// it is already playing.
	if shouldAdoptRoomTorrent(room.TorrentEvent{InfoHash: "abc", MagnetURI: "magnet:?x"}, "host", "host") {
		t.Fatal("the host re-added the torrent it already has")
	}
}

func TestAGuestAdoptsTheRoomTorrent(t *testing.T) {
	// This is the whole point of the room carrying a torrent: without it the
	// guest's backend stays empty and there is nothing to watch.
	if !shouldAdoptRoomTorrent(room.TorrentEvent{InfoHash: "abc", MagnetURI: "magnet:?x"}, "guest", "host") {
		t.Fatal("the guest did not adopt the room's torrent")
	}
}

func TestNothingIsAdoptedWithoutAMagnet(t *testing.T) {
	// An infohash identifies a torrent without carrying one. Adding by hash
	// alone produces a magnet with no trackers: it works over DHT and loses
	// every seeder.
	if shouldAdoptRoomTorrent(room.TorrentEvent{InfoHash: "abc"}, "guest", "host") {
		t.Fatal("a torrent with no magnet was adopted")
	}
}

func TestNothingIsAdoptedWithoutAHash(t *testing.T) {
	if shouldAdoptRoomTorrent(room.TorrentEvent{MagnetURI: "magnet:?x"}, "guest", "host") {
		t.Fatal("a magnet with no infohash was adopted")
	}
}

func TestJoinIsRefusedWithoutATorrent(t *testing.T) {
	// A room is bound to a torrent and the server refuses a mismatch. Catching
	// it here keeps the message clear instead of returning a 400 from a
	// stranger's server.
	if canJoinRoom("", "0123456789abcdef0123456789abcdef") {
		t.Fatal("a join without a selected torrent was allowed")
	}
	if !canJoinRoom("abc123", "0123456789abcdef0123456789abcdef") {
		t.Fatal("a join with a torrent was refused")
	}
}
