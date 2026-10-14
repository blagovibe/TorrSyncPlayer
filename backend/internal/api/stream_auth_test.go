// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
)

// /stream used to be reachable by anyone holding a signed ticket in the URL,
// which made the ticket transmissible — deviation-0029. That existed because
// mpv was believed incapable of sending a header. It is capable, in both
// deployment modes (idea-0066), so the media endpoint can take the ordinary
// access token like every other endpoint does.
//
// The ticket machinery is gone entirely. What remains is the ordinary header
// path, identical to every other endpoint — which is exactly what idea-0066
// found was possible.

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

// TestStreamIgnoresAQueryTicket вместo приёма. Тикета больше нет нигде, и
// притворяться, что старый URL работает — значит разрешить ровно то, что
// deviation-0029 был создан, чтобы отклонять: передаваемый bearer-значение в
// адресной строке.
func TestStreamIgnoresAQueryTicket(t *testing.T) {
	authSvc := apiAuthSvc
	svc := &servedTorrent{body: "payload"}

	router := httptest.NewServer(streamRouter(authSvc, svc))
	defer router.Close()

	resp, err := router.Client().Get(
		router.URL + "/api/v1/torrents/0123456789abcdef0123456789abcdef01234567/stream?ticket=1700000000.peer-7.0123456789abcdef0123456789abcdef01234567.deadbeef")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"поддельный тикет в URL открыл доступ к потоку")
	assert.Empty(t, svc.servedAs)
}

func TestStreamEnforcesRoomScopedReadAccess(t *testing.T) {
	// The room-scoped check used to live in the (deleted) stream-ticket
	// handler. Deleting that handler without moving the check here would
	// delete the restriction itself, which looks like a simplification.
	authSvc := apiAuthSvc
	svc := &servedTorrent{body: "payload", denied: true}

	router := httptest.NewServer(streamRouter(authSvc, svc))
	defer router.Close()

	req, err := http.NewRequest(http.MethodGet, router.URL+streamPath, nil)
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, authSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "peer-1")

	resp, err := router.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Empty(t, svc.servedAs, "potок отдан не участвующему в комнате")

	// The same client after joining the room gets the media.
	svc.denied = false
	resp2, err := router.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

// servedTorrent answers the stream and remembers who it answered.
type servedTorrent struct {
	internal.TorrentService
	body     string
	servedAs string
	denied   bool
}

func (s *servedTorrent) CanRead(_ context.Context, _ string, _ string) bool {
	return !s.denied
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
