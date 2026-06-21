package sync

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

func init() {
	// Initialize logger for tests
	logger.Init("error", "json")
}

// TestNewService tests the sync service initialization
func TestNewService(t *testing.T) {
	svc := NewService()
	require.NotNil(t, svc)

	defer svc.Close()

	status := svc.GetStatus(context.Background())
	assert.False(t, status.IsPlaying)
	assert.Equal(t, float64(0), status.Position)
	assert.Equal(t, float64(0), status.Duration)
	assert.Greater(t, status.Timestamp, int64(0))
}

// TestPlay tests starting playback
func TestPlay(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	status := svc.Play(context.Background())
	assert.True(t, status.IsPlaying)
	assert.Greater(t, status.Timestamp, int64(0))
}

// TestPause tests pausing playback
func TestPause(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// First start playback
	svc.Play(context.Background())

	// Then pause
	status := svc.Pause(context.Background())
	assert.False(t, status.IsPlaying)
}

// TestSeek tests seeking to a specified position
func TestSeek(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	status, err := svc.Seek(context.Background(), 100.5)
	require.NoError(t, err)
	assert.Equal(t, 100.5, status.Position)
}

// TestSeek_InvalidPosition tests seeking with invalid position
func TestSeek_InvalidPosition(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Negative position
	_, err := svc.Seek(context.Background(), -1)
	assert.Error(t, err)

	// Position too large
	_, err = svc.Seek(context.Background(), 100000)
	assert.Error(t, err)
}

// TestSetDuration tests setting duration
func TestSetDuration(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	err := svc.SetDuration(context.Background(), 3600.0)
	require.NoError(t, err)

	status := svc.GetStatus(context.Background())
	assert.Equal(t, 3600.0, status.Duration)
}

// TestSetDuration_Invalid tests setting invalid duration
func TestSetDuration_Invalid(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	err := svc.SetDuration(context.Background(), -1)
	assert.Error(t, err)
}

// TestUpdatePosition tests updating position
func TestUpdatePosition(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	err := svc.UpdatePosition(context.Background(), 50.0)
	require.NoError(t, err)

	status := svc.GetStatus(context.Background())
	assert.Equal(t, 50.0, status.Position)
}

// TestSyncWithLatency tests synchronization with network latency
func TestSyncWithLatency(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	peerStatus := models.SyncStatus{
		IsPlaying: true,
		Position:  100.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}

	status := svc.SyncWithLatency(context.Background(), peerStatus, 50) // 50ms latency
	assert.True(t, status.IsPlaying)
	assert.Greater(t, status.Position, float64(0))
}

// TestSyncWithLatency_Pause tests synchronization with pause state
func TestSyncWithLatency_Pause(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Start playback
	svc.Play(context.Background())

	peerStatus := models.SyncStatus{
		IsPlaying: false,
		Position:  50.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}

	status := svc.SyncWithLatency(context.Background(), peerStatus, 0)
	assert.False(t, status.IsPlaying)
}

// TestClose tests closing the service
func TestClose(t *testing.T) {
	svc := NewService()

	// Start
	svc.Play(context.Background())
	status := svc.GetStatus(context.Background())
	assert.True(t, status.IsPlaying)

	// Close
	svc.Close()

	// After close, status should not change
	status = svc.Play(context.Background())
	assert.False(t, status.IsPlaying)
}

// TestClose_MultipleCalls tests that multiple Close calls are safe
func TestClose_MultipleCalls(t *testing.T) {
	svc := NewService()

	// Multiple Close calls should not cause panic
	svc.Close()
	svc.Close()
	svc.Close()
}

// TestValidatePosition tests position validation
func TestValidatePosition(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	tests := []struct {
		name    string
		pos     float64
		wantErr bool
	}{
		{"valid zero", 0, false},
		{"valid positive", 100.5, false},
		{"valid max", 86400, false},
		{"negative", -1, true},
		{"too large", 86401, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Seek(context.Background(), tt.pos)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============ New integration tests ============

// TestSyncPlayback_FullCycle tests a full playback synchronization cycle
func TestSyncPlayback_FullCycle(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// 1. Initial state — paused
	status := svc.GetStatus(context.Background())
	assert.False(t, status.IsPlaying)
	assert.Equal(t, float64(0), status.Position)

	// 2. Set duration
	err := svc.SetDuration(context.Background(), 7200.0) // 2 hours
	require.NoError(t, err)

	// 3. Start playback
	status = svc.Play(context.Background())
	assert.True(t, status.IsPlaying)

	// 4. Update position
	err = svc.UpdatePosition(context.Background(), 120.0)
	require.NoError(t, err)

	status = svc.GetStatus(context.Background())
	assert.Equal(t, 120.0, status.Position)
	assert.True(t, status.IsPlaying)

	// 5. Seek
	status, err = svc.Seek(context.Background(), 300.0)
	require.NoError(t, err)
	assert.Equal(t, 300.0, status.Position)

	// 6. Pause
	status = svc.Pause(context.Background())
	assert.False(t, status.IsPlaying)
	assert.Equal(t, 300.0, status.Position)

	// 7. Verify duration is preserved
	status = svc.GetStatus(context.Background())
	assert.Equal(t, 7200.0, status.Duration)
}

// TestSyncPlayback_SeekBoundaries tests seeking at boundary values
func TestSyncPlayback_SeekBoundaries(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Set duration
	err := svc.SetDuration(context.Background(), 3600.0)
	require.NoError(t, err)

	// Seek to start
	status, err := svc.Seek(context.Background(), 0)
	require.NoError(t, err)
	assert.Equal(t, float64(0), status.Position)

	// Seek to end
	status, err = svc.Seek(context.Background(), 3600.0)
	require.NoError(t, err)
	assert.Equal(t, 3600.0, status.Position)
}

// TestGetPlaybackState_Consistency tests state consistency
func TestGetPlaybackState_Consistency(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Get state multiple times — it should be consistent
	status1 := svc.GetStatus(context.Background())
	status2 := svc.GetStatus(context.Background())
	assert.Equal(t, status1.IsPlaying, status2.IsPlaying)
	assert.Equal(t, status1.Position, status2.Position)
	assert.Equal(t, status1.Duration, status2.Duration)

	// After changes, state should reflect the changes
	svc.Play(context.Background())
	_ = svc.UpdatePosition(context.Background(), 42.0)

	status3 := svc.GetStatus(context.Background())
	assert.True(t, status3.IsPlaying)
	assert.Equal(t, 42.0, status3.Position)
}

// TestSyncWithLatency_LargeLatency tests synchronization with large latency
func TestSyncWithLatency_LargeLatency(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	peerStatus := models.SyncStatus{
		IsPlaying: true,
		Position:  500.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}

	// Large delay 500ms
	status := svc.SyncWithLatency(context.Background(), peerStatus, 500)
	assert.True(t, status.IsPlaying)
	// Position should be adjusted for latency
	assert.Greater(t, status.Position, float64(0))
}

// TestSyncWithLatency_ZeroLatency tests synchronization with zero latency
func TestSyncWithLatency_ZeroLatency(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Set position close to peer (difference < 2s — full adjustment)
	err := svc.UpdatePosition(context.Background(), 199.0)
	require.NoError(t, err)

	now := time.Now().UnixMilli()
	peerStatus := models.SyncStatus{
		IsPlaying: false,
		Position:  200.0,
		Duration:  3600.0,
		Timestamp: now,
	}

	status := svc.SyncWithLatency(context.Background(), peerStatus, 0)
	assert.False(t, status.IsPlaying)
	// With small discrepancy (<2s) full adjustment should occur
	assert.InDelta(t, 200.0, status.Position, 0.1)
}

// TestSyncWithLatency_SmallDifference tests smooth adjustment with small discrepancy
func TestSyncWithLatency_SmallDifference(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Set current position
	err := svc.UpdatePosition(context.Background(), 100.0)
	require.NoError(t, err)

	// Peer with small discrepancy (less than 2 seconds)
	peerStatus := models.SyncStatus{
		IsPlaying: true,
		Position:  101.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}

	status := svc.SyncWithLatency(context.Background(), peerStatus, 0)
	// With small discrepancy full adjustment should occur
	assert.InDelta(t, 101.0, status.Position, 0.1)
}

// TestSyncWithLatency_LargeDifference tests smooth adjustment with large discrepancy
func TestSyncWithLatency_LargeDifference(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Set current position
	err := svc.UpdatePosition(context.Background(), 100.0)
	require.NoError(t, err)

	// Peer with large discrepancy (more than 2 seconds — maximum jump)
	peerStatus := models.SyncStatus{
		IsPlaying: true,
		Position:  200.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}

	status := svc.SyncWithLatency(context.Background(), peerStatus, 0)
	// With large discrepancy smooth adjustment should occur (30% of difference)
	// Difference = 100, 30% = 30, new position = 100 + 30 = 130
	assert.InDelta(t, 130.0, status.Position, 0.1)
}

// TestUpdatePosition_InvalidValues tests updating position with invalid values
func TestUpdatePosition_InvalidValues(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Negative position
	err := svc.UpdatePosition(context.Background(), -10.0)
	assert.Error(t, err)

	// Position too large
	err = svc.UpdatePosition(context.Background(), 90000.0)
	assert.Error(t, err)
}

// TestSetDuration_EdgeCases tests setting duration in edge cases
func TestSetDuration_EdgeCases(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	// Zero duration — allowed
	err := svc.SetDuration(context.Background(), 0)
	assert.NoError(t, err)

	// Negative — not allowed
	err = svc.SetDuration(context.Background(), -1)
	assert.Error(t, err)
}

// TestPlayPauseSequence tests the Play/Pause sequence
func TestPlayPauseSequence(t *testing.T) {
	svc := NewService()
	defer svc.Close()

	for i := 0; i < 5; i++ {
		status := svc.Play(context.Background())
		assert.True(t, status.IsPlaying)

		status = svc.Pause(context.Background())
		assert.False(t, status.IsPlaying)
	}
}

// TestClosedService_Operations tests operation behavior after close
func TestClosedService_Operations(t *testing.T) {
	svc := NewService()
	svc.Close()

	// Play after close
	status := svc.Play(context.Background())
	assert.False(t, status.IsPlaying)

	// Pause after close
	status = svc.Pause(context.Background())
	assert.False(t, status.IsPlaying)

	// Seek after close
	_, err := svc.Seek(context.Background(), 100.0)
	assert.Error(t, err)

	// SetDuration after close
	err = svc.SetDuration(context.Background(), 3600.0)
	assert.Error(t, err)

	// UpdatePosition after close
	err = svc.UpdatePosition(context.Background(), 50.0)
	assert.Error(t, err)

	// SyncWithLatency after close
	peerStatus := models.SyncStatus{
		IsPlaying: true,
		Position:  100.0,
		Duration:  3600.0,
		Timestamp: time.Now().UnixMilli(),
	}
	status = svc.SyncWithLatency(context.Background(), peerStatus, 0)
	assert.False(t, status.IsPlaying)
}
