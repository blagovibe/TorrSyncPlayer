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

type UserData struct {
	Users     map[string]*models.User `json:"users"`
	UsersByID map[string]*models.User `json:"usersByID"`
}

type TokenRevocationData struct {
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

func (s *Store) SaveUsers(data *UserData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, "users.json")
	tmpPath := path + ".tmp"

	encoded, err := json.Marshal(data)
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
				Users:     make(map[string]*models.User),
				UsersByID: make(map[string]*models.User),
			}, nil
		}
		return nil, fmt.Errorf("failed to read users file: %w", err)
	}

	var result UserData
	if err := json.Unmarshal(data, &result); err != nil {
		logger.Warn("persistence: corrupted users.json, starting fresh", "error", err)
		return &UserData{
			Users:     make(map[string]*models.User),
			UsersByID: make(map[string]*models.User),
		}, nil
	}

	if result.Users == nil {
		result.Users = make(map[string]*models.User)
	}
	if result.UsersByID == nil {
		result.UsersByID = make(map[string]*models.User)
	}

	return &result, nil
}

func (s *Store) SaveRevokedTokens(data *TokenRevocationData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
				RevokedTokens: make(map[string]int64),
			}, nil
		}
		return nil, fmt.Errorf("failed to read revoked tokens file: %w", err)
	}

	var result TokenRevocationData
	if err := json.Unmarshal(data, &result); err != nil {
		logger.Warn("persistence: corrupted revoked_tokens.json, starting fresh", "error", err)
		return &TokenRevocationData{
			RevokedTokens: make(map[string]int64),
		}, nil
	}

	if result.RevokedTokens == nil {
		result.RevokedTokens = make(map[string]int64)
	}

	return &result, nil
}
