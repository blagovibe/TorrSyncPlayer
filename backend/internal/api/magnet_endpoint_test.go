// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
)

// The host announces its own film to the room itself now — there is no server
// left to broadcast it from. For that it needs the magnet of a torrent it
// already holds, and the infohash alone does not carry one: it identifies a
// torrent without transporting it (idea-0064).
const magnetTestTorrentID = "0123456789abcdef0123456789abcdef01234567"

func TestGetMagnet_ReturnsTheMagnetForTheCallersTorrent(t *testing.T) {
	svc := newRecordingTorrentService()
	svc.magnet = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&tr=http://tracker.example/announce"

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(apiAuthSvc.TokenMiddleware)
		r.Get("/torrents/{id}/magnet", GetMagnet(svc))
	})

	req := httptest.NewRequest(http.MethodGet, "/torrents/"+magnetTestTorrentID+"/magnet", nil)
	req.Header.Set(auth.HeaderAccessToken, apiAuthSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "host-peer")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "magnet:?xt=urn:btih:")
	assert.Contains(t, rec.Body.String(), magnetTestTorrentID)
}

func TestGetMagnet_UnknownTorrentIsNotFound(t *testing.T) {
	svc := newRecordingTorrentService()
	svc.magnetErr = errors.New(errors.ErrNotFound, "torrent not found")

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(apiAuthSvc.TokenMiddleware)
		r.Get("/torrents/{id}/magnet", GetMagnet(svc))
	})

	req := httptest.NewRequest(http.MethodGet, "/torrents/"+magnetTestTorrentID+"/magnet", nil)
	req.Header.Set(auth.HeaderAccessToken, apiAuthSvc.AccessToken())
	req.Header.Set(auth.HeaderClientID, "host-peer")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetMagnet_InvalidIDIsBadRequest(t *testing.T) {
	handler := GetMagnet(newRecordingTorrentService())

	r := chi.NewRouter()
	r.Get("/torrents/{id}/magnet", handler)

	req := httptest.NewRequest(http.MethodGet, "/torrents/not-an-id/magnet", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
