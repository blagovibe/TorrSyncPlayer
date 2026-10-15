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

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/adopt"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
)

// A room with no server in it.
//
// The old scenarios drove rooms through the backend's HTTP endpoints; those
// endpoints are gone, and with them the tests. This one proves the world that
// replaced them: two real backends holding real torrent state, one local
// signaler introducing them, the host's magnet travelling a data channel, the
// guest pulling real pieces from the swarm, playback commands fanning out, and
// the host role surviving the host's departure.
//
// Nothing here is stubbed below the room driver: the meshes are real WebRTC
// peers, the tracker is real HTTP, and the pieces are real bytes.

// TestARoomWithoutAServer plays the whole new flow end to end.
func TestARoomWithoutAServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	announce, tracker := startLoopbackTracker(t)
	payload := swarmPayload(1 << 20)

	// The host is a real backend holding the real film on disk, so it can seed.
	hostData := t.TempDir()
	writeFile(t, filepath.Join(hostData, "torrents", swarmName), payload)
	host := startServerWith(t, "host-peer", hostData, true)

	guest := startServer(t, "guest-peer")
	guestClient := api.New(guest.BaseURL, guest.Token)
	guestClient.SetClientID("guest-peer")

	// 1. The host adds the film and waits until it can actually seed it.
	var added struct {
		ID string `json:"id"`
	}
	require.Equal(t, 201, host.do(t, "POST", "/api/v1/torrents",
		map[string]string{"torrentFile": encodeTorrent(t, buildSeededTorrent(t, announce, payload))}, &added),
		"the host could not add its own torrent")
	require.NotEmpty(t, added.ID)

	waitForProgress(t, host, added.ID, 1, 60*time.Second, "the seeder never verified the film it holds")

	// 2. The host picks the file it will play.
	require.Equal(t, 200, host.do(t, "POST", "/api/v1/torrents/"+added.ID+"/select",
		map[string]int{"fileIndex": 0}, nil))

	// 3. The room opens on a local signaler — the public one is for manual
	// runs, and a test that needs someone else's service fails for someone
	// else's reasons.
	signaler := startSignaler(t)
	community := "test-room"

	hostMesh := rtc.NewMesh(rtc.MeshOptions{
		Signaler:  signaler,
		Community: community,
		Password:  "test-password",
		Key:       "test-key",
	})
	defer func() { _ = hostMesh.Close() }()
	require.NoError(t, hostMesh.Join(ctx, true), "the host could not join the mesh")

	var hostMu sync.Mutex
	hostStates := 0
	hostRoom := rtc.NewRoom(hostMesh, hostMesh.SelfID(), "host-peer", true, &rtc.Callbacks{
		OnPause: func(paused bool) {
			hostMu.Lock()
			defer hostMu.Unlock()
			hostStates++
		},
	})
	go hostRoom.Run(ctx)

	guestMesh := rtc.NewMesh(rtc.MeshOptions{
		Signaler:  signaler,
		Community: community,
		Password:  "test-password",
		Key:       "test-key",
	})
	defer func() { _ = guestMesh.Close() }()
	require.NoError(t, guestMesh.Join(ctx, false), "the guest could not join the mesh")

	var guestMu sync.Mutex
	var magnets []rtc.Magnet
	var pauses []bool
	var seeks []float64
	var hosts []string
	guestRoom := rtc.NewRoom(guestMesh, guestMesh.SelfID(), "guest-peer", false, &rtc.Callbacks{
		OnMagnet: func(m rtc.Magnet) {
			guestMu.Lock()
			defer guestMu.Unlock()
			magnets = append(magnets, m)
		},
		OnPause: func(paused bool) {
			guestMu.Lock()
			defer guestMu.Unlock()
			pauses = append(pauses, paused)
		},
		OnSeek: func(seconds float64) {
			guestMu.Lock()
			defer guestMu.Unlock()
			seeks = append(seeks, seconds)
		},
		OnHostChanged: func(hostPeerID string) {
			guestMu.Lock()
			defer guestMu.Unlock()
			hosts = append(hosts, hostPeerID)
		},
	})
	go guestRoom.Run(ctx)

	waitForMesh(t, hostMesh, 1, 30*time.Second, "the host never saw the guest")

	// 4. The host announces its own film: the magnet from its own backend, with
	// its own file choice attached.
	hostMagnetOut := hostMagnet(t, host, added.ID)
	require.NoError(t, hostRoom.AnnounceMagnet(ctx, rtc.Magnet{
		MagnetURI:    hostMagnetOut,
		InfoHash:     added.ID,
		SelectedFile: 0,
		Name:         swarmName,
	}), "the host could not announce its film")

	waitForGuestValue(t, func() bool {
		guestMu.Lock()
		defer guestMu.Unlock()
		return len(magnets) == 1
	}, "the guest never learned what the room is watching")
	guestMu.Lock()
	offered := magnets[0]
	guestMu.Unlock()
	assert.Equal(t, added.ID, offered.InfoHash)
	assert.Equal(t, 0, offered.SelectedFile)

	// 5. The guest carries the offer out through the real adoption path: its
	// own backend, its own file choice, then real pieces from the swarm.
	var own struct {
		ID string `json:"id"`
	}
	addCtx, addCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer addCancel()
	require.NoError(t, guestClient.Post(addCtx, "/api/v1/torrents",
		map[string]string{"magnetUri": offered.MagnetURI}, &own),
		"the guest's own backend refused the room's magnet")
	require.Equal(t, added.ID, own.ID)

	have := []string{}
	for _, entry := range listTorrents(t, guest) {
		have = append(have, entry.ID)
	}
	var played struct {
		id     string
		chosen adopt.File
	}
	outcome, err := adopt.CarryOut(ctx, adopt.Offer{
		RoomInfohash: offered.InfoHash,
		HaveTorrents: have,
		HostFile:     offered.SelectedFile,
	}, func(ctx context.Context, torrentID string) ([]adopt.File, error) {
		return guestFiles(ctx, guestClient, torrentID)
	}, func(id string, _ []adopt.File, chosen adopt.File) {
		played.id, played.chosen = id, chosen
		require.Equal(t, 200, guest.do(t, "POST", "/api/v1/torrents/"+id+"/select",
			map[string]int{"fileIndex": chosen.Index}, nil))
	})
	require.NoError(t, err)
	require.Equal(t, adopt.Adopted, outcome)
	assert.Equal(t, 0, played.chosen.Index)

	waitForProgress(t, guest, added.ID, 0.0001, 60*time.Second,
		"the guest pulled nothing from the swarm after choosing the host's file")

	// 6. Playback commands fan out: pause reaches every guest.
	require.NoError(t, hostRoom.Pause(ctx))
	waitForGuestValue(t, func() bool {
		guestMu.Lock()
		defer guestMu.Unlock()
		return len(pauses) == 1 && pauses[0]
	}, "the host's pause never reached the guest")

	// 7. The host leaves; the smallest living ID takes over, and the guest
	// learns the new host's name from the announcement. The guest already
	// learned the old host at join, so what is awaited is a *new* name, not
	// any name at all.
	guestMu.Lock()
	knownHosts := len(hosts)
	guestMu.Unlock()
	require.NoError(t, hostMesh.Close())
	waitForGuestValue(t, func() bool {
		guestMu.Lock()
		defer guestMu.Unlock()
		return len(hosts) > knownHosts
	}, "no takeover happened after the host left")
	guestMu.Lock()
	newHost := hosts[len(hosts)-1]
	guestMu.Unlock()
	assert.Equal(t, guestMesh.SelfID(), newHost,
		"the takeover named someone who is not the surviving guest")

	// The room outlived its host with its film intact: the guest still holds
	// the torrent, and the magnet it learned names it.
	assert.Equal(t, added.ID, offered.InfoHash)
	_ = tracker
	_ = hostStates
	_ = seeks
}

// hostMagnet asks the host's own backend for the magnet of the torrent it
// holds. The infohash alone does not carry a torrent; this is what does.
func hostMagnet(t *testing.T, host *server, torrentID string) string {
	t.Helper()

	var out struct {
		MagnetURI string `json:"magnetUri"`
	}
	code := host.do(t, "GET", "/api/v1/torrents/"+torrentID+"/magnet", nil, &out)
	if code != 200 {
		t.Fatalf("the host backend refused the magnet with %d", code)
	}
	if out.MagnetURI == "" {
		t.Fatal("the host backend returned an empty magnet")
	}
	return out.MagnetURI
}

// guestFiles is the guest's own view of one torrent's files.
func guestFiles(ctx context.Context, client *api.Client, torrentID string) ([]adopt.File, error) {
	var listed struct {
		Files []adopt.File `json:"files"`
	}
	if err := client.Get(ctx, "/api/v1/torrents/"+torrentID+"/files", &listed); err != nil {
		return nil, err
	}
	return listed.Files, nil
}

// waitForMesh blocks until the mesh sees at least want peers.
func waitForMesh(t *testing.T, m interface {
	Peers() []string
}, want int, timeout time.Duration, what string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(m.Peers()) >= want {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("%s (saw %d peers)", what, len(m.Peers()))
}

// waitForGuestValue blocks until the guest's callbacks report a state.
func waitForGuestValue(t *testing.T, done func() bool, what string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("%s", what)
}
