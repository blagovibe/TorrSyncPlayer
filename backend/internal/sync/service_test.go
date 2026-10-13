package sync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

const testRoomID = "test-room"
const testUserID = "test-user"

func init() {
	// Initialize logger for tests
	logger.Init("error", "json")
}

// TestNewService tests the sync service initialization
func TestNewService(t *testing.T) {
	svc := NewService(nil)
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
	svc := NewService(nil)
	defer svc.Close()

	ctx := context.Background()
	status, err := svc.Play(ctx, testRoomID, testUserID)
	require.NoError(t, err)
	assert.True(t, status.IsPlaying)
	assert.Greater(t, status.Timestamp, int64(0))
}

// TestPause tests pausing playback
func TestPause(t *testing.T) {
	svc := NewService(nil)
	defer svc.Close()

	ctx := context.Background()

	// First start playback
	_, err := svc.Play(ctx, testRoomID, testUserID)
	require.NoError(t, err)

	// Then pause
	status, err := svc.Pause(ctx, testRoomID, testUserID)
	require.NoError(t, err)
	assert.False(t, status.IsPlaying)
}

// TestSeek tests seeking to a specified position. A small jump (within
// MaxPositionJump) is applied directly without smoothing.
func TestSeek(t *testing.T) {
	svc := NewService(nil)
	defer svc.Close()

	ctx := context.Background()

	// Seek from 0 to 1.0 (small jump) -> applied directly.
	status, err := svc.Seek(ctx, testRoomID, testUserID, 1.0)
	require.NoError(t, err)
	assert.Equal(t, 1.0, status.Position)

	// Seek from 1.0 to 2.0 (small jump) -> applied directly.
	status, err = svc.Seek(ctx, testRoomID, testUserID, 2.0)
	require.NoError(t, err)
	assert.Equal(t, 2.0, status.Position)
}

// TestSeek_LatencyCompensation verifies that a large position jump is smoothed
// (only SmoothAdjustmentRatio of the way is applied in a single seek) instead
// of snapping directly to the requested position.
func TestSeek_LatencyCompensation(t *testing.T) {
	svc := NewService(nil)
	defer svc.Close()

	ctx := context.Background()

	status, err := svc.Seek(ctx, testRoomID, testUserID, 100.5)
	require.NoError(t, err)
	// Jump from 0 to 100.5 exceeds MaxPositionJump (2.0s), so it is smoothed.
	want := 0.0 + (100.5-0.0)*constants.SmoothAdjustmentRatio
	assert.InDelta(t, want, status.Position, 1e-9)
	assert.NotEqual(t, 100.5, status.Position)
}

// TestSeek_InvalidPosition tests seeking with invalid position
func TestSeek_InvalidPosition(t *testing.T) {
	svc := NewService(nil)
	defer svc.Close()

	ctx := context.Background()

	// Negative position
	_, err := svc.Seek(ctx, testRoomID, testUserID, -1)
	assert.Error(t, err)

	// Position too large
	_, err = svc.Seek(ctx, testRoomID, testUserID, 100000)
	assert.Error(t, err)
}

// TestClose tests closing the service
func TestClose(t *testing.T) {
	svc := NewService(nil)

	ctx := context.Background()

	// Start playback
	_, err := svc.Play(ctx, testRoomID, testUserID)
	require.NoError(t, err)
	status := svc.GetStatus(testRoomID)
	assert.True(t, status.IsPlaying)

	// Close
	svc.Close()

	// After close, the service reports closed state gracefully.
	status = svc.GetStatus(testRoomID)
	assert.NotNil(t, status)
}

// TestBuffering tests setting buffering state
func TestBuffering(t *testing.T) {
	svc := NewService(nil)
	defer svc.Close()

	ctx := context.Background()

	// Set buffering to true
	status, err := svc.Buffering(ctx, testRoomID, testUserID, true)
	require.NoError(t, err)
	assert.True(t, status.Buffering)
	assert.Greater(t, status.Timestamp, int64(0))

	// Set buffering to false
	status, err = svc.Buffering(ctx, testRoomID, testUserID, false)
	require.NoError(t, err)
	assert.False(t, status.Buffering)
	assert.Greater(t, status.Timestamp, int64(0))
}
