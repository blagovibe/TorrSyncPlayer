// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bug-0052: Close() must terminate even when persistence is attached.
//
// Close takes s.mu.Lock() and then calls SaveRegistry(), which takes
// s.mu.RLock() on the same mutex. sync.RWMutex is not reentrant, so the call
// blocks forever. Reachable on every graceful shutdown that runs with
// --data-dir set, because that is what wires persistence in.
func TestClose_ReturnsWithPersistenceAttached(t *testing.T) {
	store, err := persistence.NewStore(t.TempDir())
	require.NoError(t, err)

	svc := newStubService(t, &stubBackend{handle: newStubHandle()}, nil)
	svc.SetPersistence(store)

	done := make(chan error, 1)
	go func() { done <- svc.Close() }()

	select {
	case closeErr := <-done:
		assert.NoError(t, closeErr)
	case <-time.After(10 * time.Second):
		t.Fatal("Close() blocked: it holds s.mu.Lock() and SaveRegistry() asks for s.mu.RLock() on the same mutex")
	}
}

// Close must still flush what it was asked to persist. The deadlock fix must
// not be "skip the save" — that would silently drop torrent ownership on exit,
// which is exactly the regression bug-0051 was written to prevent.
func TestClose_PersistsBeforeReturning(t *testing.T) {
	store, err := persistence.NewStore(t.TempDir())
	require.NoError(t, err)

	svc := newStubService(t, &stubBackend{handle: newStubHandle()}, nil)
	svc.SetPersistence(store)

	_, err = svc.AddMagnet(context.Background(), "owner-peer", testMagnet)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- svc.Close() }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Close() blocked")
	}

	owners, err := store.LoadTorrents()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"abc123": "owner-peer"}, owners.Owners,
		"Close() returned without flushing ownership to disk")
}
