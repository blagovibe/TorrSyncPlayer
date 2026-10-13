// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bug-0051: torrent ownership survives restart.
//
// Torrent ownership (bug-0049) and room-scoped read access (bug-0049 layer 2)
// both depend on torrentOwners map. Without persistence, a restart forgets
// who added what, making all torrents unowned and therefore readable/deletable
// by everyone — exactly the regression bug-0049 fixed.

func TestSaveAndLoadTorrentOwnership(t *testing.T) {
	t.Run("round-trip preserves ownership", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		data := &TorrentOwnershipData{
			Owners: map[string]string{
				"abc123": "user-a",
				"def456": "user-b",
			},
		}

		err = store.SaveTorrents(data)
		require.NoError(t, err)

		loaded, err := store.LoadTorrents()
		require.NoError(t, err)
		assert.Equal(t, data.Owners, loaded.Owners)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles non-existent file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		loaded, err := store.LoadTorrents()
		require.NoError(t, err)
		assert.Empty(t, loaded.Owners)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles corrupted file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "torrents.json"), []byte("invalid json"), 0600)
		require.NoError(t, err)

		loaded, err := store.LoadTorrents()
		require.NoError(t, err)
		assert.Empty(t, loaded.Owners)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles empty owners map", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		data := &TorrentOwnershipData{Owners: map[string]string{}}

		err = store.SaveTorrents(data)
		require.NoError(t, err)

		loaded, err := store.LoadTorrents()
		require.NoError(t, err)
		assert.Empty(t, loaded.Owners)
	})
}
