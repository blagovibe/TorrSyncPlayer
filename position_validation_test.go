package main

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ==================== Position Validation Tests ====================

func TestValidatePosition_NegativePosition(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Negative position should return error
	err := service.Play(-1.0)
	assert.Error(t, err, "Play with negative position should return error")
	assert.Contains(t, err.Error(), "position cannot be negative")

	err = service.PauseAt(-5.0)
	assert.Error(t, err, "PauseAt with negative position should return error")

	err = service.Seek(-100.0)
	assert.Error(t, err, "Seek with negative position should return error")
}

func TestValidatePosition_ExceedsMaxDuration(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Position exceeding MaxDuration (86400 seconds = 24 hours) should return error
	err := service.Play(86401.0)
	assert.Error(t, err, "Play with position exceeding max duration should return error")
	assert.Contains(t, err.Error(), "position exceeds maximum duration")

	err = service.PauseAt(100000.0)
	assert.Error(t, err, "PauseAt with position exceeding max duration should return error")

	err = service.Seek(99999.0)
	assert.Error(t, err, "Seek with position exceeding max duration should return error")
}

func TestValidatePosition_NaNPosition(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// NaN position should return error
	err := service.Play(math.NaN())
	assert.Error(t, err, "Play with NaN position should return error")
	assert.Contains(t, err.Error(), "position cannot be NaN")
}

func TestValidatePosition_InfinitePosition(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// +Inf position should return error (exceeds max duration or is infinite)
	err := service.Play(math.Inf(1))
	assert.Error(t, err, "Play with +Inf position should return error")
	// +Inf is caught by either "exceeds maximum duration" or "cannot be infinite"
	assert.True(t,
		strings.Contains(err.Error(), "position cannot be infinite") ||
			strings.Contains(err.Error(), "position exceeds maximum duration"),
		"Error should mention infinite or exceeds duration")

	// -Inf position should return error (negative or infinite)
	err = service.Play(math.Inf(-1))
	assert.Error(t, err, "Play with -Inf position should return error")
	assert.True(t,
		strings.Contains(err.Error(), "position cannot be infinite") ||
			strings.Contains(err.Error(), "position cannot be negative"),
		"Error should mention infinite or negative")
}

func TestValidatePosition_ValidPositions(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Zero position should be valid
	err := service.Play(0.0)
	assert.NoError(t, err, "Play with zero position should succeed")

	// Normal position should be valid
	err = service.Play(100.0)
	assert.NoError(t, err, "Play with normal position should succeed")

	// Max duration position should be valid
	err = service.Play(86400.0)
	assert.NoError(t, err, "Play with max duration position should succeed")

	// PauseAt with valid positions
	err = service.PauseAt(0.0)
	assert.NoError(t, err, "PauseAt with zero position should succeed")

	err = service.PauseAt(50000.0)
	assert.NoError(t, err, "PauseAt with normal position should succeed")

	// Seek with valid positions
	err = service.Seek(0.0)
	assert.NoError(t, err, "Seek with zero position should succeed")

	err = service.Seek(3600.0)
	assert.NoError(t, err, "Seek with normal position should succeed")
}

func TestValidatePosition_StateUnchangedOnError(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Set initial valid state
	err := service.Play(100.0)
	assert.NoError(t, err)
	state := service.GetState()
	assert.Equal(t, 100.0, state.Position)

	// Try invalid position - state should not change
	service.Play(-50.0)
	state = service.GetState()
	assert.Equal(t, 100.0, state.Position, "Position should not change on invalid input")

	service.Seek(999999.0)
	state = service.GetState()
	assert.Equal(t, 100.0, state.Position, "Position should not change on invalid seek")
}

func TestMaxDurationConstant(t *testing.T) {
	// Verify MaxDuration is set to 86400 (24 hours in seconds)
	assert.Equal(t, 86400, MaxDuration, "MaxDuration should be 86400 seconds (24 hours)")
}
