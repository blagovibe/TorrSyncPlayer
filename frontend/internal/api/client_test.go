// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// There used to be two planes — a room server next to the data plane — and
// these tests held the routing between them. Rooms meet over WebRTC now, so
// the client talks to one backend: its own. What remains pinned is that every
// call carries the token and the client id, and lands on that backend.
func TestCallsCarryTheTokenAndTheClientID(t *testing.T) {
	var token, clientID, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token = r.Header.Get("X-Access-Token")
		clientID = r.Header.Get("X-Client-ID")
		path = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "secret")
	c.SetClientID("run-42")

	var out map[string]any
	require.NoError(t, c.Get(context.Background(), "/api/v1/torrents", &out))

	assert.Equal(t, "secret", token, "the backend never saw the access token")
	assert.Equal(t, "run-42", clientID, "ownership on the backend is keyed by the client id")
	assert.Equal(t, "/api/v1/torrents", path)
}

func TestStreamURLIsBuiltFromTheLocalBackend(t *testing.T) {
	// The line that decides who downloads the film. Built from anyone else's
	// address, the guest plays the host's download — precisely what the goal
	// forbids.
	c := New("http://127.0.0.1:5555", "")

	assert.Equal(t,
		"http://127.0.0.1:5555/api/v1/torrents/abc123/stream",
		c.StreamURL("abc123"))
}

func TestSubtitlesURLIsBuiltFromTheLocalBackend(t *testing.T) {
	c := New("http://127.0.0.1:5555", "")

	assert.Equal(t,
		"http://127.0.0.1:5555/api/v1/torrents/abc123/subtitles/2/film.srt",
		c.SubtitlesURL("abc123", 2, "film.srt"))
}

func TestSubtitlesURLEscapesTheFileName(t *testing.T) {
	c := New("http://127.0.0.1:5555", "")

	assert.Equal(t,
		"http://127.0.0.1:5555/api/v1/torrents/abc123/subtitles/2/subs%2Ffilm%20RU.srt",
		c.SubtitlesURL("abc123", 2, "subs/film RU.srt"))
}

func TestAPIErrorNamesWhatFailed(t *testing.T) {
	// "the request failed" is not actionable. The path and status are.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid or missing access token"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")

	var out map[string]any
	err := c.Get(context.Background(), "/api/v1/torrents", &out)

	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.Status)
	assert.True(t, strings.Contains(err.Error(), "/api/v1/torrents"), "the error does not say where: %v", err)
}

func TestTrailingSlashInBaseURLDoesNotDoubleUp(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "")

	var out map[string]any
	require.NoError(t, c.Get(context.Background(), "/api/v1/torrents", &out))

	assert.Equal(t, "/api/v1/torrents", seen)
}

// A call the window has already given two minutes must not be cut off by a
// blanket cap inside the client.
//
// The backend answers "add this magnet" only once the swarm has delivered the
// metadata, and it allows 60 seconds for that
// (backend/internal/constants/constants.go, MagnetMetadataTimeout). The client
// used to cap every request at 30 seconds, which silently overrode the caller's
// context: a guest joining a room on a slow swarm got "context deadline exceeded"
// at half a minute while its own backend was still working — the same shape of
// lie as a dead backend being called a timeout (bug-0079).
func TestTheClientOutlivesTheBackendsSlowestAnswer(t *testing.T) {
	// The number lives in the other module, so it is restated here rather than
	// imported: the coupling is real, and no test can span the boundary and keep
	// it honest.
	const backendMetadataWait = 60 * time.Second

	if requestTimeout < backendMetadataWait {
		t.Fatalf("the client would give up after %v, before the backend's %v wait finishes",
			requestTimeout, backendMetadataWait)
	}
}
