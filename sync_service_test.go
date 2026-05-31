package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSyncService(t *testing.T) {
	service := NewSyncService()
	require.NotNil(t, service, "SyncService should not be nil")
	assert.NotNil(t, service.commandChan, "commandChan should be initialized")
	assert.NotNil(t, service.stateUpdateChan, "stateUpdateChan should be initialized")
	assert.NotNil(t, service.closeChan, "closeChan should be initialized")
	assert.NotNil(t, service.rttMeasurements, "rttMeasurements should be initialized")
	assert.Equal(t, 1500.0, service.syncTolerance, "default sync tolerance should be 1500ms")
	assert.Equal(t, 1500.0, service.syncStats.SyncTolerance, "syncStats tolerance should be 1500ms")
	assert.Equal(t, 2*1000000000, int(service.heartbeatInterval), "heartbeat interval should be 2 seconds")
}

func TestSyncServicePlayCommand(t *testing.T) {
	service := NewSyncService()

	// Set as host to test command broadcasting
	service.SetAsHost()

	// Test play command
	service.Play(100.0)

	// Verify state was updated
	state := service.GetState()
	assert.True(t, state.IsPlaying, "isPlaying should be true after Play")
	assert.Equal(t, 100.0, state.Position, "position should be 100.0")
	assert.Equal(t, 1.0, state.PlaybackRate, "playbackRate should default to 1.0")
	assert.Greater(t, state.Timestamp, int64(0), "timestamp should be set")
}

func TestSyncServicePauseCommand(t *testing.T) {
	service := NewSyncService()

	// Set as host
	service.SetAsHost()

	// First play, then pause at position
	service.Play(50.0)
	service.PauseAt(75.0)

	// Verify state was updated
	state := service.GetState()
	assert.False(t, state.IsPlaying, "isPlaying should be false after Pause")
	assert.Equal(t, 75.0, state.Position, "position should be 75.0")
	assert.Greater(t, state.Timestamp, int64(0), "timestamp should be set")
}

func TestSyncServiceSeekCommand(t *testing.T) {
	service := NewSyncService()

	// Set as host
	service.SetAsHost()

	// Test seek command
	service.Seek(200.0)

	// Verify state was updated
	state := service.GetState()
	assert.Equal(t, 200.0, state.Position, "position should be 200.0")
	assert.Greater(t, state.Timestamp, int64(0), "timestamp should be set")
}

func TestSyncServiceHandlePlayCommand(t *testing.T) {
	service := NewSyncService()

	// Set as guest (host commands should be ignored)
	service.SetAsGuest()

	cmd := SyncCommand{
		Type:      "play",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"position":     100.0,
			"playbackRate": 1.5,
		},
	}

	service.HandleSyncCommand(cmd)

	// Verify state was updated
	state := service.GetState()
	assert.True(t, state.IsPlaying, "isPlaying should be true")
	assert.Equal(t, 1.5, state.PlaybackRate, "playbackRate should be 1.5")
}

func TestSyncServiceHandlePauseCommand(t *testing.T) {
	service := NewSyncService()

	// Set as guest
	service.SetAsGuest()

	// First play
	service.Play(100.0)

	cmd := SyncCommand{
		Type:      "pause",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"position": 150.0,
		},
	}

	service.HandleSyncCommand(cmd)

	// Verify state was updated
	state := service.GetState()
	assert.False(t, state.IsPlaying, "isPlaying should be false")
}

func TestSyncServiceHandleSeekCommand(t *testing.T) {
	service := NewSyncService()

	// Set as guest
	service.SetAsGuest()

	cmd := SyncCommand{
		Type:      "seek",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"position": 300.0,
		},
	}

	service.HandleSyncCommand(cmd)

	// Verify state was updated
	state := service.GetState()
	assert.Equal(t, 300.0, state.Position, "position should be 300.0")
}

func TestSyncServiceHandleInvalidCommandData(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Test with invalid data type (not map[string]interface{})
	cmd := SyncCommand{
		Type:      "play",
		Timestamp: time.Now().UnixMilli(),
		Data:      "invalid data",
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandleUnknownCommandType(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	cmd := SyncCommand{
		Type:      "unknown_command",
		Timestamp: time.Now().UnixMilli(),
		Data:      nil,
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandleNilCommandData(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	cmd := SyncCommand{
		Type:      "play",
		Timestamp: time.Now().UnixMilli(),
		Data:      nil,
	}

	// Should not panic with nil data
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandleMissingPositionInData(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Data without position key
	cmd := SyncCommand{
		Type:      "play",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"other_key": "value",
		},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandleWrongPositionType(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Data with wrong type for position
	cmd := SyncCommand{
		Type:      "play",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"position": "not a number",
		},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHostIgnoresOwnCommands(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Host should ignore play commands
	cmd := SyncCommand{
		Type:      "play",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"position": 100.0,
		},
	}

	// Get initial state
	initialState := service.GetState()

	service.HandleSyncCommand(cmd)

	// State should not change (host ignores own commands)
	state := service.GetState()
	assert.Equal(t, initialState.IsPlaying, state.IsPlaying)
}

func TestSyncServiceGetState(t *testing.T) {
	service := NewSyncService()

	state := service.GetState()
	assert.False(t, state.IsPlaying, "initial isPlaying should be false")
	assert.Equal(t, 0.0, state.Position, "initial position should be 0")
	assert.Equal(t, 0.0, state.Duration, "initial duration should be 0")
	assert.Equal(t, 0.0, state.PlaybackRate, "initial playbackRate should be 0")
}

func TestSyncServiceGetStats(t *testing.T) {
	service := NewSyncService()

	stats := service.GetStats()
	assert.Equal(t, 0.0, stats.RTT, "initial RTT should be 0")
	assert.Equal(t, 0.0, stats.Drift, "initial drift should be 0")
	assert.Equal(t, 1500.0, stats.SyncTolerance, "sync tolerance should be 1500")
	assert.Equal(t, 0, stats.CorrectionCount, "initial correction count should be 0")
}

func TestSyncServiceSetSyncTolerance(t *testing.T) {
	service := NewSyncService()

	service.SetSyncTolerance(2000.0)

	stats := service.GetStats()
	assert.Equal(t, 2000.0, stats.SyncTolerance, "sync tolerance should be updated to 2000")
}

func TestSyncServiceIsHost(t *testing.T) {
	service := NewSyncService()

	// Initially should not be host
	assert.False(t, service.IsHost(), "should not be host initially")

	// Set as host
	service.SetAsHost()
	assert.True(t, service.IsHost(), "should be host after SetAsHost")

	// Set as guest
	service.SetAsGuest()
	assert.False(t, service.IsHost(), "should not be host after SetAsGuest")
}

func TestSyncServiceUpdateState(t *testing.T) {
	service := NewSyncService()

	newState := PlaybackState{
		IsPlaying:    true,
		Position:     100.0,
		Duration:     3600.0,
		PlaybackRate: 1.5,
		Timestamp:    time.Now().UnixMilli(),
	}

	service.UpdateState(newState)

	state := service.GetState()
	assert.Equal(t, newState.IsPlaying, state.IsPlaying)
	assert.Equal(t, newState.Position, state.Position)
	assert.Equal(t, newState.Duration, state.Duration)
	assert.Equal(t, newState.PlaybackRate, state.PlaybackRate)
}

func TestSyncServiceHandleHeartbeatAsGuest(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Heartbeat with data from host
	cmd := SyncCommand{
		Type:      "heartbeat",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"position":     100.0,
			"isPlaying":    true,
			"playbackRate": 1.0,
		},
	}

	service.HandleSyncCommand(cmd)

	state := service.GetState()
	assert.True(t, state.IsPlaying, "isPlaying should be true")
	assert.Equal(t, 1.0, state.PlaybackRate, "playbackRate should be 1.0")
}

func TestSyncServiceHandleHeartbeatAsHost(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Host receives heartbeat from guest (no data)
	cmd := SyncCommand{
		Type:      "heartbeat",
		Timestamp: time.Now().UnixMilli(),
		Data:      nil,
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandleStateCommand(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	stateData := PlaybackState{
		IsPlaying:    true,
		Position:     200.0,
		Duration:     3600.0,
		PlaybackRate: 1.0,
		Timestamp:    time.Now().UnixMilli(),
	}

	cmd := SyncCommand{
		Type:      "state",
		Timestamp: time.Now().UnixMilli(),
		Data:      stateData,
	}

	service.HandleSyncCommand(cmd)

	state := service.GetState()
	assert.True(t, state.IsPlaying, "isPlaying should be true")
	assert.Equal(t, 1.0, state.PlaybackRate, "playbackRate should be 1.0")
}

func TestSyncServiceHandleStateCommandInvalidData(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Invalid state data (not a PlaybackState)
	cmd := SyncCommand{
		Type:      "state",
		Timestamp: time.Now().UnixMilli(),
		Data:      "invalid",
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceSetPlaybackRate(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	service.SetPlaybackRate(2.0)

	state := service.GetState()
	assert.Equal(t, 2.0, state.PlaybackRate, "playbackRate should be 2.0")
}

func TestSyncServiceGetDrift(t *testing.T) {
	service := NewSyncService()

	drift := service.GetDrift()
	assert.Equal(t, 0.0, drift, "initial drift should be 0")
}

func TestSyncServiceGetRTT(t *testing.T) {
	service := NewSyncService()

	rtt := service.GetRTT()
	assert.Equal(t, 0.0, rtt, "initial RTT should be 0")
}

func TestSyncServiceSyncNowAsGuest(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Should not panic
	assert.NotPanics(t, func() {
		service.SyncNow()
	})
}

func TestSyncServiceSyncNowAsHost(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Host should ignore SyncNow
	assert.NotPanics(t, func() {
		service.SyncNow()
	})
}

func TestSyncServiceStartStopHeartbeat(t *testing.T) {
	service := NewSyncService()
	ctx := context.Background()

	// Initialize with context
	err := service.Init(ctx)
	require.NoError(t, err)

	// Start heartbeat
	assert.NotPanics(t, func() {
		service.StartHeartbeat()
	})

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Close service (stops heartbeat via closeChan)
	assert.NotPanics(t, func() {
		service.Close()
	})
}

func TestSyncServiceClose(t *testing.T) {
	service := NewSyncService()

	// Should not panic
	assert.NotPanics(t, func() {
		service.Close()
	})
}

func TestSyncServiceMultiplePlayPause(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Multiple play/pause cycles
	for i := 0; i < 10; i++ {
		service.Play(float64(i * 10))
		state := service.GetState()
		assert.True(t, state.IsPlaying)

		service.PauseAt(float64(i*10 + 5))
		state = service.GetState()
		assert.False(t, state.IsPlaying)
	}
}

func TestSyncServiceRapidSeek(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Rapid seek operations
	for i := 0; i < 100; i++ {
		service.Seek(float64(i))
	}

	state := service.GetState()
	assert.Equal(t, 99.0, state.Position, "position should be 99")
}

func TestSyncServiceConcurrentAccess(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Test concurrent reads and writes
	done := make(chan bool)

	// Writer
	go func() {
		for i := 0; i < 100; i++ {
			service.Play(float64(i))
		}
		done <- true
	}()

	// Reader
	go func() {
		for i := 0; i < 100; i++ {
			service.GetState()
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done
}

func TestSyncServiceHandleCommandWithEmptyData(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Play command with empty data map
	cmd := SyncCommand{
		Type:      "play",
		Timestamp: time.Now().UnixMilli(),
		Data:      map[string]interface{}{},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandlePauseWithEmptyData(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	cmd := SyncCommand{
		Type:      "pause",
		Timestamp: time.Now().UnixMilli(),
		Data:      map[string]interface{}{},
	}

	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandleSeekWithEmptyData(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	cmd := SyncCommand{
		Type:      "seek",
		Timestamp: time.Now().UnixMilli(),
		Data:      map[string]interface{}{},
	}

	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandleHeartbeatWithEmptyData(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	cmd := SyncCommand{
		Type:      "heartbeat",
		Timestamp: time.Now().UnixMilli(),
		Data:      map[string]interface{}{},
	}

	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceDefaultPlaybackRate(t *testing.T) {
	service := NewSyncService()
	service.SetAsHost()

	// Play without setting playback rate
	service.Play(0.0)

	state := service.GetState()
	assert.Equal(t, 1.0, state.PlaybackRate, "default playback rate should be 1.0")
}

func TestSyncServiceHandlePlayCommandCompensation(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Old timestamp to test delay compensation
	oldTimestamp := time.Now().UnixMilli() - 1000 // 1 second ago

	cmd := SyncCommand{
		Type:      "play",
		Timestamp: oldTimestamp,
		Data: map[string]interface{}{
			"position":     100.0,
			"playbackRate": 1.0,
		},
	}

	service.HandleSyncCommand(cmd)

	state := service.GetState()
	// Position should be compensated for the 1 second delay
	assert.Greater(t, state.Position, 100.0, "position should be compensated for delay")
}

func TestSyncServiceHandlePauseCommandCompensation(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Old timestamp to test delay compensation
	oldTimestamp := time.Now().UnixMilli() - 500 // 0.5 seconds ago

	cmd := SyncCommand{
		Type:      "pause",
		Timestamp: oldTimestamp,
		Data: map[string]interface{}{
			"position": 100.0,
		},
	}

	service.HandleSyncCommand(cmd)

	state := service.GetState()
	// Position should be compensated for the 0.5 second delay
	assert.Greater(t, state.Position, 100.0, "position should be compensated for delay")
}

func TestSyncServiceHandleStateCommandCompensation(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Old timestamp to test delay compensation
	oldTimestamp := time.Now().UnixMilli() - 2000 // 2 seconds ago

	stateData := PlaybackState{
		IsPlaying:    true,
		Position:     100.0,
		PlaybackRate: 1.0,
	}

	cmd := SyncCommand{
		Type:      "state",
		Timestamp: oldTimestamp,
		Data:      stateData,
	}

	service.HandleSyncCommand(cmd)

	state := service.GetState()
	// Position should be compensated for the 2 second delay
	assert.Greater(t, state.Position, 100.0, "position should be compensated for delay")
}

func TestSyncServiceHandleHeartbeatCompensation(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Old timestamp to test delay compensation
	oldTimestamp := time.Now().UnixMilli() - 1000 // 1 second ago

	cmd := SyncCommand{
		Type:      "heartbeat",
		Timestamp: oldTimestamp,
		Data: map[string]interface{}{
			"position":     100.0,
			"isPlaying":    true,
			"playbackRate": 1.0,
		},
	}

	service.HandleSyncCommand(cmd)

	state := service.GetState()
	// Position should be compensated for the 1 second delay
	assert.Greater(t, state.Position, 100.0, "position should be compensated for delay")
}

func TestSyncServiceHandleHeartbeatNotPlayingCompensation(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Old timestamp
	oldTimestamp := time.Now().UnixMilli() - 1000

	cmd := SyncCommand{
		Type:      "heartbeat",
		Timestamp: oldTimestamp,
		Data: map[string]interface{}{
			"position":     100.0,
			"isPlaying":    false,
			"playbackRate": 1.0,
		},
	}

	service.HandleSyncCommand(cmd)

	state := service.GetState()
	// Position should NOT be compensated when not playing
	assert.Equal(t, 100.0, state.Position, "position should not be compensated when not playing")
}

func TestSyncServiceHandleHeartbeatMissingFields(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Heartbeat with missing fields
	cmd := SyncCommand{
		Type:      "heartbeat",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"position": 100.0,
			// Missing isPlaying and playbackRate
		},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandleHeartbeatWrongTypes(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Heartbeat with wrong types
	cmd := SyncCommand{
		Type:      "heartbeat",
		Timestamp: time.Now().UnixMilli(),
		Data: map[string]interface{}{
			"position":     "not a number",
			"isPlaying":    "not a bool",
			"playbackRate": "not a number",
		},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandleStateCommandNotPlaying(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	stateData := PlaybackState{
		IsPlaying:    false,
		Position:     100.0,
		PlaybackRate: 1.0,
	}

	cmd := SyncCommand{
		Type:      "state",
		Timestamp: time.Now().UnixMilli() - 2000,
		Data:      stateData,
	}

	service.HandleSyncCommand(cmd)

	state := service.GetState()
	// Position should NOT be compensated when not playing
	assert.Equal(t, 100.0, state.Position, "position should not be compensated when not playing")
}

func TestSyncServiceHandleStateCommandWithMarshalError(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Data that cannot be marshaled (contains channel)
	badData := make(chan int)

	cmd := SyncCommand{
		Type:      "state",
		Timestamp: time.Now().UnixMilli(),
		Data:      badData,
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

func TestSyncServiceHandleStateCommandWithUnmarshalError(t *testing.T) {
	service := NewSyncService()
	service.SetAsGuest()

	// Invalid JSON structure for PlaybackState
	cmd := SyncCommand{
		Type:      "state",
		Timestamp: time.Now().UnixMilli(),
		Data:      map[string]interface{}{"isPlaying": "not a bool"},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		service.HandleSyncCommand(cmd)
	})
}

// ==================== Position Validation Tests ====================
