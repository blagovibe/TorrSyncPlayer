// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// /stream used to be reachable by anyone holding a signed ticket in the URL,
// which made the ticket transmissible — deviation-0029. That existed because
// mpv was believed incapable of sending a header. It is capable, in both
// deployment modes (idea-0066), so the media endpoint can take the ordinary
// access token like every other endpoint does.
//
// The ticket keeps working: a player built against the old contract must not
// break before the new path has proven itself.

func TestStreamAcceptsTheOrdinaryAccessToken(t *testing.T) {
	authSvc := apiAuthSvc
	svc := &servedTorrent{body: "payload"}

	router := httptest.NewServer(streamRouter(authSvc, svc))
	defer router.Close()

	req, err := http.NewRequest(http.MethodGet, router.URL+streamPath, nil)
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, authSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "peer-1")

	resp, err := router.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "peer-1", svc.servedAs,
		"поток отдан не тому клиенту, чей токен предъявлен")
}

func TestStreamRejectsAWrongAccessToken(t *testing.T) {
	authSvc := apiAuthSvc
	svc := &servedTorrent{body: "payload"}

	router := httptest.NewServer(streamRouter(authSvc, svc))
	defer router.Close()

	req, err := http.NewRequest(http.MethodGet, router.URL+streamPath, nil)
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, "not-the-token")

	resp, err := router.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Empty(t, svc.servedAs, "поток отдан при неверном токене")
}

func TestStreamRejectsARequestWithNeitherCredential(t *testing.T) {
	authSvc := apiAuthSvc
	svc := &servedTorrent{body: "payload"}

	router := httptest.NewServer(streamRouter(authSvc, svc))
	defer router.Close()

	resp, err := router.Client().Get(router.URL + streamPath)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestStreamStillAcceptsAnIssuedTicket(t *testing.T) {
	// The old contract must keep working until the new path is proven.
	authSvc := apiAuthSvc
	svc := &servedTorrent{body: "payload"}

	router := httptest.NewServer(streamRouter(authSvc, svc))
	defer router.Close()

	ticket, err := authSvc.GenerateStreamTicket("peer-7", "0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)

	resp, err := router.Client().Get(router.URL + "/api/v1/torrents/0123456789abcdef0123456789abcdef01234567/stream?ticket=" + ticket)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "peer-7", svc.servedAs)
}

// servedTorrent answers the stream and remembers who it answered.
type servedTorrent struct {
	internal.TorrentService
	body     string
	servedAs string
}

func (s *servedTorrent) ServeFile(w http.ResponseWriter, _ *http.Request, _ string, userID string) {
	s.servedAs = userID
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(s.body))
}

// streamRouter mounts /stream the way the real one is mounted: outside the
// protected group, because it authenticates itself.
func streamRouter(authSvc *auth.AuthService, svc internal.TorrentService) http.Handler {
	r := chi.NewRouter()
	r.Get(streamRoute, StreamFile(svc, authSvc))
	return r
}

// streamTorrentID must be 40 hex characters: the handler validates it before
// anything else, so a short id would fail as 400 and never reach the
// authentication under test.
const (
	streamTorrentID = "0123456789abcdef0123456789abcdef01234567"
	streamRoute     = "/api/v1/torrents/{id}/stream"
	streamPath      = "/api/v1/torrents/0123456789abcdef0123456789abcdef01234567/stream"
)
