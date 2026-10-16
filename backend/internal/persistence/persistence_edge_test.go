// SPDX-License-Identifier: MIT
package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Version stamping, migration and failure paths. The round-trip tests prove
// the happy path; these prove the edges the happy path cannot see: a save
// that forgot to stamp the version still loads fine (migration covers for
// it), so only reading the file back catches the missing stamp.

func TestVersionIsStampedOnSave(t *testing.T) {
	t.Run("torrents.json carries the version", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		require.NoError(t, store.SaveTorrents(&TorrentOwnershipData{
			Owners: map[string]string{"abc123": "user-1"},
		}))

		raw, err := os.ReadFile(filepath.Join(tmpDir, "torrents.json"))
		require.NoError(t, err)
		var onDisk map[string]any
		require.NoError(t, json.Unmarshal(raw, &onDisk))
		assert.Equal(t, float64(PersistenceVersion), onDisk["version"],
			"the file must carry the version, not rely on load-time migration")
	})

	t.Run("torrents_registry.json carries the version", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		require.NoError(t, store.SaveTorrentRegistry(&TorrentRegistryData{
			Torrents: map[string]TorrentInfo{"abc123": {InfoHash: "abc123"}},
		}))

		raw, err := os.ReadFile(filepath.Join(tmpDir, "torrents_registry.json"))
		require.NoError(t, err)
		var onDisk map[string]any
		require.NoError(t, json.Unmarshal(raw, &onDisk))
		assert.Equal(t, float64(PersistenceVersion), onDisk["version"])
	})
}

func TestVersionZeroMigratesOnLoad(t *testing.T) {
	t.Run("torrents.json without a version migrates", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, "torrents.json"),
			[]byte(`{"owners":{"abc123":"user-1"}}`), 0600))

		loaded, err := store.LoadTorrents()
		require.NoError(t, err)
		assert.Equal(t, PersistenceVersion, loaded.Version)
		assert.Equal(t, map[string]string{"abc123": "user-1"}, loaded.Owners)
	})

	t.Run("torrents_registry.json without a version migrates", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, "torrents_registry.json"),
			[]byte(`{"torrents":{"abc123":{"infoHash":"abc123"}}}`), 0600))

		loaded, err := store.LoadTorrentRegistry()
		require.NoError(t, err)
		assert.Equal(t, PersistenceVersion, loaded.Version)
		assert.Contains(t, loaded.Torrents, "abc123")
	})
}

func TestMissingKeysNormalizeToEmptyMaps(t *testing.T) {
	t.Run("torrents.json without owners", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, "torrents.json"), []byte(`{}`), 0600))

		loaded, err := store.LoadTorrents()
		require.NoError(t, err)
		assert.NotNil(t, loaded.Owners, "callers must not nil-check after load")
		assert.Empty(t, loaded.Owners)
	})

	t.Run("torrents_registry.json without torrents", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, "torrents_registry.json"), []byte(`{}`), 0600))

		loaded, err := store.LoadTorrentRegistry()
		require.NoError(t, err)
		assert.NotNil(t, loaded.Torrents)
		assert.Empty(t, loaded.Torrents)
	})
}

func TestStoreCreationFailsOnAFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0600))

	_, err := NewStore(blocker)
	require.Error(t, err, "a data directory cannot be a file")
}

func TestWriteFailureReportsAnError(t *testing.T) {
	// A directory where the temp file goes makes the write fail, which
	// drives the error path no happy-path test reaches.
	for _, tc := range []struct {
		name string
		file string
		save func(*Store) error
	}{
		{"torrents", "torrents.json.tmp", func(s *Store) error {
			return s.SaveTorrents(&TorrentOwnershipData{Owners: map[string]string{"a": "b"}})
		}},
		{"registry", "torrents_registry.json.tmp", func(s *Store) error {
			return s.SaveTorrentRegistry(&TorrentRegistryData{Torrents: map[string]TorrentInfo{}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			store, err := NewStore(tmpDir)
			require.NoError(t, err)

			require.NoError(t, os.Mkdir(filepath.Join(tmpDir, tc.file), 0700))
			require.Error(t, tc.save(store), "writing over a directory must fail loudly")
		})
	}
}

func TestRenameFailureReportsAnError(t *testing.T) {
	// Renaming a file over a directory fails, which drives the one error
	// path a happy-path test never reaches — and proves the temp file is
	// cleaned up rather than left behind.
	for _, tc := range []struct {
		name string
		file string
		save func(*Store) error
	}{
		{"torrents", "torrents.json", func(s *Store) error {
			return s.SaveTorrents(&TorrentOwnershipData{Owners: map[string]string{"a": "b"}})
		}},
		{"registry", "torrents_registry.json", func(s *Store) error {
			return s.SaveTorrentRegistry(&TorrentRegistryData{Torrents: map[string]TorrentInfo{}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			store, err := NewStore(tmpDir)
			require.NoError(t, err)

			require.NoError(t, os.Mkdir(filepath.Join(tmpDir, tc.file), 0700))
			require.Error(t, tc.save(store), "renaming over a directory must fail loudly")
			assert.NoFileExists(t, filepath.Join(tmpDir, tc.file+".tmp"),
				"the temp file must not survive a failed rename")
		})
	}
}
