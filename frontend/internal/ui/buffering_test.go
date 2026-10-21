// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The goal says: "Buffering is not a command but a report: the room stops, it
// does not lead."
//
// These tests pin the half that was missing: the report now stops the room.
// The report travels (the rtc package's own tests pin that), and the host —
// the one who owns the room's play state — stops the playback for everyone
// and lets it go again when the one who was behind is back.
//
// What is deliberately not here: a guest that hears the report does not stop
// anything. A report is not a command, and the room's play state is the
// host's.

// roomTransport is a transport that keeps what the room sent and hands it
// what the test feeds it, so the room can be driven the way the network
// drives it.
type roomTransport struct {
	mu       sync.Mutex
	self     string
	peers    []string
	incoming chan rtcRoomMessage
	onSend   func(msg any)
	closed   bool
}

// rtcRoomMessage is one message arriving from a peer.
type rtcRoomMessage struct {
	from string
	msg  any
}

func (t *roomTransport) Send(_ context.Context, msg any) error {
	t.mu.Lock()
	onSend := t.onSend
	t.mu.Unlock()
	if onSend != nil {
		onSend(msg)
	}
	return nil
}

func (t *roomTransport) Next(ctx context.Context) (string, any, bool) {
	select {
	case in := <-t.incoming:
		return in.from, in.msg, true
	case <-ctx.Done():
		return "", nil, false
	}
}

func (t *roomTransport) Peers() []string           { return t.peers }
func (t *roomTransport) SelfID() string            { return t.self }
func (t *roomTransport) DialHint(_ []rtc.PeerAddr) {}
func (t *roomTransport) Close() error              { return nil }

// feed hands a message to the room, as the network would.
func (t *roomTransport) feed(from string, msg any) {
	t.incoming <- rtcRoomMessage{from: from, msg: msg}
}

// hostWindow builds a host's window with a real player and a real room, the
// way the application builds them: the room over the transport, the window's
// callbacks on the room.
func hostWindow(t *testing.T, label, media string, isHost bool) (*window, *roomTransport) {
	t.Helper()

	_ = test.NewApp()

	p := startFollowerPlayer(t, label, media)
	tr := &roomTransport{self: "self", peers: []string{"guest-peer"}, incoming: make(chan rtcRoomMessage, 16)}

	w := &window{mpv: p, roomLabel: widget.NewLabel("")}
	w.rtcRoom = rtc.NewRoom(tr, "self", "self-user", isHost, w.roomCallbacks())

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go w.rtcRoom.Run(ctx)

	// The guest says hello, so it is in the room's roster — a peer that is
	// not in the roster cannot be behind as far as the room is concerned.
	tr.feed("guest-peer", rtc.NewHello("guest-peer", "guest-user", "guest", false, nil))
	waitFor(t, followerWait, "the guest to join the room", func() bool {
		return len(w.rtcRoom.Peers()) == 1
	})

	return w, tr
}

// TestTheRoomStopsForTheOneWhoIsBehind is the promise, end to end: a guest
// reports that it ran out of data, and the host stops the room.
func TestTheRoomStopsForTheOneWhoIsBehind(t *testing.T) {
	if !mpvOnPath(t) {
		t.Skip("no mpv on PATH")
	}
	media := followerMedia(t)
	w, tr := hostWindow(t, "buffering", media, true)

	ctx, cancel := context.WithTimeout(context.Background(), followerWait*2)
	defer cancel()

	// The file has to be open before the room means anything.
	waitFor(t, followerWait, "the file to be open", func() bool {
		tracks, err := w.mpv.Tracks(ctx, player.TrackVideo)
		return err == nil && len(tracks) > 0
	})

	// A guest reports that it is behind.
	tr.feed("guest-peer", rtc.NewBuffering(true))

	waitFor(t, followerWait, "the room to stop", func() bool {
		paused, err := w.mpv.Paused(ctx)
		return err == nil && paused
	})
}

// TestTheRoomIsReleasedWhenTheOneWhoIsBehindIsBack is the other half: the
// room does not stay stopped forever.
func TestTheRoomIsReleasedWhenTheOneWhoIsBehindIsBack(t *testing.T) {
	if !mpvOnPath(t) {
		t.Skip("no mpv on PATH")
	}
	media := followerMedia(t)
	w, tr := hostWindow(t, "buffering-back", media, true)

	ctx, cancel := context.WithTimeout(context.Background(), followerWait*2)
	defer cancel()

	waitFor(t, followerWait, "the file to be open", func() bool {
		tracks, err := w.mpv.Tracks(ctx, player.TrackVideo)
		return err == nil && len(tracks) > 0
	})

	tr.feed("guest-peer", rtc.NewBuffering(true))
	waitFor(t, followerWait, "the room to stop", func() bool {
		paused, err := w.mpv.Paused(ctx)
		return err == nil && paused
	})

	tr.feed("guest-peer", rtc.NewBuffering(false))

	waitFor(t, followerWait, "the room to play again", func() bool {
		paused, err := w.mpv.Paused(ctx)
		return err == nil && !paused
	})
}

// TestAGuestDoesNotStopTheRoom is the other half of "a report is not a
// command": the room's play state is the host's, and a guest that heard the
// report does not stop anything by itself.
func TestAGuestDoesNotStopTheRoom(t *testing.T) {
	if !mpvOnPath(t) {
		t.Skip("no mpv on PATH")
	}
	media := followerMedia(t)
	w, tr := hostWindow(t, "buffering-guest", media, false)

	ctx, cancel := context.WithTimeout(context.Background(), followerWait*2)
	defer cancel()

	waitFor(t, followerWait, "the file to be open", func() bool {
		tracks, err := w.mpv.Tracks(ctx, player.TrackVideo)
		return err == nil && len(tracks) > 0
	})

	tr.feed("host-peer", rtc.NewBuffering(true))

	// The guest's player must not stop: a report is not a command.
	time.Sleep(2 * time.Second)
	paused, err := w.mpv.Paused(ctx)
	require.NoError(t, err)
	assert.False(t, paused, "a guest must not stop the room for the host")
}
