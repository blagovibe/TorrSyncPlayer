// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
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
	event := rtc.Magnet{
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
	hashOnly := rtc.Magnet{InfoHash: "abc", SelectedFile: 0}
	if shouldAdoptRoomTorrent(hashOnly, "guest", "host") {
		t.Error("a torrent event with no magnet was adopted")
	}

	magnetOnly := rtc.Magnet{MagnetURI: "magnet:?xt=urn:btih:abc", SelectedFile: 0}
	if shouldAdoptRoomTorrent(magnetOnly, "guest", "host") {
		t.Error("a torrent event with no infohash was adopted")
	}
}

// TestTheHostAnnouncesItsOwnFilm pins what onCreatePressed needs before it
// opens the mesh: the magnet of the selected torrent, from this client's own
// backend, with this peer's own file choice attached. An infohash alone does
// not carry a torrent; a magnet without trackers resolves over DHT and drops
// every seeder.
func TestTheHostAnnouncesItsOwnFilm(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/magnet"):
			_, _ = w.Write([]byte(`{"magnetUri":"magnet:?xt=urn:btih:abc&tr=http://tracker.example/announce"}`))
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = w.Write([]byte(`{"files":[{"index":0,"name":"Film.mkv","size":100}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	w := &window{
		client:       api.New(server.URL, "token", server.URL, "token"),
		current:      &Torrent{ID: "abc", Name: "Film"},
		selectedFile: 0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	magnet, err := w.roomMagnet(ctx, "abc")
	if err != nil {
		t.Fatalf("the host could not magnetize its own film: %v", err)
	}
	if magnet.MagnetURI != "magnet:?xt=urn:btih:abc&tr=http://tracker.example/announce" {
		t.Errorf("the magnet lost its trackers: %q", magnet.MagnetURI)
	}
	if magnet.InfoHash != "abc" {
		t.Errorf("the magnet names %q, not the selected torrent", magnet.InfoHash)
	}
	if magnet.SelectedFile != 0 {
		t.Errorf("the announcement names file %d, not the selected one", magnet.SelectedFile)
	}
}
