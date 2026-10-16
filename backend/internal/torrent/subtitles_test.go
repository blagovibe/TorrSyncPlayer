// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Subtitles travel a different road than video: the played file is the
// selection, but a subtitle is any file of the torrent named .srt. ServeFile
// answers 400 without a selection of its own; ServeSubtitles must not — a
// guest that never selected anything still needs the subtitles of the film
// the room watches. What it keeps from ServeFile is every gate: the torrent
// must exist, be readable, and the index must name a file.
func TestServeSubtitlesServesArbitraryFileByIndex(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	// No selection at all — and the backend must still be reached, with the
	// requested index rather than any selection.
	rec := httptest.NewRecorder()
	svc.ServeSubtitles(rec, httptest.NewRequest(http.MethodGet, "/subtitles", http.NoBody), "abc123", "peer-a", 1)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "payload", rec.Body.String())

	backend.mu.Lock()
	defer backend.mu.Unlock()
	require.Len(t, backend.servedIndex, 1)
	assert.Equal(t, 1, backend.servedIndex[0], "must serve the requested file, not the selection")
}

func TestServeSubtitlesUnknownTorrent(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	rec := httptest.NewRecorder()
	svc.ServeSubtitles(rec, httptest.NewRequest(http.MethodGet, "/subtitles", http.NoBody), "nope", "peer-a", 0)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.servedIndex)
}

func TestServeSubtitlesRefusesBadIndex(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	for _, index := range []int{-1, 2, 99} {
		rec := httptest.NewRecorder()
		svc.ServeSubtitles(rec, httptest.NewRequest(http.MethodGet, "/subtitles", http.NoBody), "abc123", "peer-a", index)
		assert.Equal(t, http.StatusBadRequest, rec.Code, "index %d names no file", index)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.servedIndex)
}

func TestServeSubtitlesRefusesUnreadableTorrent(t *testing.T) {
	backend := &stubBackend{handle: newStubHandle()}
	svc := newStubService(t, backend, nil)
	t.Cleanup(func() { _ = svc.Close() })

	_, err := svc.AddMagnet(context.Background(), "peer-a", testMagnet)
	require.NoError(t, err)

	// peer-b added nothing and sits in no room with peer-a: like ServeFile,
	// the answer is 404 rather than 403, so an outsider does not learn the
	// torrent exists.
	rec := httptest.NewRecorder()
	svc.ServeSubtitles(rec, httptest.NewRequest(http.MethodGet, "/subtitles", http.NoBody), "abc123", "peer-b", 1)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	backend.mu.Lock()
	defer backend.mu.Unlock()
	assert.Empty(t, backend.servedIndex)
}
