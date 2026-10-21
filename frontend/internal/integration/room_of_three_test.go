// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
)

// A room of two is a pair with a protocol. A room of three is a room.
//
// Everything the room promises is decided by three or more: the mesh has to
// grow past the star that a host and one guest make, the address book has to
// carry the third participant to the others, the host's commands have to
// reach everyone, a report from somebody who is not the host has to travel,
// and the role after the host leaves has to be decided among the survivors.
//
// All of it here is the real thing: real backends holding a real film, a
// real tracker, real meshes over real libp2p streams, and the room driver the
// application uses.

// TestARoomOfThreeIsARoomAndNotAPair is the room with more than two people in
// it.
func TestARoomOfThreeIsARoomAndNotAPair(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	announce, tracker := startLoopbackTracker(t)
	payload := swarmPayload(1 << 20)

	// The host is a real backend holding the real film, so it can seed.
	hostData := t.TempDir()
	writeFile(t, filepath.Join(hostData, "torrents", swarmName), payload)
	host := startServerWith(t, "host-peer", hostData, true)

	guestOne := startServer(t, "guest-one")
	guestTwo := startServer(t, "guest-two")

	// The film, in the host's own hands.
	var added struct {
		ID string `json:"id"`
	}
	require.Equal(t, 201, host.do(t, "POST", "/api/v1/torrents",
		map[string]string{"torrentFile": encodeTorrent(t, buildSeededTorrent(t, announce, payload))}, &added),
		"the host could not add its own torrent")
	waitForProgress(t, host, added.ID, 1, 60*time.Second, "the seeder never verified the film it holds")
	require.Equal(t, 200, host.do(t, "POST", "/api/v1/torrents/"+added.ID+"/select",
		map[string]int{"fileIndex": 0}, nil))

	// The room: three participants, each with its own backend and its own
	// mesh. The guests come by the address the code carried, the way a guest
	// comes in the product.
	inv := rtc.Invite{Community: "three-room", Password: "three-password", Key: "three-key"}
	bootstrap := startTestbed(t, inv)

	hostEvents := newRoomEvents()
	hostMesh := rtc.NewMesh(rtc.MeshOptions{
		Community: inv.Community,
		Password:  inv.Password,
		Key:       inv.Key,
		Bootstrap: bootstrap,
		UserID:    "host-peer",
	})
	defer func() { _ = hostMesh.Close() }()
	require.NoError(t, hostMesh.Join(ctx, true), "the host could not join")
	hostRoom := rtc.NewRoom(hostMesh, hostMesh.SelfID(), "host-peer", true, hostEvents.callbacks())
	go hostRoom.Run(ctx)

	// The first guest comes by the address.
	guestOneEvents := newRoomEvents()
	guestOneMesh := rtc.NewMesh(rtc.MeshOptions{
		Community: inv.Community,
		Password:  inv.Password,
		Key:       inv.Key,
		Bootstrap: bootstrap,
		UserID:    "guest-one",
		Addrs:     hostMesh.Addrs(),
	})
	defer func() { _ = guestOneMesh.Close() }()
	require.NoError(t, guestOneMesh.Join(ctx, false), "the first guest could not join")
	guestOneRoom := rtc.NewRoom(guestOneMesh, guestOneMesh.SelfID(), "guest-one", false, guestOneEvents.callbacks())
	go guestOneRoom.Run(ctx)

	// And the second, the same way. The room is already two people by now,
	// so the address book has something to carry.
	guestTwoEvents := newRoomEvents()
	guestTwoMesh := rtc.NewMesh(rtc.MeshOptions{
		Community: inv.Community,
		Password:  inv.Password,
		Key:       inv.Key,
		Bootstrap: bootstrap,
		UserID:    "guest-two",
		Addrs:     hostMesh.Addrs(),
	})
	defer func() { _ = guestTwoMesh.Close() }()
	require.NoError(t, guestTwoMesh.Join(ctx, false), "the second guest could not join")
	guestTwoRoom := rtc.NewRoom(guestTwoMesh, guestTwoMesh.SelfID(), "guest-two", false, guestTwoEvents.callbacks())
	go guestTwoRoom.Run(ctx)

	// 1. The mesh is a mesh: the two guests reach each other, not only the
	// host. With a host and one guest this is not even a question — there
	// is nobody else to reach.
	waitForMesh(t, guestOneMesh, 2, 60*time.Second, "the first guest never reached the second")
	waitForMesh(t, guestTwoMesh, 2, 60*time.Second, "the second guest never reached the first")

	guestOnePeers := guestOneMesh.Peers()
	assert.Contains(t, guestOnePeers, guestTwoMesh.SelfID(),
		"the first guest does not see the second: the room is a star, not a mesh")
	assert.Contains(t, guestOnePeers, hostMesh.SelfID(),
		"the first guest does not see the host")

	// And the room knows: the roster of each guest names the other one.
	waitForGuestValue(t, func() bool {
		return guestOneEvents.rosterHas(guestTwoMesh.SelfID())
	}, "the first guest's roster never named the second")

	// 2. The host announces its film, and both guests learn what the room is
	// watching. The magnet is the room's, and the room has three people in
	// it now.
	magnet := rtc.Magnet{
		MagnetURI:    hostMagnet(t, host, added.ID),
		InfoHash:     added.ID,
		SelectedFile: 0,
		Name:         swarmName,
	}
	require.NoError(t, hostRoom.AnnounceMagnet(ctx, magnet), "the host could not announce its film")

	waitForGuestValue(t, func() bool { return guestOneEvents.magnetCount() == 1 },
		"the first guest never learned what the room is watching")
	waitForGuestValue(t, func() bool { return guestTwoEvents.magnetCount() == 1 },
		"the second guest never learned what the room is watching")

	// 3. Both guests carry the offer out through the real path: their own
	// backends, their own file choice, and real pieces from the swarm.
	for _, guest := range []struct {
		name   string
		server *server
	}{
		{"the first guest", guestOne},
		{"the second guest", guestTwo},
	} {
		var own struct {
			ID string `json:"id"`
		}
		require.Equal(t, 201, guest.server.do(t, "POST", "/api/v1/torrents",
			map[string]string{"magnetUri": magnet.MagnetURI}, &own),
			"%s's own backend refused the room's magnet", guest.name)
		require.Equal(t, added.ID, own.ID, "%s got a different film", guest.name)

		// The guest chooses the file the host is playing. Without a choice
		// the prioritizer has nothing to prioritize, and the guest waits
		// for data it never asked for.
		require.Equal(t, 200, guest.server.do(t, "POST", "/api/v1/torrents/"+own.ID+"/select",
			map[string]int{"fileIndex": 0}, nil),
			"%s could not choose the file the room is watching", guest.name)

		waitForProgress(t, guest.server, added.ID, 0.0001, 60*time.Second,
			guest.name+" pulled nothing from the swarm")
	}

	// 4. The host's commands reach everyone: a pause is not a message to
	// the one who happens to be next.
	require.NoError(t, hostRoom.Pause(ctx))

	waitForGuestValue(t, func() bool { return guestOneEvents.pauseCount() == 1 },
		"the host's pause never reached the first guest")
	waitForGuestValue(t, func() bool { return guestTwoEvents.pauseCount() == 1 },
		"the host's pause never reached the second guest")

	// 5. A report from somebody who is not the host travels to the whole
	// room. It is not a command — the room's play state is the host's — but
	// everybody is told who is behind.
	require.NoError(t, guestOneRoom.ReportBuffering(ctx, true))

	waitForGuestValue(t, func() bool { return guestTwoEvents.bufferingCount() == 1 },
		"the first guest's report never reached the second")
	waitForGuestValue(t, func() bool { return hostEvents.bufferingCount() == 1 },
		"the first guest's report never reached the host")

	// 6. The host leaves, and the role goes to the smallest living peer ID
	// among the survivors — and both survivors agree on who it is. The rule
	// is deterministic, so the answer cannot be "whoever spoke first".
	knownHosts := len(guestOneEvents.hosts) + len(guestTwoEvents.hosts)
	require.NoError(t, hostMesh.Close())

	waitForGuestValue(t, func() bool {
		return len(guestOneEvents.hosts)+len(guestTwoEvents.hosts) > knownHosts
	}, "no takeover happened after the host left")

	survivors := []string{guestOneMesh.SelfID(), guestTwoMesh.SelfID()}
	newHost := survivors[0]
	if survivors[1] < survivors[0] {
		newHost = survivors[1]
	}
	assert.Equal(t, newHost, guestOneEvents.lastHost(),
		"the first guest names a different new host")
	assert.Equal(t, newHost, guestTwoEvents.lastHost(),
		"the second guest names a different new host")

	_ = tracker
}

// roomEvents is what one participant's room reported.
//
// The room speaks in callbacks, and a test has to wait for one of them: the
// network decides when a message arrives, not the test.
type roomEvents struct {
	mu        sync.Mutex
	roster    []rtc.Peer
	magnets   int
	pauses    int
	buffering int
	hosts     []string
}

func newRoomEvents() *roomEvents { return &roomEvents{} }

func (e *roomEvents) callbacks() *rtc.Callbacks {
	return &rtc.Callbacks{
		OnRoster: func(peers []rtc.Peer) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.roster = peers
		},
		OnMagnet: func(rtc.Magnet) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.magnets++
		},
		OnPause: func(bool) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.pauses++
		},
		OnBuffering: func(string, bool) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.buffering++
		},
		OnHostChanged: func(hostPeerID string) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.hosts = append(e.hosts, hostPeerID)
		},
	}
}

func (e *roomEvents) magnetCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.magnets
}

func (e *roomEvents) pauseCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.pauses
}

func (e *roomEvents) bufferingCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.buffering
}

func (e *roomEvents) lastHost() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.hosts) == 0 {
		return ""
	}
	return e.hosts[len(e.hosts)-1]
}

// rosterHas says whether the last roster names a peer.
func (e *roomEvents) rosterHas(peerID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.roster {
		if p.PeerID == peerID {
			return true
		}
	}
	return false
}
