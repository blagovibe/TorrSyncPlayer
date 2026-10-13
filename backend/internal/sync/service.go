// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package sync provides the playback synchronization service.
// Ensures playback position synchronization between peers with network latency compensation.
// Uses structured logging with operation context.
package sync

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/persistence"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/utils"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

type Service struct {
	mu            sync.RWMutex
	rooms         map[string]models.SyncStatus
	isClosed      bool
	closeOnce     sync.Once
	persistence   *persistence.Store
	saveDebouncer *utils.Debouncer
	hostChecker   HostChecker
}

// HostChecker checks if a user is the host of a room.
type HostChecker interface {
	IsHost(ctx context.Context, userID, roomID string) (bool, error)
}

// NewService creates a new synchronization service.
// State is tracked per room so concurrent rooms do not overwrite each other.
// Returns an initialized service.
func NewService(hostChecker HostChecker) *Service {
	svc := &Service{
		rooms:       make(map[string]models.SyncStatus),
		hostChecker: hostChecker,
	}
	svc.saveDebouncer = utils.NewDebouncer(constants.P2PDebounceInterval, svc.flushState)

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
//
// It deliberately does NOT take s.mu. Play, Pause and Seek all call this while
// holding s.mu for their whole body, and re-acquiring that non-reentrant mutex
// here deadlocked them: the goroutine waited on a lock it already owned, and
// every other sync goroutine queued behind it. Against a server with
// persistence enabled — which is how cmd/server runs — play, pause and seek
// then hung forever. GetStatus takes only a read lock and never reaches here,
// so it kept answering while the write path was wedged.
//
// No extra synchronisation is needed. utils.Debouncer has its own mutex,
// Trigger is safe to call concurrently, and it runs flushState out-of-line in
// its own goroutine; flushState takes s.mu.RLock() itself, so it simply waits
// for an in-flight writer to release the lock.
func (s *Service) scheduleSave() {
	if s.persistence == nil {
		return
	}
	s.saveDebouncer.Trigger()
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

// validateHost checks if the user is the host of the room.
// Returns an error if the user is not the host or if the check fails.
func (s *Service) validateHost(ctx context.Context, userID, roomID string) error {
	if s.hostChecker == nil {
		// If no host checker is configured, allow (for backward compatibility in tests)
		return nil
	}
	isHost, err := s.hostChecker.IsHost(ctx, userID, roomID)
	if err != nil {
		logger.Warn("Sync: host check failed", "userID", userID, "roomID", roomID, "error", err)
		return errors.Wrap(errors.ErrUnavailable, "host check failed", err)
	}
	if !isHost {
		logger.Warn("Sync: non-host attempted to control playback", "userID", userID, "roomID", roomID)
		return errors.New(errors.ErrForbidden, "only the room host can control playback")
	}
	return nil
}

// Play starts playback for the specified room.
// Only the room host can call this.
// Sets IsPlaying flag to true and updates the timestamp.
// Returns the current synchronization status or an error if not host.
func (s *Service) Play(ctx context.Context, roomID, userID string) (models.SyncStatus, error) {
	if err := s.validateHost(ctx, userID, roomID); err != nil {
		return models.SyncStatus{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to play on a closed service")
		return s.rooms[roomID], errors.New(errors.ErrUnavailable, "service closed")
	}

	status := s.ensureRoomLocked(roomID)
	status.IsPlaying = true
	status.Timestamp = time.Now().UnixMilli()
	s.rooms[roomID] = status
	s.scheduleSave()

	logger.Info("Sync: playback started", "roomID", roomID, "position", status.Position, "userID", userID)
	return status, nil
}

// Pause pauses playback for the specified room.
// Only the room host can call this.
// Sets IsPlaying flag to false and updates the timestamp.
// Returns the current synchronization status or an error if not host.
func (s *Service) Pause(ctx context.Context, roomID, userID string) (models.SyncStatus, error) {
	if err := s.validateHost(ctx, userID, roomID); err != nil {
		return models.SyncStatus{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to pause on a closed service")
		return s.rooms[roomID], errors.New(errors.ErrUnavailable, "service closed")
	}

	status := s.ensureRoomLocked(roomID)
	status.IsPlaying = false
	status.Timestamp = time.Now().UnixMilli()
	s.rooms[roomID] = status
	s.scheduleSave()

	logger.Info("Sync: playback paused", "roomID", roomID, "position", status.Position, "userID", userID)
	return status, nil
}

// Seek seeks to the specified position in the given room.
// Only the room host can call this.
// Parameter position - position in seconds (0 - 86400).
// Validates position before applying. Large jumps (greater than
// MaxPositionJump) are smoothed toward the requested position using
// SmoothAdjustmentRatio to avoid visual glitches and desync caused by
// network latency or out-of-order events.
// Returns the updated status or an error if position is invalid or not host.
func (s *Service) Seek(ctx context.Context, roomID, userID string, position float64) (models.SyncStatus, error) {
	if err := s.validateHost(ctx, userID, roomID); err != nil {
		return models.SyncStatus{}, err
	}

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
		"smoothed", math.Abs(position-oldPosition) > constants.MaxPositionJump, "userID", userID)
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

// Buffering sets the buffering state for the specified room.
//
// Unlike Play, Pause and Seek, this is NOT a host-only command: buffering is a
// peer reporting its own local condition ("I am out of data"), not a decision
// about what the room should do. Restricting it to the host would strand the
// room: a non-host peer that recovered could not say so, and the pause would
// never lift. Any room member may report buffering (idea-0018).
//
// Room membership is the caller's precondition, not this method's job: the
// HTTP handler resolves the room via P2PService.GetRoomInfo for the calling
// client, which fails with "not connected to a room" (400) before this method
// is reached. The ctx and userID parameters are kept for interface uniformity
// with the host-only methods and for logging.
//
// Updates the buffering flag and timestamp.
// Returns the current synchronization status.
func (s *Service) Buffering(_ context.Context, roomID, userID string, buffering bool) (models.SyncStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to set buffering on a closed service")
		return s.rooms[roomID], errors.New(errors.ErrUnavailable, "service closed")
	}

	status := s.ensureRoomLocked(roomID)
	status.Buffering = buffering
	status.Timestamp = time.Now().UnixMilli()
	s.rooms[roomID] = status
	s.scheduleSave()

	logger.Info("Sync: buffering changed", "roomID", roomID, "buffering", buffering, "userID", userID)
	return status, nil
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
		s.saveDebouncer.Stop()
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
