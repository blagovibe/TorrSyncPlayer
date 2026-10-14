// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package room

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorder notes what the room server was asked, so the tests assert on the
// wire rather than on the client's own bookkeeping. Requests arrive from the
// HTTP server's goroutines, so the slice is behind a lock.
type recorder struct {
	mu        sync.Mutex
	recorders []recordedRequest
}

type recordedRequest struct {
	Method string
	Path   string
	Body   map[string]any
	Token  string
	Client string
}

func (r *recorder) add(req recordedRequest) {
	r.mu.Lock()
	r.recorders = append(r.recorders, req)
	r.mu.Unlock()
}

func (r *recorder) all() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.recorders...)
}

func (r *recorder) last() recordedRequest {
	all := r.all()
	if len(all) == 0 {
		return recordedRequest{}
	}
	return all[len(all)-1]
}

func roomServer(t *testing.T, rec *recorder, respond func(path string) (int, string)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		entry := recordedRequest{
			Method: req.Method,
			Path:   req.URL.Path,
			Token:  req.Header.Get("X-Access-Token"),
			Client: req.Header.Get("X-Client-ID"),
		}
		if req.Body != nil {
			_ = json.NewDecoder(req.Body).Decode(&entry.Body)
		}
		rec.add(entry)

		status, body := respond(req.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

const testRoomID = "0123456789abcdef0123456789abcdef"

func TestCreateSendsTheTorrentBinding(t *testing.T) {
	rec := &recorder{}
	server := roomServer(t, rec, func(string) (int, string) {
		return http.StatusCreated, `{"id":"` + testRoomID + `"}`
	})

	session := NewSession(nil, server.URL, "tok", "peer-1")
	id, err := session.Create(context.Background(), "Movie night", "secretpw", "abc123")
	require.NoError(t, err)
	assert.Equal(t, testRoomID, id)
	assert.True(t, session.InRoom())
	assert.Equal(t, testRoomID, session.RoomID())

	sent := rec.last()
	assert.Equal(t, "/api/v1/rooms", sent.Path)
	assert.Equal(t, "abc123", sent.Body["torrentId"], "the room was created without saying what it watches")
	assert.Equal(t, "tok", sent.Token)
	assert.Equal(t, "peer-1", sent.Client)
}

func TestCreateRefusesToMakeARoomWithNoTorrent(t *testing.T) {
	// A room with nothing to watch is a room with nothing to do. The server
	// would refuse it too; catching it here keeps the message clear.
	rec := &recorder{}
	server := roomServer(t, rec, func(string) (int, string) { return http.StatusCreated, "{}" })

	session := NewSession(nil, server.URL, "tok", "peer-1")
	_, err := session.Create(context.Background(), "Movie night", "", "")

	require.Error(t, err)
	assert.Empty(t, rec.all(), "a request was sent for a room with no torrent")
}

func TestJoinSendsRoomAndTorrent(t *testing.T) {
	rec := &recorder{}
	server := roomServer(t, rec, func(string) (int, string) {
		return http.StatusOK, `{"message":"Joined the room","id":"` + testRoomID + `"}`
	})

	session := NewSession(nil, server.URL, "tok", "peer-1")
	require.NoError(t, session.Join(context.Background(), testRoomID, "secretpw", "abc123"))

	sent := rec.last()
	assert.Equal(t, "/api/v1/rooms/join", sent.Path)
	assert.Equal(t, testRoomID, sent.Body["roomId"])
	assert.Equal(t, "abc123", sent.Body["torrentId"])
	assert.True(t, session.InRoom())
}

func TestJoinFailureLeavesTheSessionOutside(t *testing.T) {
	// Otherwise a refused join leaves the client believing it is in a room and
	// streaming events it will never be sent.
	rec := &recorder{}
	server := roomServer(t, rec, func(string) (int, string) {
		return http.StatusForbidden, `{"error":"wrong password"}`
	})

	session := NewSession(nil, server.URL, "tok", "peer-1")
	require.Error(t, session.Join(context.Background(), testRoomID, "wrong", "abc123"))
	assert.False(t, session.InRoom())
	assert.Empty(t, session.RoomID())
}

func TestSyncCommandsGoToTheRoomServer(t *testing.T) {
	rec := &recorder{}
	server := roomServer(t, rec, func(string) (int, string) { return http.StatusOK, "{}" })

	session := NewSession(nil, server.URL, "tok", "peer-1")
	ctx := context.Background()

	require.NoError(t, session.Play(ctx))
	require.NoError(t, session.Pause(ctx))
	require.NoError(t, session.Seek(ctx, 42.5))
	require.NoError(t, session.ReportBuffering(ctx, true))

	paths := make([]string, 0, 4)
	for _, request := range rec.all() {
		paths = append(paths, request.Path)
	}
	assert.Equal(t, []string{
		"/api/v1/sync/play",
		"/api/v1/sync/pause",
		"/api/v1/sync/seek",
		"/api/v1/sync/buffering",
	}, paths)

	assert.Equal(t, 42.5, rec.all()[2].Body["position"])
	assert.Equal(t, true, rec.all()[3].Body["buffering"],
		"буферизация объявляется любым участником, а не только хостом")
}

func TestInfoReturnsTheBindingAndRoster(t *testing.T) {
	rec := &recorder{}
	server := roomServer(t, rec, func(string) (int, string) {
		return http.StatusOK, `{"id":"` + testRoomID + `","torrentId":"abc123","hostUserId":"host",` +
			`"rosterSeq":4,"peers":[{"peerID":"a","userID":"host","isHost":true}]}`
	})

	session := NewSession(nil, server.URL, "tok", "peer-1")
	torrentID, peers, seq, host, err := session.Info(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "abc123", torrentID)
	assert.Equal(t, 4, seq)
	assert.Equal(t, "host", host)
	assert.Len(t, peers, 1)
}

func TestLeaveClearsTheSessionEvenWhenTheServerRefuses(t *testing.T) {
	// The server prunes idle peers on its own, so the worst case is a ghost in
	// the roster. Leaving the client believing it is still inside is worse.
	rec := &recorder{}
	server := roomServer(t, rec, func(path string) (int, string) {
		if path == "/api/v1/rooms/leave" {
			return http.StatusInternalServerError, `{"error":"boom"}`
		}
		return http.StatusOK, `{"id":"` + testRoomID + `"}`
	})

	session := NewSession(nil, server.URL, "tok", "peer-1")
	require.NoError(t, session.Join(context.Background(), testRoomID, "", "abc"))
	require.True(t, session.InRoom())

	require.Error(t, session.Leave(context.Background()))
	assert.False(t, session.InRoom())
	assert.Empty(t, session.RoomID())
}

// ── invite code ────────────────────────────────────────────────────────────

func TestInviteCodeRoundTrip(t *testing.T) {
	code := InviteCode{RoomID: testRoomID, Password: "secretpw"}
	assert.Equal(t, testRoomID+":secretpw", code.Encode())

	parsed, err := ParseInviteCode(code.Encode())
	require.NoError(t, err)
	assert.Equal(t, testRoomID, parsed.RoomID)
	assert.Equal(t, "secretpw", parsed.Password)
}

func TestInviteCodeSplitsOnTheFirstColonOnly(t *testing.T) {
	// A password may contain anything, colons included. Splitting on the last
	// colon, or on all of them, cuts the password in the wrong place.
	parsed, err := ParseInviteCode(testRoomID + ":a:b:c")
	require.NoError(t, err)
	assert.Equal(t, "a:b:c", parsed.Password)
}

func TestInviteCodeWithoutAPasswordIsAnIdAndNoPassword(t *testing.T) {
	// Someone pasting only the id has made a typo, not a mistake worth refusing
	// over: the server judges the password, not this.
	parsed, err := ParseInviteCode(testRoomID)
	require.NoError(t, err)
	assert.Equal(t, testRoomID, parsed.RoomID)
	assert.Empty(t, parsed.Password)
}

func TestInviteCodeToleratesSurroundingWhitespace(t *testing.T) {
	// Codes get pasted from chat windows, which add spaces and newlines.
	parsed, err := ParseInviteCode("  " + testRoomID + ":pw \n")
	require.NoError(t, err)
	assert.Equal(t, testRoomID, parsed.RoomID)
	assert.Equal(t, "pw", parsed.Password)
}

func TestInviteCodeRejectsMalformedIdsBeforeContactingAnyone(t *testing.T) {
	// Checking here is not security — the server checks again — but it turns a
	// typo into an immediate message instead of a round trip to a stranger.
	for _, bad := range []string{
		"too-short",
		"0123456789abcdef0123456789abcdeg", // g is not hex
		"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
	} {
		_, err := ParseInviteCode(bad + ":pw")
		assert.Error(t, err, "%q was accepted", bad)
	}
}

func TestInviteCodeCarriesNoAccessToken(t *testing.T) {
	// The token is a credential for the whole server, not for one room. Putting
	// it in an invitation would raise a guest from "join this room" to
	// "control this server" — which is the privilege the URL ticket cannot
	// grant either.
	code := InviteCode{RoomID: testRoomID, Password: "secretpw"}.Encode()

	assert.Equal(t, 1, strings.Count(code, ":"))
	assert.NotContains(t, strings.ToLower(code), "token")
}
