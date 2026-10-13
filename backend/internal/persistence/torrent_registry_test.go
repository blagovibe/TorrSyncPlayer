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

// bug-0038: torrent registry survives restart.
//
// Torrent metadata (infohash, name, files, size) must be persisted so that
// after a restart the server knows which torrents it has on disk and can
// serve them again. Without this, --disk-storage leaves orphaned pieces.

func TestSaveAndLoadTorrentRegistry(t *testing.T) {
	t.Run("round-trip preserves registry", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		data := &TorrentRegistryData{
			Torrents: map[string]TorrentInfo{
				"abc123": {
					InfoHash:  "abc123",
					Name:      "Test Movie",
					Size:      1024,
					FileCount: 1,
					Files:     []FileInfo{{Index: 0, Name: "movie.mp4", Size: 1024}},
				},
			},
		}

		err = store.SaveTorrentRegistry(data)
		require.NoError(t, err)

		loaded, err := store.LoadTorrentRegistry()
		require.NoError(t, err)
		assert.Equal(t, data.Torrents, loaded.Torrents)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles non-existent file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		loaded, err := store.LoadTorrentRegistry()
		require.NoError(t, err)
		assert.Empty(t, loaded.Torrents)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles corrupted file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "torrents_registry.json"), []byte("invalid json"), 0600)
		require.NoError(t, err)

		loaded, err := store.LoadTorrentRegistry()
		require.NoError(t, err)
		assert.Empty(t, loaded.Torrents)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles empty torrents map", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		data := &TorrentRegistryData{Torrents: map[string]TorrentInfo{}}

		err = store.SaveTorrentRegistry(data)
		require.NoError(t, err)

		loaded, err := store.LoadTorrentRegistry()
		require.NoError(t, err)
		assert.Empty(t, loaded.Torrents)
	})
}
