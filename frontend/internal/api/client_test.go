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

// The two planes are the whole point of the split: where a request lands
// decides who downloads the film. These tests hold the routing, because a
// regression here is invisible in every other test — both servers answer the
// same question the same way.
// planePair is two servers that answer identically and count their own hits.
// They must answer identically: if one of them answered differently, the test
// would pass for the wrong reason.
type planePair struct {
	data, control *httptest.Server
	dataHits      *int
	controlHits   *int
}

func newPlanePair(t *testing.T) planePair {
	t.Helper()

	var dataHits, controlHits int

	newSide := func(hits *int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			*hits++
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
	}

	pair := planePair{dataHits: &dataHits, controlHits: &controlHits}
	pair.data = newSide(&dataHits)
	pair.control = newSide(&controlHits)

	t.Cleanup(pair.data.Close)
	t.Cleanup(pair.control.Close)
	return pair
}

func TestTorrentCallsReachTheLocalBackend(t *testing.T) {
	pair := newPlanePair(t)
	c := New(pair.data.URL, "data-secret", pair.control.URL, "control-secret")

	var out map[string]any
	require.NoError(t, c.Get(context.Background(), Data, "/api/v1/torrents", &out))

	assert.Equal(t, 1, *pair.dataHits, "запрос торрентов ушёл не в свой бэкенд: гость смотрел бы чужую загрузку")
	assert.Equal(t, 0, *pair.controlHits, "запрос торрентов ушёл на сервер хоста")
}

func TestRoomCallsReachTheControlPlane(t *testing.T) {
	pair := newPlanePair(t)
	c := New(pair.data.URL, "data-secret", pair.control.URL, "control-secret")

	var out map[string]any
	require.NoError(t, c.Post(context.Background(), Control, "/api/v1/sync/play", nil, &out))

	assert.Equal(t, 1, *pair.controlHits, "команда синхронизации ушла в свой бэкенд")
	assert.Equal(t, 0, *pair.dataHits)
}

func TestEachPlaneIsSentItsOwnToken(t *testing.T) {
	// The tokens are different secrets. Sending the wrong one is a 401, and
	// then the application does not work on either plane.
	var dataToken, controlToken string
	data := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dataToken = r.Header.Get("X-Access-Token")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer data.Close()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controlToken = r.Header.Get("X-Access-Token")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer control.Close()

	c := New(data.URL, "data-secret", control.URL, "control-secret")

	var out map[string]any
	require.NoError(t, c.Get(context.Background(), Data, "/api/v1/torrents", &out))
	require.NoError(t, c.Get(context.Background(), Control, "/api/v1/rooms/info", &out))

	assert.Equal(t, "data-secret", dataToken)
	assert.Equal(t, "control-secret", controlToken)
}

func TestClientIDGoesToBothPlanes(t *testing.T) {
	// Ownership on the data plane is keyed by it, and on the control plane it
	// identifies the peer in the room. Missing it from either breaks something
	// quietly.
	var dataID, controlID string
	data := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dataID = r.Header.Get("X-Client-ID")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer data.Close()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controlID = r.Header.Get("X-Client-ID")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer control.Close()

	c := New(data.URL, "", control.URL, "")
	c.SetClientID("run-42")

	var out map[string]any
	require.NoError(t, c.Get(context.Background(), Data, "/api/v1/torrents", &out))
	require.NoError(t, c.Get(context.Background(), Control, "/api/v1/rooms/info", &out))

	assert.Equal(t, "run-42", dataID)
	assert.Equal(t, "run-42", controlID)
}

func TestStreamURLIsBuiltFromTheDataPlane(t *testing.T) {
	// The line that decides who downloads the film. Built from the host's
	// address, the guest plays the host's download — precisely what the goal
	// forbids.
	c := New("http://127.0.0.1:5555", "", "http://192.168.1.50:8889", "")

	assert.Equal(t,
		"http://127.0.0.1:5555/api/v1/torrents/abc123/stream",
		c.StreamURL("abc123"))
}

func TestAPIErrorNamesThePlaneThatFailed(t *testing.T) {
	// "the request failed" is not actionable. Which plane refused decides what
	// to do about it.
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid or missing access token"}`))
	}))
	defer control.Close()

	data := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer data.Close()

	c := New(data.URL, "", control.URL, "")

	var out map[string]any
	err := c.Get(context.Background(), Control, "/api/v1/rooms/info", &out)

	require.Error(t, err)
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.Status)
	assert.Equal(t, Control, apiErr.Plane)
	assert.True(t, strings.Contains(err.Error(), "control"), "the error does not name the plane: %v", err)
}

func TestTrailingSlashInBaseURLDoesNotDoubleUp(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "", srv.URL+"/", "")

	var out map[string]any
	require.NoError(t, c.Get(context.Background(), Data, "/api/v1/torrents", &out))

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
