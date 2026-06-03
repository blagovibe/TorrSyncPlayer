// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package auth

import (
	"sync"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
)

// TokenRevocationStore хранит отозванные JWT токены.
// Использует in-memory хранилище с TTL для автоматической очистки.
type TokenRevocationStore struct {
	mu            sync.RWMutex
	revokedTokens map[string]time.Time // jti -> expiry time
	ttl           time.Duration
	stopChan      chan struct{}
	stopOnce      sync.Once
}

// NewTokenRevocationStore создаёт новое хранилище отозванных токенов.
func NewTokenRevocationStore() *TokenRevocationStore {
	store := &TokenRevocationStore{
		revokedTokens: make(map[string]time.Time),
		ttl:           constants.RevocationStoreTTL, // Храним отозванные токены 24 часа
		stopChan:      make(chan struct{}),
	}

	// Запускаем периодическую очистку
	go store.cleanup()

	return store
}

// cleanup периодически удаляет истёкшие записи об отзыве.
func (s *TokenRevocationStore) cleanup() {
	ticker := time.NewTicker(constants.CSRFCleanupInterval)
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

// Revoke отзывает токен по его JTI.
// expiry - время истечения токена (для автоматической очистки).
func (s *TokenRevocationStore) Revoke(jti string, expiry time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokedTokens[jti] = expiry
}

// IsRevoked проверяет, отозван ли токен.
func (s *TokenRevocationStore) IsRevoked(jti string) bool {
	s.mu.RLock()
	expiry, exists := s.revokedTokens[jti]
	s.mu.RUnlock()

	if !exists {
		return false
	}

	// Если токен уже истёк, удаляем запись
	if time.Now().After(expiry) {
		s.mu.Lock()
		delete(s.revokedTokens, jti)
		s.mu.Unlock()
		return false
	}

	return true
}

// Count возвращает количество отозванных токенов.
func (s *TokenRevocationStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.revokedTokens)
}

// Stop останавливает горутину очистки.
// Безопасно вызывать несколько раз.
func (s *TokenRevocationStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
}

// Глобальное хранилище отозванных токенов
var revocationStore = NewTokenRevocationStore()

// GetRevocationStore возвращает глобальное хранилище отозванных токенов.
func GetRevocationStore() *TokenRevocationStore {
	return revocationStore
}

// SetRevocationStore устанавливает хранилище отозванных токенов.
// Используется для тестирования.
func SetRevocationStore(store *TokenRevocationStore) {
	revocationStore = store
}
