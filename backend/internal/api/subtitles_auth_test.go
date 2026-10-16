// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
)

// Subtitles ride the same road as video (idea-0080, variant 2): the file
// travels an endpoint, mpv fetches it with the ordinary access-token header.
// So the endpoint takes the same gates as /stream — header, client id,
// room-scoped read access — while the index in the URL names any file of the
// torrent, not the selected one.

func TestSubtitlesAcceptTheOrdinaryAccessToken(t *testing.T) {
	authSvc := apiAuthSvc
	svc := &servedSubtitles{body: "1\n00:00:01,000 --> 00:00:02,000\nHi\n"}

	router := httptest.NewServer(subtitlesRouter(authSvc, svc))
	defer router.Close()

	req, err := http.NewRequest(http.MethodGet, router.URL+subtitlesPath(3), nil)
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, authSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "peer-1")

	resp, err := router.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "peer-1", svc.servedAs)
	assert.Equal(t, streamTorrentID, svc.servedTorrent)
	assert.Equal(t, 3, svc.servedIndex, "must serve the requested file, not the selection")
}

// TestSubtitlesIgnoreTheTrailingName pins that the file name after the index
// is mpv's, not the server's: it rides along for format detection, and a
// different name must not move the answer off the indexed file.
func TestSubtitlesIgnoreTheTrailingName(t *testing.T) {
	authSvc := apiAuthSvc
	svc := &servedSubtitles{body: "payload"}

	router := httptest.NewServer(subtitlesRouter(authSvc, svc))
	defer router.Close()

	req, err := http.NewRequest(http.MethodGet,
		router.URL+"/api/v1/torrents/"+streamTorrentID+"/subtitles/3/not-the-real-name.srt", nil)
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, authSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "peer-1")

	resp, err := router.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, 3, svc.servedIndex)
}

func TestSubtitlesRejectAWrongAccessToken(t *testing.T) {
	authSvc := apiAuthSvc
	svc := &servedSubtitles{body: "payload"}

	router := httptest.NewServer(subtitlesRouter(authSvc, svc))
	defer router.Close()

	req, err := http.NewRequest(http.MethodGet, router.URL+subtitlesPath(0), nil)
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, "not-the-token")

	resp, err := router.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.Empty(t, svc.servedAs)
}

func TestSubtitlesRejectARequestWithNeitherCredential(t *testing.T) {
	authSvc := apiAuthSvc
	svc := &servedSubtitles{body: "payload"}

	router := httptest.NewServer(subtitlesRouter(authSvc, svc))
	defer router.Close()

	resp, err := router.Client().Get(router.URL + subtitlesPath(0))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestSubtitlesEnforceRoomScopedReadAccess(t *testing.T) {
	authSvc := apiAuthSvc
	svc := &servedSubtitles{body: "payload", denied: true}

	router := httptest.NewServer(subtitlesRouter(authSvc, svc))
	defer router.Close()

	req, err := http.NewRequest(http.MethodGet, router.URL+subtitlesPath(0), nil)
	require.NoError(t, err)
	req.Header.Set(auth.HeaderAccessToken, authSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "peer-1")

	resp, err := router.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	assert.Empty(t, svc.servedAs, "subtitles handed to a non-participant")

	svc.denied = false
	resp2, err := router.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
}

func TestSubtitlesRefuseABadIndex(t *testing.T) {
	authSvc := apiAuthSvc
	svc := &servedSubtitles{body: "payload"}

	router := httptest.NewServer(subtitlesRouter(authSvc, svc))
	defer router.Close()

	for _, path := range []string{
		"/api/v1/torrents/" + streamTorrentID + "/subtitles/abc/film.srt",
		"/api/v1/torrents/" + streamTorrentID + "/subtitles/-1/film.srt",
	} {
		req, err := http.NewRequest(http.MethodGet, router.URL+path, nil)
		require.NoError(t, err)
		req.Header.Set(auth.HeaderAccessToken, authSvc.AccessToken())
		req.Header.Set(auth.HeaderClientID, "peer-1")

		resp, err := router.Client().Do(req)
		require.NoError(t, err)
		func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, path)
	}
	assert.Empty(t, svc.servedAs)
}

// servedSubtitles answers the subtitles endpoint and remembers what it
// answered, like servedTorrent does for the stream.
type servedSubtitles struct {
	internal.TorrentService
	body          string
	servedAs      string
	servedTorrent string
	servedIndex   int
	denied        bool
}

func (s *servedSubtitles) CanRead(_ context.Context, _ string, _ string) bool {
	return !s.denied
}

func (s *servedSubtitles) ServeSubtitles(w http.ResponseWriter, _ *http.Request, torrentID, userID string, fileIndex int) {
	s.servedAs = userID
	s.servedTorrent = torrentID
	s.servedIndex = fileIndex
	w.WriteHeader(http.StatusOK)
	// #nosec G705 -- test double writes its own fixture bytes, not user input
	_, _ = w.Write([]byte(s.body))
}

func subtitlesRouter(authSvc *auth.AuthService, svc internal.TorrentService) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/torrents/{id}/subtitles/{fileIndex}/*", SubtitlesFile(svc, authSvc))
	return r
}

func subtitlesPath(index int) string {
	return fmt.Sprintf("/api/v1/torrents/%s/subtitles/%d/film.srt", streamTorrentID, index)
}
