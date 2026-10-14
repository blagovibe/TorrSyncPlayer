// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/room"
)

// The decision that belongs to the window and nowhere else: whether the room's
// torrent event is meant for this peer at all.
//
// The room broadcasts its torrent on every join, so the host is offered its own
// film back each time. The host already has it, and re-adding it would start a
// fresh download of something it is playing.

// TestTheHostIsOfferedItsOwnFilmAndRefusesIt keeps the host from re-adding what
// it is already playing.
func TestTheHostIsOfferedItsOwnFilmAndRefusesIt(t *testing.T) {
	event := room.TorrentEvent{
		InfoHash:     "abc",
		MagnetURI:    "magnet:?xt=urn:btih:abc",
		SelectedFile: 1,
	}

	if !shouldAdoptRoomTorrent(event, "guest", "host") {
		t.Fatal("a guest refused the room's torrent")
	}
	if shouldAdoptRoomTorrent(event, "host", "host") {
		t.Error("the host adopted its own torrent event")
	}
}

// TestATorrentEventWithoutAMagnetIsRefused is the other guard: an infohash
// identifies a torrent without carrying one, and adding by hash alone builds a
// magnet with no trackers — it resolves over DHT and loses every seeder.
func TestATorrentEventWithoutAMagnetIsRefused(t *testing.T) {
	hashOnly := room.TorrentEvent{InfoHash: "abc", SelectedFile: 0}
	if shouldAdoptRoomTorrent(hashOnly, "guest", "host") {
		t.Error("a torrent event with no magnet was adopted")
	}

	magnetOnly := room.TorrentEvent{MagnetURI: "magnet:?xt=urn:btih:abc", SelectedFile: 0}
	if shouldAdoptRoomTorrent(magnetOnly, "guest", "host") {
		t.Error("a torrent event with no infohash was adopted")
	}
}
