// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package sync provides the playback synchronization service.
// Ensures playback position synchronization between peers with network latency compensation.
// Uses structured logging with operation context.
package sync

import (
	"math"
	"sync"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/persistence"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

type Service struct {
	mu            sync.RWMutex
	rooms         map[string]models.SyncStatus
	isClosed      bool
	closeOnce     sync.Once
	persistence   *persistence.Store
	saveDebouncer *time.Timer
}

// NewService creates a new synchronization service.
// State is tracked per room so concurrent rooms do not overwrite each other.
// Returns an initialized service.
func NewService() *Service {
	svc := &Service{
		rooms: make(map[string]models.SyncStatus),
	}

	logger.Info("Sync: service initialized")
	return svc
}

// SetPersistence enables JSON-file persistence of per-room playback state.
// When set, sync status survives a server restart. Without it, state is
// in-memory only.
func (s *Service) SetPersistence(store *persistence.Store) {
	s.mu.Lock()
	s.persistence = store
	s.mu.Unlock()

	if data, err := store.LoadSync(); err == nil {
		s.mu.Lock()
		for roomID, status := range data.Status {
			s.rooms[roomID] = status
		}
		s.mu.Unlock()
		logger.Info("Sync: restored playback state from persistence", "rooms", len(data.Status))
	} else {
		logger.Warn("Sync: failed to load persisted sync state", "error", err)
	}
}

// scheduleSave persists playback state on a debounce timer.
func (s *Service) scheduleSave() {
	if s.persistence == nil {
		return
	}
	s.mu.Lock()
	if s.saveDebouncer != nil {
		s.saveDebouncer.Stop()
	}
	s.saveDebouncer = time.AfterFunc(constants.P2PDebounceInterval, s.flushState)
	s.mu.Unlock()
}

// flushState writes the current playback state to disk.
func (s *Service) flushState() {
	s.mu.RLock()
	data := &persistence.SyncData{Status: make(map[string]models.SyncStatus, len(s.rooms))}
	for roomID, status := range s.rooms {
		data.Status[roomID] = status
	}
	store := s.persistence
	s.mu.RUnlock()

	if store == nil {
		return
	}
	if err := store.SaveSync(data); err != nil {
		logger.Warn("Sync: failed to persist playback state", "error", err)
	}
}

// getRoom returns the sync status for a room, creating an initial entry (stopped,
// position 0) on first access. Caller must NOT hold the lock.
func (s *Service) getRoom(roomID string) models.SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureRoomLocked(roomID)
}

// ensureRoomLocked returns the sync status for a room, creating it if missing.
// Caller MUST hold the write lock.
func (s *Service) ensureRoomLocked(roomID string) models.SyncStatus {
	status, ok := s.rooms[roomID]
	if !ok {
		status = models.SyncStatus{
			IsPlaying: false,
			Position:  0,
			Duration:  0,
			Timestamp: time.Now().UnixMilli(),
		}
		s.rooms[roomID] = status
	}
	return status
}

// Play starts playback for the specified room.
// Sets IsPlaying flag to true and updates the timestamp.
// Returns the current synchronization status.
func (s *Service) Play(roomID string) models.SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to play on a closed service")
		return s.rooms[roomID]
	}

	status := s.ensureRoomLocked(roomID)
	status.IsPlaying = true
	status.Timestamp = time.Now().UnixMilli()
	s.rooms[roomID] = status
	s.scheduleSave()

	logger.Info("Sync: playback started", "roomID", roomID, "position", status.Position)
	return status
}

// Pause pauses playback for the specified room.
// Sets IsPlaying flag to false and updates the timestamp.
// Returns the current synchronization status.
func (s *Service) Pause(roomID string) models.SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to pause on a closed service")
		return s.rooms[roomID]
	}

	status := s.ensureRoomLocked(roomID)
	status.IsPlaying = false
	status.Timestamp = time.Now().UnixMilli()
	s.rooms[roomID] = status
	s.scheduleSave()

	logger.Info("Sync: playback paused", "roomID", roomID, "position", status.Position)
	return status
}

// Seek seeks to the specified position in the given room.
// Parameter position - position in seconds (0 - 86400).
// Validates position before applying. Large jumps (greater than
// MaxPositionJump) are smoothed toward the requested position using
// SmoothAdjustmentRatio to avoid visual glitches and desync caused by
// network latency or out-of-order events.
// Returns the updated status or an error if position is invalid.
func (s *Service) Seek(roomID string, position float64) (models.SyncStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to seek on a closed service")
		return s.rooms[roomID], errors.New(errors.ErrUnavailable, "service closed")
	}

	// Validate position
	if err := validation.ValidatePosition(position); err != nil {
		logger.Warn("Sync: invalid seek position", "position", position, "error", err)
		return s.rooms[roomID], errors.Wrap(errors.ErrInvalidInput, "invalid position", err)
	}

	status := s.ensureRoomLocked(roomID)
	oldPosition := status.Position
	status.Position = applyLatencyCompensation(oldPosition, position)
	status.Timestamp = time.Now().UnixMilli()
	s.rooms[roomID] = status
	s.scheduleSave()

	logger.Info("Sync: seek", "roomID", roomID, "oldPosition", oldPosition,
		"requestedPosition", position, "appliedPosition", status.Position,
		"smoothed", math.Abs(position-oldPosition) > constants.MaxPositionJump)
	return status, nil
}

// applyLatencyCompensation smooths a requested seek position relative to the
// current position. If the jump is within MaxPositionJump it is applied
// directly; otherwise the applied position is moved only a fraction
// (SmoothAdjustmentRatio) of the way toward the target to avoid abrupt,
// latency-induced desync.
func applyLatencyCompensation(current, requested float64) float64 {
	if math.Abs(requested-current) <= constants.MaxPositionJump {
		return requested
	}
	return current + (requested-current)*constants.SmoothAdjustmentRatio
}

// GetStatus returns the current playback status for the given room.
// Includes position, duration, playback state and timestamp.
func (s *Service) GetStatus(roomID string) models.SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if status, ok := s.rooms[roomID]; ok {
		return status
	}
	return models.SyncStatus{
		IsPlaying: false,
		Position:  0,
		Duration:  0,
		Timestamp: time.Now().UnixMilli(),
	}
}

// Close closes the synchronization service.
// Stops playback and marks the service as closed.
// Safe for multiple calls (uses sync.Once).
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.flushState()

		s.mu.Lock()
		defer s.mu.Unlock()

		s.isClosed = true
		for roomID := range s.rooms {
			status := s.rooms[roomID]
			status.IsPlaying = false
			s.rooms[roomID] = status
		}

		logger.Info("Sync: service stopped")
	})
}
