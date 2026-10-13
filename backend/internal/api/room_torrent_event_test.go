// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The room has to tell its members what it is watching.
//
// Every participant pulls the media from the swarm themselves, so a guest that
// knows only the room's infohash has nothing to download: an infohash
// identifies a torrent without carrying one. Until the room hands the magnet
// over, splitting the planes changes where requests go but not what anyone can
// watch — the guest's backend stays empty.
//
// These tests go through the real router and the real room service, and read
// the real SSE stream: what is being asserted is what a member actually
// receives, not what a mock was told to record.

const (
	roomTestTorrent = "3b2b1a9c0d1e2f3a4b5c6d7e8f901234567890ab"
	roomTestMagnet  = "magnet:?xt=urn:btih:3b2b1a9c0d1e2f3a4b5c6d7e8f901234567890ab&dn=Big+Buck+Bunny"
)

// A guest that joins a room is told which torrent the room is watching, over
// the room's own event stream — the same stream that carries the roster.
func TestGuestJoiningRoomReceivesTheTorrent(t *testing.T) {
	server := roomTorrentServer(t)
	roomID, events := hostRoomWithStream(t, server)
	defer closeRoom(t, server, roomID)

	joinRoom(t, server, roomID, "guest-peer")

	ev := waitForTorrentEvent(t, events)
	assert.Equal(t, roomTestTorrent, ev.InfoHash)
	assert.Equal(t, roomTestMagnet, ev.MagnetURI,
		"гость получил magnet, отличный от исходного — у него оказался бы другой торрент")
	assert.Equal(t, "Big Buck Bunny", ev.Name)
}

// The host's chosen file travels with the torrent. Selection is per-peer
// (bug-0048), so a guest picking its own file means the room watches two
// different parts of one release.
func TestRoomTorrentCarriesTheHostsSelectedFile(t *testing.T) {
	server := roomTorrentServer(t)
	roomID, events := hostRoomWithStream(t, server)
	defer closeRoom(t, server, roomID)

	joinRoom(t, server, roomID, "guest-peer")

	assert.Equal(t, 2, waitForTorrentEvent(t, events).SelectedFile)
}

// A join must not fail because the server cannot describe the torrent. The
// room still exists and is still watchable for what the host already has;
// refusing the join would lock people out of a room they were allowed into.
func TestJoinSucceedsWhenTheTorrentCannotBeDescribed(t *testing.T) {
	torrents := newRecordingTorrentService()
	torrents.magnet = roomTestMagnet
	torrents.magnetErr = errNotImplemented
	server := roomTorrentServerWith(t, torrents)
	roomID, events := hostRoomWithStream(t, server)
	defer closeRoom(t, server, roomID)

	rec := roomRequest(t, server, http.MethodPost, "/api/v1/rooms/join", map[string]any{
		"roomId": roomID, "password": "secretpw", "torrentId": roomTestTorrent,
	}, "guest-peer")

	assert.Equal(t, http.StatusOK, rec.Code,
		"гость, которому не описали торрент, не должен остаться за дверью")
	assert.True(t, waitForNoTorrentEvent(t, events),
		"раз торрент описать не удалось, событие torrent всё равно ушло — гость получил бы пустоту")
}

// ── server ─────────────────────────────────────────────────────────────────

// roomTorrentServer builds a router over the real room service and a torrent
// service that can describe one torrent. The room service is the real one:
// the broadcast path is the thing under test.
func roomTorrentServer(t *testing.T) *httptest.Server {
	t.Helper()

	torrents := newRecordingTorrentService()
	torrents.magnet = roomTestMagnet
	torrents.name = "Big Buck Bunny"
	torrents.selected = 2

	return roomTorrentServerWith(t, torrents)
}

func roomTorrentServerWith(t *testing.T, torrents *recordingTorrentService) *httptest.Server {
	t.Helper()

	router := NewRouter(RouterConfig{
		TorrentSvc:  torrents,
		P2pSvc:      apiP2pSvc,
		SyncSvc:     apiSyncSvc,
		AuthService: apiAuthSvc,
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}

// hostRoomWithStream creates a room as the host and opens its event stream.
//
// The stream is opened after creation on purpose: the host is inside the room
// from the moment it exists, and what matters is what an *existing* member
// receives when somebody else arrives.
func hostRoomWithStream(t *testing.T, server *httptest.Server) (string, <-chan roomFrame) {
	t.Helper()

	rec := roomRequest(t, server, http.MethodPost, "/api/v1/rooms", map[string]any{
		"name": "Movie night", "password": "secretpw", "torrentId": roomTestTorrent,
	}, "host-peer")
	require.Equal(t, http.StatusCreated, rec.Code)

	var room models.RoomInfo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &room))
	require.NotEmpty(t, room.ID)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	req, err := http.NewRequest(http.MethodGet,
		server.URL+"/api/v1/rooms/"+room.ID+"/events", nil)
	require.NoError(t, err)
	req.Header.Set("X-Access-Token", apiAuthSvc.AccessToken())
	req.Header.Set("X-Client-ID", "host-peer")

	resp, err := server.Client().Do(req.WithContext(ctx))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	frames := make(chan roomFrame, 64)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			raw, ok := strings.CutPrefix(scanner.Text(), "data:")
			if !ok {
				continue
			}
			var frame roomFrame
			if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &frame); err != nil {
				continue
			}
			frames <- frame
		}
	}()

	return room.ID, frames
}

// roomFrame is one decoded SSE frame. The server writes the whole P2PEvent as
// the data payload, so the type selects the payload shape — see
// internal/models/events.go.
type roomFrame struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

func closeRoom(t *testing.T, server *httptest.Server, roomID string) {
	t.Helper()
	roomRequest(t, server, http.MethodPost, "/api/v1/rooms/leave", nil, "host-peer")
	_ = roomID
}

// ── requests ───────────────────────────────────────────────────────────────

func joinRoom(t *testing.T, server *httptest.Server, roomID, userID string) {
	t.Helper()

	rec := roomRequest(t, server, http.MethodPost, "/api/v1/rooms/join", map[string]any{
		"roomId": roomID, "password": "secretpw", "torrentId": roomTestTorrent,
	}, userID)
	require.Equal(t, http.StatusOK, rec.Code)
}

// roomRequest goes through a recorder so the body is buffered. The real SSE
// response must not go through one — it never ends.
func roomRequest(t *testing.T, server *httptest.Server, method, path string,
	body map[string]any, userID string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequest(method, server.URL+path, reader)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Access-Token", apiAuthSvc.AccessToken())
	req.Header.Set("X-Client-ID", userID)

	rec := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(rec, req.WithContext(req.Context()))
	return rec
}

// ── stream reading ─────────────────────────────────────────────────────────

// waitForTorrentEvent reads the stream until the room's torrent event arrives.
// The same stream also carries connected and roster events, which are not the
// subject here.
func waitForTorrentEvent(t *testing.T, events <-chan roomFrame) models.TorrentEvent {
	t.Helper()

	timeout := time.After(5 * time.Second)
	for {
		select {
		case frame, ok := <-events:
			if !ok {
				t.Fatal("поток комнаты закрылся, не принеся событие torrent")
			}
			if frame.Type != models.EventTypeTorrent {
				continue
			}
			require.NotNil(t, frame.Data, "событие torrent без полезной нагрузки")

			return models.TorrentEvent{
				InfoHash:     str(frame.Data["infoHash"]),
				MagnetURI:    str(frame.Data["magnetUri"]),
				Name:         str(frame.Data["name"]),
				SelectedFile: num(frame.Data["selectedFile"]),
			}
		case <-timeout:
			t.Fatal("поток комнаты не принёс событие torrent за 5 секунд")
			return models.TorrentEvent{}
		}
	}
}

// waitForNoTorrentEvent drains the stream briefly and reports whether the room
// stayed silent about its torrent. The other room events keep flowing, so this
// waits on the absence of one specific type rather than on the absence of all
// traffic.
func waitForNoTorrentEvent(t *testing.T, events <-chan roomFrame) bool {
	t.Helper()

	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case frame, ok := <-events:
			if !ok {
				return true
			}
			if frame.Type == models.EventTypeTorrent {
				return false
			}
		case <-timeout:
			return true
		}
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func num(v any) int {
	f, _ := v.(float64)
	return int(f)
}
