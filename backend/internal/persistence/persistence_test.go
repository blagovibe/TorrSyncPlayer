// SPDX-License-Identifier: MIT
package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	t.Run("creates non-existent directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		nonExistentDir := filepath.Join(tmpDir, "nested", "dir")
		store, err := NewStore(nonExistentDir)
		assert.NoError(t, err)
		assert.NotNil(t, store)
	})

	t.Run("uses existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		assert.NoError(t, err)
		assert.NotNil(t, store)
	})
}

func TestAtomicWrites(t *testing.T) {
	// The property under test is the write path itself — write to a temp file,
	// then rename over the target — not any particular payload. It used to
	// used to exercise the users store; when the unused account code was
	// removed (deviation-0008) the check moved to the data that persists.
	t.Run("temp file is cleaned up after a successful write", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		ownership := &TorrentOwnershipData{
			Owners: map[string]string{"abc123": "user-1"},
		}

		err = store.SaveTorrents(ownership)
		require.NoError(t, err)

		assert.NoFileExists(t, filepath.Join(tmpDir, "torrents.json.tmp"),
			"the temp file must be renamed away, not left behind")
		assert.FileExists(t, filepath.Join(tmpDir, "torrents.json"))
	})

	t.Run("target file is not left readable by other users", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		// The registry maps torrents to owners, so the file must not
		// be world-readable.
		err = store.SaveTorrents(&TorrentOwnershipData{Owners: map[string]string{
			"abc123": "user-1",
		}})
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(tmpDir, "torrents.json"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	})
}

func TestPersistenceConcurrency(t *testing.T) {

	t.Run("concurrent save and load of the registry", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		ownership := &TorrentOwnershipData{
			Owners: map[string]string{"abc123": "user-1"},
		}

		done := make(chan struct{})
		go func() {
			for i := 0; i < 50; i++ {
				_ = store.SaveTorrents(ownership)
			}
			close(done)
		}()

		for i := 0; i < 50; i++ {
			_, _ = store.LoadTorrents()
		}

		<-done
		loaded, err := store.LoadTorrents()
		require.NoError(t, err)
		assert.Equal(t, 1, len(loaded.Owners))
	})
}
