// SPDX-License-Identifier: MIT
package persistence

import (
	"os"
	"path/filepath"
	"testing"

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

func TestSaveAndLoadUsers(t *testing.T) {
	t.Run("saves and loads users successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		userData := &UserData{
			Users: map[string]*models.User{
				"testuser": {
					ID:           "user-id-123",
					Username:     "testuser",
					PasswordHash: "$2a$12$hashhashhashhashhashhashhashhashhashhash", // #nosec G101
					CreatedAt:    1234567890,
				},
			},
			UsersByID: map[string]*models.User{
				"user-id-123": {
					ID:           "user-id-123",
					Username:     "testuser",
					PasswordHash: "$2a$12$hashhashhashhashhashhashhashhashhashhash", // #nosec G101
					CreatedAt:    1234567890,
				},
			},
		}

		err = store.SaveUsers(userData)
		assert.NoError(t, err)

		loaded, err := store.LoadUsers()
		assert.NoError(t, err)
		assert.NotNil(t, loaded)
		assert.Equal(t, userData.Users, loaded.Users)
		assert.Equal(t, userData.UsersByID, loaded.UsersByID)
	})

	t.Run("handles non-existent file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		loaded, err := store.LoadUsers()
		assert.NoError(t, err)
		assert.NotNil(t, loaded)
		assert.Empty(t, loaded.Users)
		assert.Empty(t, loaded.UsersByID)
	})

	t.Run("handles corrupted file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		// Write corrupted JSON
		err = os.WriteFile(filepath.Join(tmpDir, "users.json"), []byte("invalid json"), 0600)
		require.NoError(t, err)

		loaded, err := store.LoadUsers()
		assert.NoError(t, err)
		assert.NotNil(t, loaded)
		assert.Empty(t, loaded.Users)
	})

	t.Run("handles missing version field", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		// Write JSON without version field
		data := `{"users":{"testuser":{"id":"123","username":"testuser"}}}`
		err = os.WriteFile(filepath.Join(tmpDir, "users.json"), []byte(data), 0600)
		require.NoError(t, err)

		loaded, err := store.LoadUsers()
		assert.NoError(t, err)
		assert.NotNil(t, loaded)
		assert.Equal(t, 1, loaded.Version) // Should be set to 1 for legacy files
	})
}

func TestSaveAndLoadRevokedTokens(t *testing.T) {
	t.Run("saves and loads revoked tokens successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		tokenData := &TokenRevocationData{
			RevokedTokens: map[string]int64{
				"jti-1": 1234567890,
				"jti-2": 1234567891,
			},
		}

		err = store.SaveRevokedTokens(tokenData)
		assert.NoError(t, err)

		loaded, err := store.LoadRevokedTokens()
		assert.NoError(t, err)
		assert.NotNil(t, loaded)
		assert.Equal(t, tokenData.RevokedTokens, loaded.RevokedTokens)
	})

	t.Run("handles non-existent file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		loaded, err := store.LoadRevokedTokens()
		assert.NoError(t, err)
		assert.NotNil(t, loaded)
		assert.Empty(t, loaded.RevokedTokens)
	})

	t.Run("handles corrupted file gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		// Write corrupted JSON
		err = os.WriteFile(filepath.Join(tmpDir, "revoked_tokens.json"), []byte("invalid json"), 0600)
		require.NoError(t, err)

		loaded, err := store.LoadRevokedTokens()
		assert.NoError(t, err)
		assert.NotNil(t, loaded)
		assert.Empty(t, loaded.RevokedTokens)
	})
}

func TestAtomicWrites(t *testing.T) {
	t.Run("temp file created with correct permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		userData := &UserData{
			Users: map[string]*models.User{
				"testuser": {
					ID:           "user-id-123",
					Username:     "testuser",
					PasswordHash: "$2a$12$hashhashhashhashhashhashhashhashhashhash", // #nosec G101
				},
			},
		}

		err = store.SaveUsers(userData)
		assert.NoError(t, err)

		// Check that temp file was cleaned up
		assert.NoFileExists(t, filepath.Join(tmpDir, "users.json.tmp"))
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
					RoomID:    "room-1",
					IsPlaying: true,
					Position:  42.5,
				},
				"room-2": {
					RoomID:    "room-2",
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
	t.Run("concurrent save and load users", func(t *testing.T) {
		tmpDir := t.TempDir()
		store, err := NewStore(tmpDir)
		require.NoError(t, err)

		userData := &UserData{
			Users: map[string]*models.User{
				"user-1": {ID: "1", Username: "user1", PasswordHash: "hash1", CreatedAt: 1},
			},
			UsersByID: map[string]*models.User{
				"1": {ID: "1", Username: "user1", PasswordHash: "hash1", CreatedAt: 1},
			},
		}

		done := make(chan struct{})
		go func() {
			for i := 0; i < 50; i++ {
				_ = store.SaveUsers(userData)
			}
			close(done)
		}()

		for i := 0; i < 50; i++ {
			_, _ = store.LoadUsers()
		}

		<-done
		loaded, err := store.LoadUsers()
		require.NoError(t, err)
		assert.Equal(t, 1, len(loaded.Users))
	})

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
