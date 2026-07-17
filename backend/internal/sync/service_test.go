package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

const testRoomID = "test-room"

func init() {
	// Initialize logger for tests
	logger.Init("error", "json")
}

// TestNewService tests the sync service initialization
func TestNewService(t *testing.T) {
	svc := NewService()
	require.NotNil(t, svc)

	defer svc.Close()

	status := svc.GetStatus(testRoomID)
	assert.False(t, status.IsPlaying)
	assert.Equal(t, float64(0), status.Position)
	assert.Equal(t, float64(0), status.Duration)
	assert.Greater(t, status.Timestamp, int64(0))
}

// TestPlay tests starting playback
func TestPlay(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	status := svc.Play(testRoomID)
	assert.True(t, status.IsPlaying)
	assert.Greater(t, status.Timestamp, int64(0))
}

// TestPause tests pausing playback
func TestPause(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// First start playback
	svc.Play(testRoomID)

	// Then pause
	status := svc.Pause(testRoomID)
	assert.False(t, status.IsPlaying)
}

// TestSeek tests seeking to a specified position. A small jump (within
// MaxPositionJump) is applied directly without smoothing.
func TestSeek(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Seek from 0 to 1.0 (small jump) -> applied directly.
	status, err := svc.Seek(testRoomID, 1.0)
	require.NoError(t, err)
	assert.Equal(t, 1.0, status.Position)

	// Seek from 1.0 to 2.0 (small jump) -> applied directly.
	status, err = svc.Seek(testRoomID, 2.0)
	require.NoError(t, err)
	assert.Equal(t, 2.0, status.Position)
}

// TestSeek_LatencyCompensation verifies that a large position jump is smoothed
// (only SmoothAdjustmentRatio of the way is applied in a single seek) instead
// of snapping directly to the requested position.
func TestSeek_LatencyCompensation(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	status, err := svc.Seek(testRoomID, 100.5)
	require.NoError(t, err)
	// Jump from 0 to 100.5 exceeds MaxPositionJump (2.0s), so it is smoothed.
	want := 0.0 + (100.5-0.0)*constants.SmoothAdjustmentRatio
	assert.InDelta(t, want, status.Position, 1e-9)
	assert.NotEqual(t, 100.5, status.Position)
}

// TestSeek_InvalidPosition tests seeking with invalid position
func TestSeek_InvalidPosition(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Negative position
	_, err := svc.Seek(testRoomID, -1)
	assert.Error(t, err)

	// Position too large
	_, err = svc.Seek(testRoomID, 100000)
	assert.Error(t, err)
}

// TestClose tests closing the service
func TestClose(t *testing.T) {
	svc := NewService()

	// Start playback
	svc.Play(testRoomID)
	status := svc.GetStatus(testRoomID)
	assert.True(t, status.IsPlaying)

	// Close
	svc.Close()

	// After close, the service reports closed state gracefully.
	status = svc.GetStatus(testRoomID)
	assert.NotNil(t, status)
}
