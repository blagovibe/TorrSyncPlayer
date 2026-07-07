// SPDX-License-Identifier: MIT

// Package auth provides user store.
package auth

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/persistence"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/utils"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

var (
	dummyHashOnce sync.Once
	dummyHashVal  []byte
)

func getDummyHash() []byte {
	dummyHashOnce.Do(func() {
		hash, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing-mitigation"), constants.BcryptCost)
		if err != nil {
			logger.Error("failed to generate dummy bcrypt hash", "error", err)
			dummyHashVal = nil
		} else {
			dummyHashVal = hash
		}
	})
	return dummyHashVal
}

// UserStore in-memory user store.
// In production, a database should be used.
// Optionally persists data to a JSON file when PersistDir is specified.
type UserStore struct {
	mu         sync.RWMutex
	users      map[string]*models.User // key: username (lowercase)
	usersByID  map[string]*models.User // secondary index: user ID -> user
	persistDir string
	persistor  *persistence.Store
}

// NewUserStore creates a new user store.
func NewUserStore() *UserStore {
	return &UserStore{
		users:     make(map[string]*models.User),
		usersByID: make(map[string]*models.User),
	}
}

// SetPersistence enables JSON-file persistence.
// dataDir - directory for storing files. Created if necessary.
// Loads data from file during initialization.
func (s *UserStore) SetPersistence(dataDir string) error {
	p, err := persistence.NewStore(dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize persistence: %w", err)
	}

	data, err := p.LoadUsers()
	if err != nil {
		logger.Warn("auth: failed to load users from disk", "error", err)
	} else {
		s.mu.Lock()
		s.users = data.Users
		s.usersByID = data.UsersByID
		s.mu.Unlock()
		logger.Info("auth: loaded users from disk", "count", len(data.Users))
	}

	s.persistDir = dataDir
	s.persistor = p
	return nil
}

func (s *UserStore) persist() {
	if s.persistor == nil {
		return
	}
	data := &persistence.UserData{
		Users:     s.users,
		UsersByID: s.usersByID,
	}
	if err := s.persistor.SaveUsers(data); err != nil {
		logger.Error("auth: failed to persist users", "error", err)
	}
}

// Create creates a new user.
// Returns an error if the user already exists.
func (s *UserStore) Create(username, password string) (*models.User, error) {
	// Validate username
	if err := validation.ValidateUsername(username); err != nil {
		return nil, err
	}

	username = strings.ToLower(username)

	// Validate password
	if err := validation.ValidatePassword(password); err != nil {
		return nil, err
	}

	s.mu.Lock()
	if _, exists := s.users[username]; exists {
		s.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrUserExists, username)
	}
	s.mu.Unlock()

	passwordHash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("password hashing error: %w", err)
	}

	id, err := utils.GenerateID(constants.UserIDBytes)
	if err != nil {
		return nil, fmt.Errorf("ID generation error: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[username]; exists {
		return nil, fmt.Errorf("%w: %s", ErrUserExists, username)
	}

	user := &models.User{
		ID:           id,
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().Unix(),
	}

	s.users[username] = user
	s.usersByID[user.ID] = user

	s.persist()
	return user, nil
}

// Authenticate verifies user credentials.
// Returns the user if credentials are valid.
// Always performs both hash comparisons (dummy + real) for timing attack protection.
func (s *UserStore) Authenticate(username, password string) (*models.User, error) {
	username = strings.ToLower(username)
	s.mu.RLock()
	user, exists := s.users[username]
	s.mu.RUnlock()

	if !exists {
		if dummyHash := getDummyHash(); dummyHash != nil {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		}
		return nil, ErrInvalidCredentialsAppError
	}

	if err := CheckPassword(password, user.PasswordHash); err != nil {
		return nil, err
	}

	return user, nil
}

// GetByUsername returns a user by username (case-insensitive).
func (s *UserStore) GetByUsername(username string) (*models.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[strings.ToLower(username)]
	return user, exists
}

// GetByID returns a user by ID (using index, O(1)).
func (s *UserStore) GetByID(id string) (*models.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.usersByID[id]
	return user, exists
}

// ChangePassword changes a user's password.
// Requires the current password to be verified before changing.
// Returns an error if the current password is incorrect or user doesn't exist.
func (s *UserStore) ChangePassword(username, currentPassword, newPassword string) error {
	username = strings.ToLower(username)

	// Validate new password
	if err := validation.ValidatePassword(newPassword); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[username]
	if !exists {
		// Still perform dummy hash comparison for timing attack protection
		if dummyHash := getDummyHash(); dummyHash != nil {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(currentPassword))
		}
		return ErrInvalidCredentialsAppError
	}

	// Verify current password
	if err := CheckPassword(currentPassword, user.PasswordHash); err != nil {
		return err
	}

	// Hash and update new password
	passwordHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("password hashing error: %w", err)
	}

	user.PasswordHash = passwordHash
	s.persist()
	return nil
}
