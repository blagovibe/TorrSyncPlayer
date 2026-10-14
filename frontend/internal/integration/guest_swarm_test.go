// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"context"
	"crypto/sha1"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/adopt"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/room"
)

// The guest's half of a room, against a real swarm.
//
// Every other scenario here stops before this one on purpose: adding a magnet
// means joining a swarm and waiting for metadata, and there was no swarm to join,
// so the wait would hang rather than fail. That left the client's side of "everyone
// watches the same thing" unproven — and the defect it hid was exactly there
// (bug-0078): the room named the host's file, the guest's client wrote it into a
// field, and nothing read it.
//
// What makes this one possible is a tracker: two real backends, a real .torrent
// with real piece hashes, a seeder that actually holds the film, and the guest's
// own client pulling the room's magnet the way the window does.

// TestAGuestEndsUpPlayingWhatTheRoomIsWatching walks the guest's path end to end:
// room, magnet, swarm, and the host's file in the guest's own player.
func TestAGuestEndsUpPlayingWhatTheRoomIsWatching(t *testing.T) {
	announce, tracker := startLoopbackTracker(t)
	payload := swarmPayload(1 << 20)

	// The host is a real backend holding the real film on disk, so it can seed.
	// This is the premise the rest of the design rests on: the host's server is
	// what everyone else pulls from.
	hostData := t.TempDir()
	// The backend puts its torrent data in <data-dir>/torrents, and a single-file
	// torrent lands directly in it by its own name — so that is where the film has
	// to already be for the host to be a seeder at all.
	writeFile(t, filepath.Join(hostData, "torrents", swarmName), payload)
	host := startServerWith(t, "host-peer", hostData, true)

	guest := startServer(t, "guest-peer")

	// The guest's two planes: its own backend for the media, the host's server
	// for the room. They are different processes with different tokens, which is
	// the split the whole architecture is built on.
	guestClient := api.New(guest.BaseURL, guest.Token, host.BaseURL, host.Token)
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

	// 2. The host picks the file it will play and opens the room on it.
	require.Equal(t, 200, host.do(t, "POST", "/api/v1/torrents/"+added.ID+"/select",
		map[string]int{"fileIndex": 0}, nil))

	roomSession := room.NewSession(nil, host.BaseURL, host.Token, "host-peer")
	roomID, err := roomSession.Create(context.Background(), "Movie night", "", added.ID)
	require.NoError(t, err)
	require.Len(t, roomID, 32, "a room id is 32 hex characters")

	// 3. The guest joins over the control plane and waits to be told what the
	// room is watching.
	offered := make(chan room.TorrentEvent, 1)
	stream := room.New(room.Options{
		BaseURL:  host.BaseURL,
		Token:    host.Token,
		ClientID: "guest-peer",
	}, func(e room.Event) {
		if e.Type != room.EventTorrent {
			return
		}
		var ev room.TorrentEvent
		if err := jsonUnmarshal(e.Data, &ev); err == nil {
			select {
			case offered <- ev:
			default:
			}
		}
	})

	guestRoom := room.NewSession(nil, host.BaseURL, host.Token, "guest-peer")
	require.NoError(t, guestRoom.Join(context.Background(), roomID, "", added.ID))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream.Subscribe(ctx, roomID)

	var ev room.TorrentEvent
	select {
	case ev = <-offered:
	case <-time.After(20 * time.Second):
		t.Fatalf("the room never told the guest what it is watching (tracker: %s)", tracker.state())
	}

	require.Equal(t, added.ID, ev.InfoHash, "the room named a different torrent than the one it is bound to")
	require.Contains(t, ev.MagnetURI, "urn:btih:", "the room sent no BitTorrent info hash at all")
	require.Contains(t, ev.MagnetURI, "tr=", "the magnet carries no tracker, so no swarm can answer it")
	assert.Equal(t, 0, ev.SelectedFile, "the host's chosen file did not travel")

	// 4. The guest's client does what the window does: the magnet goes into its
	// own backend. Two minutes, because that is what the window allows and what
	// the backend needs — adding a magnet blocks until the swarm has answered
	// with the metadata, which is not a request that can be hurried.
	var own struct {
		ID string `json:"id"`
	}
	addCtx, addCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer addCancel()
	require.NoError(t, guestClient.Post(addCtx, api.Data, "/api/v1/torrents",
		map[string]string{"magnetUri": ev.MagnetURI}, &own),
		"the guest's own backend refused the room's magnet")
	require.Equal(t, added.ID, own.ID, "the magnet named a different torrent than the room")

	// The guest's own holdings, as the adoption package wants them. Rather than
	// the row-plus-widget shape the window has, because that is what the window
	// adapter is for.
	have := []string{}
	for _, entry := range listTorrents(t, guest) {
		have = append(have, entry.ID)
	}

	// 5. The adoption, driven through the real package the window uses: no
	// fakes anywhere below this line.
	//
	// It has to come *before* any piece can move, and that order is not an
	// accident of the test: anacrolix gives every piece of a fresh torrent
	// PiecePriorityNone, and this project steers priorities from the buffer
	// service, which starts with a selected file. Until the guest picks what the
	// host is watching, it is connected to the swarm and asking it for nothing —
	// the same thing the backend's swarm test got wrong first (idea-0071).
	var played struct {
		id     string
		files  []adopt.File
		chosen adopt.File
	}
	adoptCtx, adoptCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer adoptCancel()

	outcome, err := adopt.CarryOut(adoptCtx, adopt.Offer{
		RoomInfohash: ev.InfoHash,
		HaveTorrents: have,
		HostFile:     ev.SelectedFile,
	}, func(ctx context.Context, torrentID string) ([]adopt.File, error) {
		var listed struct {
			Files []adopt.File `json:"files"`
		}
		if err := guestClient.Get(ctx, api.Data, "/api/v1/torrents/"+torrentID+"/files", &listed); err != nil {
			return nil, err
		}
		return listed.Files, nil
	}, func(id string, files []adopt.File, chosen adopt.File) {
		played.id, played.files, played.chosen = id, files, chosen

		// Exactly what the window's playFile does before starting mpv: the file
		// is chosen per-peer on the guest's own data plane. Without it nothing is
		// wanted, and a torrent nobody asks for does not download.
		require.Equal(t, 200, guest.do(t, "POST", "/api/v1/torrents/"+id+"/select",
			map[string]int{"fileIndex": chosen.Index}, nil),
			"the guest's own backend refused the file the host chose")
	})

	require.NoError(t, err)
	require.Equal(t, adopt.Adopted, outcome, "the guest would not play what the room is watching")
	assert.Equal(t, added.ID, played.id, "the player would open a different torrent")
	assert.Equal(t, 0, played.chosen.Index, "the guest would play a file the host never chose")
	assert.NotEmpty(t, played.chosen.Name, "the chosen file has no name to show")

	// 6. And the pieces actually move: the guest's own backend pulls them from
	// the host's through the tracker, on the guest's own data plane.
	waitForProgress(t, guest, added.ID, 0.0001, 60*time.Second,
		"the guest pulled nothing from the swarm after choosing the host's file")

	// 7. And the host's list agrees with what the guest was handed: same torrent,
	// same file index, which is the only way the two of them watch one thing.
	hostFiles := torrentFiles(t, host, added.ID)
	require.NotEmpty(t, hostFiles, "the host has no files to compare against")
	assert.Equal(t, hostFiles[0].Name, played.chosen.Name, "the two backends disagree about the file")
}

// swarmPayload is deterministic so a mismatch is never "different random bytes".
func swarmPayload(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(i*31 + i/251)
	}
	return out
}

const swarmName = "film.mkv"

// buildSeededTorrent makes a .torrent whose pieces really hash to the payload.
//
// The other builder in this package fills "pieces" with zeroes, which is fine for
// an identity test and useless for a swarm: a seeder would have nothing verifiable
// to hand out and a leecher would reject every byte it received.
func buildSeededTorrent(t *testing.T, announce string, payload []byte) []byte {
	t.Helper()

	const pieceLength = 16384
	hashes := make([]byte, 0, ((len(payload)+pieceLength-1)/pieceLength)*20)
	for off := 0; off < len(payload); off += pieceLength {
		end := off + pieceLength
		if end > len(payload) {
			end = len(payload)
		}
		sum := sha1.Sum(payload[off:end])
		hashes = append(hashes, sum[:]...)
	}

	info := map[string]interface{}{
		"name":         swarmName,
		"piece length": pieceLength,
		"pieces":       string(hashes),
		"length":       int64(len(payload)),
	}
	return bencode(t, map[string]interface{}{"announce": announce, "info": info})
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

type torrentEntry struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
}

// listTorrents is the guest's own view of what its backend holds.
//
// A 429 is not a failure here: the protected group allows one request per second
// with a burst of ten, and a poll that asks faster than that gets told to slow
// down rather than answered. Returning nothing makes the caller ask again, which
// is what waiting means.
func listTorrents(t *testing.T, srv *server) []torrentEntry {
	t.Helper()

	var out struct {
		Torrents []torrentEntry `json:"torrents"`
	}
	if code := srv.do(t, "GET", "/api/v1/torrents", nil, &out); code != 200 && code != 429 {
		t.Fatalf("listing the torrents answered %d", code)
	}
	return out.Torrents
}

// torrentFiles is the file list a backend reports for one torrent.
func torrentFiles(t *testing.T, srv *server, torrentID string) []adopt.File {
	t.Helper()

	var listed struct {
		Files []adopt.File `json:"files"`
	}
	require.Equal(t, 200, srv.do(t, "GET", "/api/v1/torrents/"+torrentID+"/files", nil, &listed))
	return listed.Files
}

// waitForProgress blocks until a torrent on srv has reached at least want.
//
// Progress rather than presence, because for the guest "the torrent is listed" and
// "the torrent is arriving" are different facts, and only the second one proves
// the swarm worked. The backend reports progress as a fraction of the total size.
func waitForProgress(t *testing.T, srv *server, torrentID string, want float64, within time.Duration, what string) float64 {
	t.Helper()

	deadline := time.Now().Add(within)
	for {
		for _, entry := range listTorrents(t, srv) {
			if entry.ID == torrentID && entry.Progress >= want {
				return entry.Progress
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s (torrent %s, wanted progress %.4f)", what, torrentID[:8], want)
		}
		// Just over a second: the protected group allows one request per second,
		// and a tighter loop spends its time being told 429.
		time.Sleep(1100 * time.Millisecond)
	}
}
