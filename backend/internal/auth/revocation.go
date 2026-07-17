// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package auth

import (
	"fmt"
	"sync"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/persistence"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// TokenRevocationStore stores revoked JWT tokens.
// Uses in-memory storage with TTL for automatic cleanup.
// Thread-safe for concurrent access from multiple goroutines.
// Optionally persists data to a JSON file when PersistDir is specified.
type TokenRevocationStore struct {
	mu            sync.RWMutex
	revokedTokens map[string]time.Time // jti -> expiry time
	ttl           time.Duration
	stopChan      chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup // for waiting on cleanup goroutine completion
	persistDir    string
	persistor     *persistence.Store
	persistTimer  *time.Timer
	dirty         bool
}

// NewTokenRevocationStore creates a new token revocation store.
// Starts a background goroutine for cleaning expired entries.
// Call Stop() after use for proper shutdown.
func NewTokenRevocationStore() *TokenRevocationStore {
	store := &TokenRevocationStore{
		revokedTokens: make(map[string]time.Time),
		ttl:           constants.RevocationStoreTTL, // Keep revoked tokens for 24 hours
		stopChan:      make(chan struct{}),
	}

	// Start periodic cleanup
	store.wg.Add(1)
	go store.cleanup()

	return store
}

// cleanup periodically removes expired revocation entries.
// Stops when Stop() is called or stopChan is closed.
func (s *TokenRevocationStore) cleanup() {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			logger.Error("TokenRevocationStore: cleanup goroutine exited with panic", "error", r)
		}
	}()
	ticker := time.NewTicker(constants.RevocationCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for jti, expiry := range s.revokedTokens {
				if now.After(expiry) {
					delete(s.revokedTokens, jti)
				}
			}
			s.mu.Unlock()
		case <-s.stopChan:
			return
		}
	}
}

// SetPersistence enables JSON-file persistence.
// dataDir - directory for storing files. Loads data from file during initialization.
func (s *TokenRevocationStore) SetPersistence(dataDir string) error {
	p, err := persistence.NewStore(dataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize persistence: %w", err)
	}

	data, err := p.LoadRevokedTokens()
	if err != nil {
		logger.Warn("auth: failed to load revoked tokens from disk", "error", err)
	} else {
		s.mu.Lock()
		for jti, expiryUnix := range data.RevokedTokens {
			s.revokedTokens[jti] = time.Unix(expiryUnix, 0)
		}
		s.mu.Unlock()
		logger.Info("auth: loaded revoked tokens from disk", "count", len(data.RevokedTokens))
	}

	s.persistDir = dataDir
	s.persistor = p
	return nil
}

func (s *TokenRevocationStore) persist() {
	if s.persistor == nil {
		return
	}
	data := &persistence.TokenRevocationData{
		RevokedTokens: make(map[string]int64, len(s.revokedTokens)),
	}
	for jti, expiry := range s.revokedTokens {
		data.RevokedTokens[jti] = expiry.Unix()
	}
	if err := s.persistor.SaveRevokedTokens(data); err != nil {
		logger.Error("auth: failed to persist revoked tokens", "error", err)
	}
}

// schedulePersist schedules a debounced persist.
// The actual write to disk happens at most once per debounce interval.
func (s *TokenRevocationStore) schedulePersist() {
	s.mu.Lock()
	s.dirty = true
	if s.persistTimer == nil {
		s.persistTimer = time.AfterFunc(constants.P2PDebounceInterval, func() {
			s.mu.Lock()
			if s.dirty && s.persistor != nil {
				s.dirty = false
				s.mu.Unlock()
				s.persist()
				s.mu.Lock()
				s.persistTimer = nil
				s.mu.Unlock()
				return
			}
			if s.persistTimer != nil {
				s.persistTimer = nil
			}
			s.mu.Unlock()
		})
	}
	s.mu.Unlock()
}

// Revoke revokes a token by its JTI.
// expiry - token expiration time (for automatic cleanup).
func (s *TokenRevocationStore) Revoke(jti string, expiry time.Time) {
	s.mu.Lock()
	s.revokedTokens[jti] = expiry
	s.mu.Unlock()
	s.schedulePersist()
}

// IsRevoked checks if a token is revoked.
// Uses RLock for fast check first, then Lock only if deletion is needed.
func (s *TokenRevocationStore) IsRevoked(jti string) bool {
	s.mu.RLock()
	expiry, exists := s.revokedTokens[jti]
	s.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(expiry) {
		s.mu.Lock()
		// Double-check: someone might have already deleted the entry
		if e, ok := s.revokedTokens[jti]; ok && time.Now().After(e) {
			delete(s.revokedTokens, jti)
		}
		s.mu.Unlock()
		return false
	}

	return true
}

// Count returns the number of revoked tokens.
func (s *TokenRevocationStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.revokedTokens)
}

// Stop stops the cleanup goroutine and waits for its completion.
// Safe to call multiple times.
func (s *TokenRevocationStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})

	if s.persistTimer != nil {
		s.persistTimer.Stop()
	}

	// Flush pending persist on shutdown
	s.mu.Lock()
	if s.dirty && s.persistor != nil {
		s.dirty = false
		s.mu.Unlock()
		s.persist()
	} else {
		s.mu.Unlock()
	}

	s.wg.Wait()
}
