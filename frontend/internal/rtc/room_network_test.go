// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The room is the layer that turns a transport into "watching together": the
// roster, the magnet, the host's commands, a peer's report that it is
// buffering. It was only ever driven by a made-up transport, so every one of
// those was proven against a stand-in that the application never uses.
//
// This drives the real thing: two participants, a local testbed, real meshes,
// real rooms — the composition the window builds (window.go: mesh, then
// NewRoom over it). What is asserted here is what a person in a room would
// see: who is in it, what film it is, and whether the host's commands and the
// guest's reports arrive.

// roomEvents collects what a room's callbacks say.
//
// The room speaks in callbacks, and a test needs to wait for one of them: the
// network decides when, not the test.
type roomEvents struct {
	mu        sync.Mutex
	roster    []Peer
	magnet    *Magnet
	paused    *bool
	buffering map[string]bool
	host      string
}

func newRoomEvents() *roomEvents {
	return &roomEvents{buffering: map[string]bool{}}
}

func (e *roomEvents) callbacks() *Callbacks {
	return &Callbacks{
		OnRoster: func(peers []Peer) {
			e.mu.Lock()
			e.roster = peers
			e.mu.Unlock()
		},
		OnMagnet: func(m Magnet) {
			e.mu.Lock()
			cp := m
			e.magnet = &cp
			e.mu.Unlock()
		},
		OnPause: func(paused bool) {
			e.mu.Lock()
			e.paused = &paused
			e.mu.Unlock()
		},
		OnBuffering: func(peerID string, buffering bool) {
			e.mu.Lock()
			e.buffering[peerID] = buffering
			e.mu.Unlock()
		},
		OnHostChanged: func(hostPeerID string) {
			e.mu.Lock()
			e.host = hostPeerID
			e.mu.Unlock()
		},
	}
}

func (e *roomEvents) peerNamed(id string) (Peer, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.roster {
		if p.PeerID == id {
			return p, true
		}
	}
	return Peer{}, false
}

func (e *roomEvents) magnetSeen() *Magnet {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.magnet
}

func (e *roomEvents) pauseSeen() *bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.paused
}

func (e *roomEvents) bufferingOf(id string) (bool, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, ok := e.buffering[id]
	return b, ok
}

// waitFor polls until the condition holds or the test gives up.
//
// The network decides when a message arrives, so a room test cannot assert by
// reading a value right after sending: it has to wait for the value to change.
func waitFor(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", within, what)
}

// TestTwoParticipantsInARoomOverARealNetwork is the whole slice of "watching
// together" in one run: two participants meet through a local testbed, each
// drives a real room over a real mesh, and the four things a room must carry
// arrive — the roster, the magnet, the host's command, and the guest's report
// that it is buffering.
func TestTwoParticipantsInARoomOverARealNetwork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	host, guest := joinIdentifiedMeshes(t, ctx, "host-user", "guest-user")

	hostEvents := newRoomEvents()
	guestEvents := newRoomEvents()
	hostRoom := NewRoom(host, host.SelfID(), "host-user", true, hostEvents.callbacks())
	guestRoom := NewRoom(guest, guest.SelfID(), "guest-user", false, guestEvents.callbacks())

	go hostRoom.Run(ctx)
	go guestRoom.Run(ctx)

	// 1. The roster names who is in the room. A roster of nameless rows is
	//    not a roster: the person reading it cannot tell who they are
	//    watching with.
	waitFor(t, 30*time.Second, "the guest's roster to name the host", func() bool {
		p, ok := guestEvents.peerNamed(host.SelfID())
		return ok && p.UserID != ""
	})

	// 2. The host's magnet reaches the guest.
	magnet := Magnet{
		Envelope:     Envelope{Type: TypeMagnet},
		MagnetURI:    "magnet:?xt=urn:btih:abc",
		InfoHash:     "abc",
		SelectedFile: 0,
		Name:         "Movie.2024.mkv",
	}
	require.NoError(t, hostRoom.AnnounceMagnet(ctx, magnet))
	waitFor(t, 30*time.Second, "the guest to receive the magnet", func() bool {
		m := guestEvents.magnetSeen()
		return m != nil && m.InfoHash == magnet.InfoHash
	})

	// 3. The host's command reaches the guest.
	require.NoError(t, hostRoom.Pause(ctx))
	waitFor(t, 30*time.Second, "the guest to be told to pause", func() bool {
		p := guestEvents.pauseSeen()
		return p != nil && *p
	})

	// 4. The guest's report that it is buffering reaches the host. It is the
	//    host that needs it: the room stops for everyone when someone stalls,
	//    and a host that never heard about the stall would keep playing into
	//    an empty buffer.
	require.NoError(t, guestRoom.ReportBuffering(ctx, true))
	waitFor(t, 30*time.Second, "the host to hear that the guest is buffering", func() bool {
		b, ok := hostEvents.bufferingOf(guest.SelfID())
		return ok && b
	})
}

// joinIdentifiedMeshes puts two participants in the same room over a local
// testbed, each with the identity the application gives its mesh: the room
// shows people by that identity, so the greeting has to carry it.
//
// The meshes are built here rather than borrowed from joinMesh, because that
// helper leaves the identity out — and a room whose peers are nameless is
// exactly what this test exists to catch.
func joinIdentifiedMeshes(t *testing.T, ctx context.Context, hostUser, guestUser string) (*Mesh, *Mesh) {
	t.Helper()

	inv := testInvite()
	bed := startTestbed(t, ctx, inv)
	bootstrap := addrsToStrings(t, bed.AddrInfo())

	host := NewMesh(MeshOptions{
		Community: inv.Community,
		Password:  inv.Password,
		Key:       inv.Key,
		Bootstrap: bootstrap,
		UserID:    hostUser,
	})
	if err := host.Join(ctx, true); err != nil {
		_ = host.Close()
		t.Fatalf("the host did not join: %v", err)
	}

	guest := NewMesh(MeshOptions{
		Community: inv.Community,
		Password:  inv.Password,
		Key:       inv.Key,
		Bootstrap: bootstrap,
		UserID:    guestUser,
	})
	if err := guest.Join(ctx, false); err != nil {
		_ = host.Close()
		_ = guest.Close()
		t.Fatalf("the guest did not join: %v", err)
	}
	t.Cleanup(func() {
		_ = host.Close()
		_ = guest.Close()
	})

	waitFor(t, 30*time.Second, "the two participants to meet", func() bool {
		return len(host.Peers()) > 0 && len(guest.Peers()) > 0
	})
	return host, guest
}
