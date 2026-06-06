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
// Потокобезопасен для одновременного доступа из нескольких горутин.
type TokenRevocationStore struct {
	mu            sync.RWMutex
	revokedTokens map[string]time.Time // jti -> expiry time
	ttl           time.Duration
	stopChan      chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup // для ожидания завершения cleanup горутины
}

// NewTokenRevocationStore создаёт новое хранилище отозванных токенов.
// Запускает фоновую горутину очистки истёкших записей.
// Для корректного завершения вызовите Stop() после использования.
func NewTokenRevocationStore() *TokenRevocationStore {
	store := &TokenRevocationStore{
		revokedTokens: make(map[string]time.Time),
		ttl:           constants.RevocationStoreTTL, // Храним отозванные токены 24 часа
		stopChan:      make(chan struct{}),
	}

	// Запускаем периодическую очистку
	store.wg.Add(1)
	go store.cleanup()

	return store
}

// cleanup периодически удаляет истёкшие записи об отзыве.
// Завершается при вызове Stop() или при закрытии stopChan.
func (s *TokenRevocationStore) cleanup() {
	defer s.wg.Done()
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
// Если токен истёк, удаляет запись и возвращает false.
func (s *TokenRevocationStore) IsRevoked(jti string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	expiry, exists := s.revokedTokens[jti]
	if !exists {
		return false
	}

	if time.Now().After(expiry) {
		delete(s.revokedTokens, jti)
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

// Stop останавливает горутину очистки и ждёт её завершения.
// Безопасно вызывать несколько раз.
func (s *TokenRevocationStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
	s.wg.Wait()
}
