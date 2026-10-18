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
	seek      *float64
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
		OnSeek: func(seconds float64) {
			e.mu.Lock()
			e.seek = &seconds
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

func (e *roomEvents) seekSeen() (float64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.seek == nil {
		return 0, false
	}
	return *e.seek, true
}

func (e *roomEvents) hostChanged() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.host
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

	meshes, _ := joinIdentifiedMeshes(t, ctx, "host-user", "guest-user")
	host, guest := meshes[0], meshes[1]

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

	// 5. And the recovery is heard too. A guest whose stream caught up must be
	//    able to say so — otherwise the room stays paused for everyone, and it
	//    is stuck on a stall that ended. The concept says exactly this: any
	//    participant may report, not only the host.
	require.NoError(t, guestRoom.ReportBuffering(ctx, false))
	waitFor(t, 30*time.Second, "the host to hear that the guest caught up", func() bool {
		b, ok := hostEvents.bufferingOf(guest.SelfID())
		return ok && !b
	})
}

// joinIdentifiedMeshes puts participants in the same room over a local
// testbed, each with the identity the application gives its mesh: the room
// shows people by that identity, so the greeting has to carry it. The first
// one is the host; the rest are guests.
//
// The meshes are built here rather than borrowed from joinMesh, because that
// helper leaves the identity out — and a room whose peers are nameless is
// exactly what this test exists to catch.
func joinIdentifiedMeshes(t *testing.T, ctx context.Context, users ...string) ([]*Mesh, string) {
	t.Helper()

	inv := testInvite()
	bed := startTestbed(t, ctx, inv)
	bootstrap := addrsToStrings(t, bed.AddrInfo())

	meshes := make([]*Mesh, 0, len(users))
	for i, user := range users {
		mesh := NewMesh(MeshOptions{
			Community: inv.Community,
			Password:  inv.Password,
			Key:       inv.Key,
			Bootstrap: bootstrap,
			UserID:    user,
		})
		if err := mesh.Join(ctx, i == 0); err != nil {
			for _, m := range meshes {
				_ = m.Close()
			}
			_ = mesh.Close()
			t.Fatalf("participant %d did not join: %v", i, err)
		}
		meshes = append(meshes, mesh)
	}
	t.Cleanup(func() {
		for _, m := range meshes {
			_ = m.Close()
		}
	})

	waitFor(t, 30*time.Second, "the participants to meet", func() bool {
		for _, m := range meshes {
			if len(m.Peers()) < len(meshes)-1 {
				return false
			}
		}
		return true
	})
	return meshes, bootstrap
}

// TestAHostLeavingHandsTheRoomToTheSmallestLivingID is the rule the concept
// states: a host that goes away without a transfer hands the role to the
// smallest living peer ID — deterministic, no election, no chatter.
//
// It is tested over a real network with three participants, because the rule
// only means something when the peers know each other: the winner is
// computed from the peer set each side sees, and the announcement that
// follows has to reach the peer that did not win.
func TestAHostLeavingHandsTheRoomToTheSmallestLivingID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	meshes, _ := joinIdentifiedMeshes(t, ctx, "host-user", "guest-one", "guest-two")
	host, guestOne, guestTwo := meshes[0], meshes[1], meshes[2]

	hostEvents := newRoomEvents()
	guestOneEvents := newRoomEvents()
	guestTwoEvents := newRoomEvents()
	hostRoom := NewRoom(host, host.SelfID(), "host-user", true, hostEvents.callbacks())
	guestOneRoom := NewRoom(guestOne, guestOne.SelfID(), "guest-one", false, guestOneEvents.callbacks())
	guestTwoRoom := NewRoom(guestTwo, guestTwo.SelfID(), "guest-two", false, guestTwoEvents.callbacks())

	go hostRoom.Run(ctx)
	go guestOneRoom.Run(ctx)
	go guestTwoRoom.Run(ctx)

	// Both guests know the host before it leaves, or there is nobody to hand
	// the role to.
	waitFor(t, 30*time.Second, "both guests to know the host", func() bool {
		_, hostKnown := guestOneEvents.peerNamed(host.SelfID())
		_, hostKnownToo := guestTwoEvents.peerNamed(host.SelfID())
		return hostKnown && hostKnownToo
	})

	// The host's position reaches both guests: this is what keeps a follower
	// in step, and it only counts if the guest believes the host is the host.
	require.NoError(t, hostRoom.Seek(ctx, 42.5))
	waitFor(t, 30*time.Second, "both guests to receive the host's position", func() bool {
		one, okOne := guestOneEvents.seekSeen()
		two, okTwo := guestTwoEvents.seekSeen()
		return okOne && okTwo && one == 42.5 && two == 42.5
	})

	// The host leaves without handing over.
	require.NoError(t, host.Close())

	// The smallest living peer ID takes the role — and says so, so the other
	// guest converges on the same answer from the same peer set.
	winner := TakeoverHost([]string{guestOne.SelfID(), guestTwo.SelfID()})
	waitFor(t, 30*time.Second, "the guests to agree on the new host", func() bool {
		return guestOneEvents.hostChanged() == winner && guestTwoEvents.hostChanged() == winner
	})

	// And the winner really holds the role.
	winnerRoom := guestOneRoom
	if winner == guestTwo.SelfID() {
		winnerRoom = guestTwoRoom
	}
	if !winnerRoom.IsHost() {
		t.Errorf("the new host %s does not hold the role", winner)
	}
}

// The room's play state is remembered, and a newcomer is told it.
//
// The state travels only when it changes, so a pause that was lost — it
// arrived while the file was still loading, and the load overwrote it — is
// never sent again. The room keeps what it last heard, and a guest that
// joins a paused room must not start playing on its own (bug-0120).
func TestTheRoomRemembersThePlayStateAndTellsANewcomer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	meshes, bootstrap := joinIdentifiedMeshes(t, ctx, "host-user", "guest-user")
	host, guest := meshes[0], meshes[1]

	hostEvents := newRoomEvents()
	guestEvents := newRoomEvents()
	hostRoom := NewRoom(host, host.SelfID(), "host-user", true, hostEvents.callbacks())
	guestRoom := NewRoom(guest, guest.SelfID(), "guest-user", false, guestEvents.callbacks())

	go hostRoom.Run(ctx)
	go guestRoom.Run(ctx)

	// The host pauses: the room remembers, and the guest hears.
	require.NoError(t, hostRoom.Pause(ctx))
	waitFor(t, 30*time.Second, "the guest to be told to pause", func() bool {
		p := guestEvents.pauseSeen()
		return p != nil && *p
	})

	paused, known := hostRoom.PlayState()
	if !known || !paused {
		t.Errorf("the host's room does not remember that the room is paused: %v %v", paused, known)
	}
	paused, known = guestRoom.PlayState()
	if !known || !paused {
		t.Errorf("the guest's room does not remember that the room is paused: %v %v", paused, known)
	}

	// And the guest that joins later is told the state in the answer to its
	// hello: a newcomer that missed the change must not start playing.
	late := NewMesh(MeshOptions{
		Community: testInvite().Community,
		Password:  testInvite().Password,
		Key:       testInvite().Key,
		Bootstrap: bootstrap,
		UserID:    "late-user",
	})
	if err := late.Join(ctx, false); err != nil {
		t.Fatalf("the late guest did not join: %v", err)
	}
	t.Cleanup(func() { _ = late.Close() })

	lateEvents := newRoomEvents()
	lateRoom := NewRoom(late, late.SelfID(), "late-user", false, lateEvents.callbacks())
	go lateRoom.Run(ctx)

	waitFor(t, 30*time.Second, "the late guest to be told the room is paused", func() bool {
		p := lateEvents.pauseSeen()
		return p != nil && *p
	})
}
