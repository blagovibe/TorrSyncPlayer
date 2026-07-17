// SPDX-License-Identifier: MIT
package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// PersistenceVersion is the current format version for migration handling
const PersistenceVersion = 1

type UserData struct {
	Version   int                     `json:"version"`
	Users     map[string]*models.User `json:"users"`
	UsersByID map[string]*models.User `json:"usersByID"`
}

type TokenRevocationData struct {
	Version       int              `json:"version"`
	RevokedTokens map[string]int64 `json:"revokedTokens"`
}

// RoomSnapshot is the serializable subset of a room. Live peer connections
// (SSE channels) cannot be persisted; only room metadata and the host are
// saved so a room can be re-created after a restart.
type RoomSnapshot struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	HostID     string `json:"hostId"`
	HostUserID string `json:"hostUserId"`
	Password   string `json:"password"`
	CreatedAt  int64  `json:"createdAt"`
}

type RoomsData struct {
	Version int                      `json:"version"`
	Rooms   map[string]*RoomSnapshot `json:"rooms"`
}

type SyncData struct {
	Version int                          `json:"version"`
	Status  map[string]models.SyncStatus `json:"status"`
}

type Store struct {
	dir string
	mu  sync.RWMutex
}

func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}
	return &Store{dir: dataDir}, nil
}

// persistedUser is an internal representation for JSON serialization.
// Unlike models.User, it includes PasswordHash for persistence.
type persistedUser struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"passwordHash"`
	CreatedAt    int64  `json:"createdAt"`
}

// toPersistedUser converts a models.User to persistedUser for serialization.
func toPersistedUser(u *models.User) *persistedUser {
	return &persistedUser{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
	}
}

// fromPersistedUser converts a persistedUser back to models.User.
func fromPersistedUser(p *persistedUser) *models.User {
	return &models.User{
		ID:           p.ID,
		Username:     p.Username,
		PasswordHash: p.PasswordHash,
		CreatedAt:    p.CreatedAt,
	}
}

func (s *Store) SaveUsers(data *UserData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set version for future migration support
	data.Version = PersistenceVersion

	path := filepath.Join(s.dir, "users.json")
	tmpPath := path + ".tmp"

	// Convert users to persistedUser format for serialization
	persistedData := struct {
		Version   int                       `json:"version"`
		Users     map[string]*persistedUser `json:"users"`
		UsersByID map[string]*persistedUser `json:"usersByID"`
	}{
		Version: PersistenceVersion,
	}
	persistedData.Users = make(map[string]*persistedUser, len(data.Users))
	persistedData.UsersByID = make(map[string]*persistedUser, len(data.UsersByID))
	for k, v := range data.Users {
		persistedData.Users[k] = toPersistedUser(v)
	}
	for k, v := range data.UsersByID {
		persistedData.UsersByID[k] = toPersistedUser(v)
	}

	encoded, err := json.Marshal(persistedData)
	if err != nil {
		return fmt.Errorf("failed to marshal users: %w", err)
	}

	if err := os.WriteFile(tmpPath, encoded, 0600); err != nil {
		return fmt.Errorf("failed to write users file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if rmErr := os.Remove(tmpPath); rmErr != nil {
			logger.Error("persistence: failed to remove temp file", "path", tmpPath, "error", rmErr)
		}
		return fmt.Errorf("failed to rename users file: %w", err)
	}

	return nil
}

func (s *Store) LoadUsers() (*UserData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.dir, "users.json")
	data, err := os.ReadFile(path) //nolint:gosec // path is under data directory
	if err != nil {
		if os.IsNotExist(err) {
			return &UserData{
				Version:   PersistenceVersion,
				Users:     make(map[string]*models.User),
				UsersByID: make(map[string]*models.User),
			}, nil
		}
		return nil, fmt.Errorf("failed to read users file: %w", err)
	}

	// Use internal type for deserialization to include PasswordHash
	var persistedData struct {
		Version   int                       `json:"version"`
		Users     map[string]*persistedUser `json:"users"`
		UsersByID map[string]*persistedUser `json:"usersByID"`
	}
	if err := json.Unmarshal(data, &persistedData); err != nil {
		logger.Warn("persistence: corrupted users.json, starting fresh", "error", err)
		return &UserData{
			Version:   PersistenceVersion,
			Users:     make(map[string]*models.User),
			UsersByID: make(map[string]*models.User),
		}, nil
	}

	result := &UserData{
		Version:   persistedData.Version,
		Users:     make(map[string]*models.User),
		UsersByID: make(map[string]*models.User),
	}

	// Convert back to models.User
	for k, v := range persistedData.Users {
		result.Users[k] = fromPersistedUser(v)
	}
	for k, v := range persistedData.UsersByID {
		result.UsersByID[k] = fromPersistedUser(v)
	}

	// Version check for migration (future-proofing)
	if result.Version == 0 {
		result.Version = PersistenceVersion // Assume v1 for legacy files without version
	}

	if result.Users == nil {
		result.Users = make(map[string]*models.User)
	}
	if result.UsersByID == nil {
		result.UsersByID = make(map[string]*models.User)
	}

	return result, nil
}

func (s *Store) SaveRevokedTokens(data *TokenRevocationData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Set version for future migration support
	data.Version = PersistenceVersion

	path := filepath.Join(s.dir, "revoked_tokens.json")
	tmpPath := path + ".tmp"

	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal revoked tokens: %w", err)
	}

	if err := os.WriteFile(tmpPath, encoded, 0600); err != nil {
		return fmt.Errorf("failed to write revoked tokens file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if rmErr := os.Remove(tmpPath); rmErr != nil {
			logger.Error("persistence: failed to remove temp file", "path", tmpPath, "error", rmErr)
		}
		return fmt.Errorf("failed to rename revoked tokens file: %w", err)
	}

	return nil
}

func (s *Store) LoadRevokedTokens() (*TokenRevocationData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.dir, "revoked_tokens.json")
	data, err := os.ReadFile(path) //nolint:gosec // path is under data directory
	if err != nil {
		if os.IsNotExist(err) {
			return &TokenRevocationData{
				Version:       PersistenceVersion,
				RevokedTokens: make(map[string]int64),
			}, nil
		}
		return nil, fmt.Errorf("failed to read revoked tokens file: %w", err)
	}

	var result TokenRevocationData
	if err := json.Unmarshal(data, &result); err != nil {
		logger.Warn("persistence: corrupted revoked_tokens.json, starting fresh", "error", err)
		return &TokenRevocationData{
			Version:       PersistenceVersion,
			RevokedTokens: make(map[string]int64),
		}, nil
	}

	// Version check for migration (future-proofing)
	if result.Version == 0 {
		result.Version = PersistenceVersion // Assume v1 for legacy files without version
	}

	if result.RevokedTokens == nil {
		result.RevokedTokens = make(map[string]int64)
	}

	return &result, nil
}

// SaveRooms persists room metadata atomically to rooms.json.
func (s *Store) SaveRooms(data *RoomsData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data.Version = PersistenceVersion

	path := filepath.Join(s.dir, "rooms.json")
	tmpPath := path + ".tmp"

	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal rooms: %w", err)
	}

	if err := os.WriteFile(tmpPath, encoded, 0600); err != nil {
		return fmt.Errorf("failed to write rooms file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if rmErr := os.Remove(tmpPath); rmErr != nil {
			logger.Error("persistence: failed to remove temp file", "path", tmpPath, "error", rmErr)
		}
		return fmt.Errorf("failed to rename rooms file: %w", err)
	}

	return nil
}

// LoadRooms loads room metadata from rooms.json, tolerating a missing or
// corrupted file by returning an empty collection.
func (s *Store) LoadRooms() (*RoomsData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.dir, "rooms.json")
	data, err := os.ReadFile(path) //nolint:gosec // path is under data directory
	if err != nil {
		if os.IsNotExist(err) {
			return &RoomsData{Version: PersistenceVersion, Rooms: make(map[string]*RoomSnapshot)}, nil
		}
		return nil, fmt.Errorf("failed to read rooms file: %w", err)
	}

	var result RoomsData
	if err := json.Unmarshal(data, &result); err != nil {
		logger.Warn("persistence: corrupted rooms.json, starting fresh", "error", err)
		return &RoomsData{Version: PersistenceVersion, Rooms: make(map[string]*RoomSnapshot)}, nil
	}

	if result.Version == 0 {
		result.Version = PersistenceVersion
	}
	if result.Rooms == nil {
		result.Rooms = make(map[string]*RoomSnapshot)
	}

	return &result, nil
}

// SaveSync persists per-room playback state atomically to sync.json.
func (s *Store) SaveSync(data *SyncData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data.Version = PersistenceVersion

	path := filepath.Join(s.dir, "sync.json")
	tmpPath := path + ".tmp"

	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal sync state: %w", err)
	}

	if err := os.WriteFile(tmpPath, encoded, 0600); err != nil {
		return fmt.Errorf("failed to write sync file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if rmErr := os.Remove(tmpPath); rmErr != nil {
			logger.Error("persistence: failed to remove temp file", "path", tmpPath, "error", rmErr)
		}
		return fmt.Errorf("failed to rename sync file: %w", err)
	}

	return nil
}

// LoadSync loads per-room playback state from sync.json, tolerating a missing
// or corrupted file by returning an empty collection.
func (s *Store) LoadSync() (*SyncData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.dir, "sync.json")
	data, err := os.ReadFile(path) //nolint:gosec // path is under data directory
	if err != nil {
		if os.IsNotExist(err) {
			return &SyncData{Version: PersistenceVersion, Status: make(map[string]models.SyncStatus)}, nil
		}
		return nil, fmt.Errorf("failed to read sync file: %w", err)
	}

	var result SyncData
	if err := json.Unmarshal(data, &result); err != nil {
		logger.Warn("persistence: corrupted sync.json, starting fresh", "error", err)
		return &SyncData{Version: PersistenceVersion, Status: make(map[string]models.SyncStatus)}, nil
	}

	if result.Version == 0 {
		result.Version = PersistenceVersion
	}
	if result.Status == nil {
		result.Status = make(map[string]models.SyncStatus)
	}

	return &result, nil
}
