// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package sync provides the playback synchronization service.
// Ensures playback position synchronization between peers with network latency compensation.
// Uses structured logging with operation context.
package sync

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

type Service struct {
	mu        sync.RWMutex
	status    models.SyncStatus
	isClosed  bool
	closeOnce sync.Once
}

// NewService creates a new synchronization service.
// Initializes the initial state: playback stopped, position 0.
// Returns an initialized service.
func NewService() *Service {
	svc := &Service{
		status: models.SyncStatus{
			IsPlaying: false,
			Position:  0,
			Duration:  0,
			Timestamp: time.Now().UnixMilli(),
		},
	}

	logger.Info("Sync: service initialized")
	return svc
}

// Play starts playback.
// Sets IsPlaying flag to true and updates the timestamp.
// Returns the current synchronization status.
func (s *Service) Play(ctx context.Context) models.SyncStatus {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to play on a closed service")
		return s.status
	}

	s.status.IsPlaying = true
	s.status.Timestamp = time.Now().UnixMilli()

	logger.Info("Sync: playback started", "position", s.status.Position)
	return s.status
}

// Pause pauses playback.
// Sets IsPlaying flag to false and updates the timestamp.
// Returns the current synchronization status.
func (s *Service) Pause(ctx context.Context) models.SyncStatus {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to pause on a closed service")
		return s.status
	}

	s.status.IsPlaying = false
	s.status.Timestamp = time.Now().UnixMilli()

	logger.Info("Sync: playback paused", "position", s.status.Position)
	return s.status
}

// Seek seeks to the specified position.
// Parameter position - position in seconds (0 - 86400).
// Validates position before applying.
// Returns the updated status or an error if position is invalid.
func (s *Service) Seek(ctx context.Context, position float64) (models.SyncStatus, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to seek on a closed service")
		return s.status, errors.New(errors.ErrUnavailable, "service closed")
	}

	// Validate position
	if err := validation.ValidatePosition(position); err != nil {
		logger.Warn("Sync: invalid seek position", "position", position, "error", err)
		return s.status, errors.Wrap(errors.ErrInvalidInput, "invalid position", err)
	}

	oldPosition := s.status.Position
	s.status.Position = position
	s.status.Timestamp = time.Now().UnixMilli()

	logger.Info("Sync: seek", "oldPosition", oldPosition, "newPosition", position)
	return s.status, nil
}

// GetStatus returns the current playback status.
// Includes position, duration, playback state and timestamp.
func (s *Service) GetStatus(ctx context.Context) models.SyncStatus {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.status
}

// SetDuration sets the media file duration.
// Parameter duration - duration in seconds (must be positive).
// Returns an error if the value is invalid.
func (s *Service) SetDuration(ctx context.Context, duration float64) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to set duration on a closed service")
		return errors.New(errors.ErrUnavailable, "service closed")
	}

	if duration < 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		logger.Warn("Sync: invalid duration", "duration", duration)
		return errors.InvalidInput(fmt.Sprintf("invalid duration: %f", duration))
	}

	s.status.Duration = duration
	logger.Info("Sync: duration set", "duration", duration)
	return nil
}

// SyncWithLatency synchronizes playback with network latency compensation.
// Parameter peerStatus - remote peer status.
// Parameter latencyMs - network latency in milliseconds.
// Uses smooth position adjustment to avoid abrupt jumps.
// Returns the updated local status.
func (s *Service) SyncWithLatency(ctx context.Context, peerStatus models.SyncStatus, latencyMs int) models.SyncStatus {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to sync on a closed service")
		return s.status
	}

	// Validate latency
	if latencyMs < 0 {
		latencyMs = 0
	}

	// Latency compensation
	latencySeconds := float64(latencyMs) / constants.MsPerSecond

	// Calculate expected peer position accounting for latency
	expectedPosition := peerStatus.Position
	if peerStatus.IsPlaying {
		diff := time.Now().UnixMilli() - peerStatus.Timestamp
		if diff > constants.MaxSyncTimestampDiff {
			diff = 0
		}
		elapsed := float64(diff) / constants.MsPerSecond
		expectedPosition = peerStatus.Position + elapsed - latencySeconds
	}

	// Validate calculated position
	if err := validation.ValidatePosition(expectedPosition); err != nil {
		logger.Warn("Sync: received invalid position from peer",
			"peerPosition", peerStatus.Position,
			"expectedPosition", expectedPosition,
			"error", err,
		)
		return s.status
	}

	// Smooth position adjustment (not an abrupt jump)
	positionDiff := expectedPosition - s.status.Position

	if math.Abs(positionDiff) > constants.MaxPositionJump {
		// Smooth adjustment
		s.status.Position += positionDiff * constants.SmoothAdjustmentRatio
	} else {
		// Small discrepancy - adjust fully
		s.status.Position = expectedPosition
	}

	s.status.Timestamp = time.Now().UnixMilli()

	// Synchronize playback state
	if s.status.IsPlaying != peerStatus.IsPlaying {
		s.status.IsPlaying = peerStatus.IsPlaying
	}

	logger.Debug("Sync: synchronization completed",
		"localPosition", s.status.Position,
		"peerPosition", peerStatus.Position,
		"expectedPosition", expectedPosition,
		"latencyMs", latencyMs,
		"positionDiff", positionDiff,
	)

	return s.status
}

// UpdatePosition updates the current playback position.
// Called by the local player when the position changes.
// Parameter position - new position in seconds.
// Returns an error if position is invalid or service is closed.
func (s *Service) UpdatePosition(ctx context.Context, position float64) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isClosed {
		logger.Warn("Sync: attempt to update position on a closed service")
		return fmt.Errorf("service closed")
	}

	if err := validation.ValidatePosition(position); err != nil {
		logger.Warn("Sync: invalid position for update", "position", position, "error", err)
		return err
	}

	s.status.Position = position
	s.status.Timestamp = time.Now().UnixMilli()

	return nil
}

// Close closes the synchronization service.
// Stops playback and marks the service as closed.
// Safe for multiple calls (uses sync.Once).
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.isClosed = true
		s.status.IsPlaying = false

		logger.Info("Sync: service stopped")
	})
}
