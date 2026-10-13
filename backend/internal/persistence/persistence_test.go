// SPDX-License-Identifier: MIT
package persistence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
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

		rooms := &RoomsData{
			Rooms: map[string]*RoomSnapshot{
				"room-1": {ID: "room-1", Name: "Test Room", HostID: "host-1", CreatedAt: 1},
			},
		}

		err = store.SaveRooms(rooms)
		require.NoError(t, err)

		assert.NoFileExists(t, filepath.Join(tmpDir, "rooms.json.tmp"),
			"the temp file must be renamed away, not left behind")
		assert.FileExists(t, filepath.Join(tmpDir, "rooms.json"))
	})

	t.Run("target file is not left readable by other users", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		// Room metadata includes the room password hash, so the file must not
		// be world-readable.
		err = store.SaveRooms(&RoomsData{Rooms: map[string]*RoomSnapshot{
			"room-1": {ID: "room-1", Name: "Test Room", HostID: "host-1", Password: "hash", CreatedAt: 1},
		}})
		require.NoError(t, err)

		info, err := os.Stat(filepath.Join(tmpDir, "rooms.json"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	})
}

func TestSaveAndLoadRooms(t *testing.T) {
	t.Run("round-trip preserves rooms", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		now := time.Now().Unix()
		data := &RoomsData{
			Rooms: map[string]*RoomSnapshot{
				"room-1": {
					ID:         "room-1",
					Name:       "Test Room",
					HostID:     "host-1",
					HostUserID: "user-1",
					Password:   "secret",
					CreatedAt:  now,
				},
				"room-2": {
					ID:         "room-2",
					Name:       "Another Room",
					HostID:     "host-2",
					HostUserID: "user-2",
					CreatedAt:  now + 1,
				},
			},
		}

		err = store.SaveRooms(data)
		require.NoError(t, err)

		loaded, err := store.LoadRooms()
		require.NoError(t, err)
		assert.Equal(t, data.Rooms, loaded.Rooms)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles non-existent file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		loaded, err := store.LoadRooms()
		require.NoError(t, err)
		assert.Empty(t, loaded.Rooms)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles corrupted file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "rooms.json"), []byte("invalid json"), 0600)
		require.NoError(t, err)

		loaded, err := store.LoadRooms()
		require.NoError(t, err)
		assert.Empty(t, loaded.Rooms)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles empty rooms map", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		data := &RoomsData{
			Rooms: map[string]*RoomSnapshot{},
		}

		err = store.SaveRooms(data)
		require.NoError(t, err)

		loaded, err := store.LoadRooms()
		require.NoError(t, err)
		assert.Empty(t, loaded.Rooms)
	})
}

func TestSaveAndLoadSync(t *testing.T) {
	t.Run("round-trip preserves sync state", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		data := &SyncData{
			Status: map[string]models.SyncStatus{
				"room-1": {
					IsPlaying: true,
					Position:  42.5,
				},
				"room-2": {
					IsPlaying: false,
					Position:  0,
				},
			},
		}

		err = store.SaveSync(data)
		require.NoError(t, err)

		loaded, err := store.LoadSync()
		require.NoError(t, err)
		assert.Equal(t, data.Status, loaded.Status)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles non-existent file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		loaded, err := store.LoadSync()
		require.NoError(t, err)
		assert.Empty(t, loaded.Status)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles corrupted file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(tmpDir, "sync.json"), []byte("invalid json"), 0600)
		require.NoError(t, err)

		loaded, err := store.LoadSync()
		require.NoError(t, err)
		assert.Empty(t, loaded.Status)
		assert.Equal(t, PersistenceVersion, loaded.Version)
	})

	t.Run("handles empty status map", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		data := &SyncData{
			Status: map[string]models.SyncStatus{},
		}

		err = store.SaveSync(data)
		require.NoError(t, err)

		loaded, err := store.LoadSync()
		require.NoError(t, err)
		assert.Empty(t, loaded.Status)
	})
}

func TestPersistenceConcurrency(t *testing.T) {

	t.Run("concurrent save and load rooms", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		roomData := &RoomsData{
			Rooms: map[string]*RoomSnapshot{
				"room-1": {ID: "room-1", Name: "Room 1", HostID: "host-1", CreatedAt: 1},
			},
		}

		done := make(chan struct{})
		go func() {
			for i := 0; i < 50; i++ {
				_ = store.SaveRooms(roomData)
			}
			close(done)
		}()

		for i := 0; i < 50; i++ {
			_, _ = store.LoadRooms()
		}

		<-done
		loaded, err := store.LoadRooms()
		require.NoError(t, err)
		assert.Equal(t, 1, len(loaded.Rooms))
	})
}
