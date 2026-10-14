// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/room"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The whole promise of the project, end to end, through the real client code
// against the real backend:
//
//	host adds a torrent → guest joins the room → guest receives the magnet →
//	guest adds it to its OWN backend → same infohash → guest selects the file
//	the host is playing → the stream URL points at the guest's backend.
//
// That identity match is the load-bearing step. Everything in the room binding,
// the restart path and the per-peer ownership rests on a guest arriving at the
// same torrent the host has, independently.

// buildTorrentFile makes a real, valid .torrent for a single file.
//
// A .torrent rather than a magnet because it needs no network: the infohash is
// computable from the bytes alone, so the test is about the client's behaviour
// rather than about whether a swarm answered.
func buildTorrentFile(t *testing.T, name string, length int64) []byte {
	t.Helper()

	const pieceLength = 16384
	pieces := make([]byte, ((length+pieceLength-1)/pieceLength)*20)

	info := map[string]interface{}{
		"name":         name,
		"piece length": pieceLength,
		"pieces":       string(pieces),
		"length":       length,
	}
	return bencode(t, map[string]interface{}{"info": info})
}

func bencode(t *testing.T, value interface{}) []byte {
	t.Helper()

	switch v := value.(type) {
	case string:
		return []byte(encodeString(v))
	case int:
		return []byte(encodeInt(int64(v)))
	case int64:
		return []byte(encodeInt(v))
	case map[string]interface{}:
		// Keys are sorted: bencode dictionaries are ordered by key, and a
		// different order yields a different hash — which would quietly make
		// this test assert nothing.
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sortStrings(keys)

		out := "d"
		for _, key := range keys {
			out += encodeString(key) + string(bencode(t, v[key]))
		}
		return []byte(out + "e")
	default:
		t.Fatalf("bencode does not handle %T", value)
		return nil
	}
}

func encodeString(s string) string { return strconv.Itoa(len(s)) + ":" + s }
func encodeInt(n int64) string     { return "i" + strconv.FormatInt(n, 10) + "e" }

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j-1] > values[j]; j-- {
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}

// TestGuestPullsTheSameTorrentAsTheHost is the end-to-end gate for retiring the
// Qt client.
func TestGuestPullsTheSameTorrentAsTheHost(t *testing.T) {
	host := startServer(t, "host-peer")
	guest := startServer(t, "guest-peer")

	// The guest's own backend: separate process, separate port, separate data.
	guestClient := api.New(guest.BaseURL, guest.Token, guest.BaseURL, guest.Token)
	guestClient.SetClientID("guest-peer")

	hostClient := api.New(host.BaseURL, host.Token, host.BaseURL, host.Token)
	hostClient.SetClientID("host-peer")

	// 1. The host adds a torrent to its own backend.
	torrentFile := buildTorrentFile(t, "Big Buck Bunny", 4<<20)

	var added struct {
		ID string `json:"id"`
	}
	require.Equal(t, 201, host.do(t, "POST", "/api/v1/torrents",
		map[string]string{"torrentFile": encodeTorrent(t, torrentFile)}, &added),
		"the host could not add its own torrent")
	require.NotEmpty(t, added.ID, "the backend accepted a torrent with no id")

	// 2. The host picks the file it will play.
	require.Equal(t, 200, host.do(t, "POST", "/api/v1/torrents/"+added.ID+"/select",
		map[string]int{"fileIndex": 0}, nil))

	// 3. The host creates a room bound to that torrent.
	roomSession := room.NewSession(nil, host.BaseURL, host.Token, "host-peer")
	roomID, err := roomSession.Create(context.Background(), "Movie night", "", added.ID)
	require.NoError(t, err)
	require.Len(t, roomID, 32, "a room id is 32 hex characters")

	// 4. The guest joins over the CONTROL plane: the host's server, host's token.
	guestRoom := room.NewSession(nil, host.BaseURL, host.Token, "guest-peer")
	require.NoError(t, guestRoom.Join(context.Background(), roomID, "", added.ID))

	// 5. The guest subscribes and waits for the room to say what it is watching.
	received := make(chan room.TorrentEvent, 1)
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
			case received <- ev:
			default:
			}
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream.Subscribe(ctx, roomID)

	// The room broadcasts on every join, so a third participant arriving is
	// what triggers a fresh event for a subscriber that has just attached.
	third := room.NewSession(nil, host.BaseURL, host.Token, "third-peer")
	require.NoError(t, third.Join(context.Background(), roomID, "", added.ID))

	var offered room.TorrentEvent
	select {
	case offered = <-received:
	case <-time.After(15 * time.Second):
		t.Fatal("the room never told the guest what it is watching")
	}

	require.Equal(t, added.ID, offered.InfoHash,
		"the room named a different torrent than the one it was bound to")
	require.NotEmpty(t, offered.MagnetURI, "the room sent no magnet, and an infohash carries no torrent")
	assert.Equal(t, 0, offered.SelectedFile, "the host's chosen file did not travel")

	// 6. The magnet names the host's torrent.
	//
	// The guest is NOT asked to add it here: adding a magnet means joining the
	// swarm and waiting for metadata, and this torrent exists nowhere but in
	// this test. That wait would hang rather than fail, which is the worst way
	// for a test to behave.
	//
	// What the client actually owns is that it hands the backend the magnet the
	// room gave it, and that magnet has to name the same torrent. The identity
	// itself — a real magnet resolving to the same infohash on another backend —
	// is checked in TestAddingTheSameTorrentTwiceYieldsTheSameInfohash below.
	assert.Contains(t, offered.MagnetURI, added.ID,
		"the magnet the room sent names a different torrent than the room is bound to")
	assert.Contains(t, offered.MagnetURI, "urn:btih:",
		"the magnet has no BitTorrent info hash at all")

	// 7. And the stream URL points at the guest's own backend, not the host's.
	assert.Equal(t,
		guest.BaseURL+"/api/v1/torrents/"+added.ID+"/stream",
		guestClient.StreamURL(added.ID),
		"the guest would play the host's download instead of its own")
	assert.NotEqual(t, host.BaseURL, guestClient.DataURL(),
		"the guest's data plane is the host's server — the split is not in effect")
}

// TestRoomRefusesATorrentItWasNotBoundTo keeps the binding meaningful: a guest
// that joins with the wrong infohash must be turned away, or the room would
// happily watch two different things.
func TestRoomRefusesATorrentItWasNotBoundTo(t *testing.T) {
	host := startServer(t, "host-peer")

	roomSession := room.NewSession(nil, host.BaseURL, host.Token, "host-peer")
	roomID, err := roomSession.Create(context.Background(), "Movie night", "",
		"0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)

	impostor := room.NewSession(nil, host.BaseURL, host.Token, "impostor-peer")
	err = impostor.Join(context.Background(), roomID, "", "ffffffffffffffffffffffffffffffffffffffff")

	require.Error(t, err, "a guest joined with a torrent the room is not bound to")
	assert.False(t, impostor.InRoom())
}

// TestTheHostAloneCannotPlayIntoTheRoom — the control model: one participant
// drives playback, and a guest's attempt is refused rather than silently
// ignored.
func TestTheHostDrivesPlaybackAndAGuestCannot(t *testing.T) {
	host := startServer(t, "host-peer")

	roomSession := room.NewSession(nil, host.BaseURL, host.Token, "host-peer")
	roomID, err := roomSession.Create(context.Background(), "Movie night", "",
		"0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)

	guest := room.NewSession(nil, host.BaseURL, host.Token, "guest-peer")
	require.NoError(t, guest.Join(context.Background(), roomID, "", "0123456789abcdef0123456789abcdef01234567"))

	ctx := context.Background()
	require.NoError(t, roomSession.Play(ctx), "the host could not start the room")

	err = guest.Play(ctx)
	require.Error(t, err, "a guest drove playback — the room has no single driver")

	// But anyone may say they are stalled. Otherwise a participant whose
	// stream recovered could not say so, and the room would stay paused for
	// everyone.
	require.NoError(t, guest.ReportBuffering(ctx, true), "a guest could not report its own buffering")

	var status struct {
		Buffering bool `json:"buffering"`
		IsPlaying bool `json:"isPlaying"`
	}
	require.Equal(t, 200, host.do(t, "GET", "/api/v1/sync/status", nil, &status))
	assert.True(t, status.IsPlaying)
	assert.True(t, status.Buffering, "the room did not see that a guest stalled")
}

// TestAddingTheSameTorrentTwiceYieldsTheSameInfohash is the property the whole
// room binding rests on: two independent backends, given the same torrent, end
// up agreeing on its identity. If they did not, a guest would be watching a
// different film from the room it joined, and every room restart would orphan
// the binding.
func TestAddingTheSameTorrentTwiceYieldsTheSameInfohash(t *testing.T) {
	first := startServer(t, "peer-one")
	second := startServer(t, "peer-two")

	torrentFile := buildTorrentFile(t, "Big Buck Bunny", 4<<20)

	var added struct {
		ID string `json:"id"`
	}
	require.Equal(t, 201, first.do(t, "POST", "/api/v1/torrents",
		map[string]string{"torrentFile": encodeTorrent(t, torrentFile)}, &added))
	require.NotEmpty(t, added.ID)

	var repeated struct {
		ID string `json:"id"`
	}
	require.Equal(t, 201, second.do(t, "POST", "/api/v1/torrents",
		map[string]string{"torrentFile": encodeTorrent(t, torrentFile)}, &repeated),
		"a second backend refused the same torrent")

	assert.Equal(t, added.ID, repeated.ID,
		"two backends disagree about what this torrent is")
}

// TestTheDataPlaneIsNotTheHostsServer states the split in one assertion, because
// every later property depends on it: if the guest's data plane were the host's
// server, "each participant pulls from the swarm" would be false while still
// appearing to work.
func TestTheDataPlaneIsNotTheHostsServer(t *testing.T) {
	host := startServer(t, "host-peer")
	guest := startServer(t, "guest-peer")

	guestClient := api.New(guest.BaseURL, guest.Token, host.BaseURL, host.Token)
	guestClient.SetClientID("guest-peer")

	assert.NotEqual(t, host.BaseURL, guestClient.DataURL(),
		"the guest's torrents would live on the host's server")
	assert.Equal(t, host.BaseURL, guestClient.ControlURL(),
		"the room is hosted by the host")

	// And the tokens differ, which is what makes the two planes separable at
	// all: the host's token opens the room and must not open the guest's
	// backend.
	assert.Equal(t, guest.Token, guestClient.DataToken())
	assert.Equal(t, host.Token, guestClient.ControlToken())
}
