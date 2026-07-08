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
