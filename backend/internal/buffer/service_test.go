package buffer

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

func init() {
	logger.Init("error", "json")
}

func TestNewService(t *testing.T) {
	s := NewService(1024 * 1024)
	assert.NotNil(t, s)
	assert.Equal(t, int64(1024*1024), s.maxCacheSize)
}

func TestService_GetBufferInfo_NotFound(t *testing.T) {
	s := NewService(1024)
	_, err := s.GetBufferInfo(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestService_Close(t *testing.T) {
	s := NewService(1024)
	s.Close()
	assert.Equal(t, 0, len(s.torrentBuffers))
}

func TestService_StartStopPeriodicUpdate(t *testing.T) {
	s := NewService(1024)
	s.StartPeriodicUpdate(100 * time.Millisecond)
	time.Sleep(150 * time.Millisecond)
	s.StopPeriodicUpdate()
	assert.NotNil(t, s)
}

func TestService_StopPeriodicUpdate_NotStarted(t *testing.T) {
	s := NewService(1024)
	s.StopPeriodicUpdate()
	assert.NotNil(t, s)
}

func TestTorrentBuffer(t *testing.T) {
	tb := &TorrentBuffer{
		TorrentID:       "test-id",
		CurrentPosition: 1000,
		BufferPercent:   10,
		BufferDuration:  60,
		MaxBufferSize:   512 * 1024 * 1024,
		PieceSize:       256 * 1024,
		TotalPieces:     100,
	}
	assert.Equal(t, "test-id", tb.TorrentID)
	assert.Equal(t, int64(1000), tb.CurrentPosition)
	assert.Equal(t, 10, tb.BufferPercent)
}

func TestService_UpdatePosition_Nonexistent(t *testing.T) {
	s := NewService(1024)
	s.UpdatePosition(context.Background(), "nonexistent", 1000)
	assert.Equal(t, 0, len(s.torrentBuffers))
}

func TestService_ConcurrentAccess(t *testing.T) {
	s := NewService(1024)
	done := make(chan struct{})
	go func() {
		s.UpdatePosition(context.Background(), "test", 100)
		close(done)
	}()
	go func() {
		_, _ = s.GetBufferInfo(context.Background(), "test")
	}()
	<-done
	require.NotNil(t, s)
}

func TestService_UnregisterTorrent(t *testing.T) {
	s := NewService(1024)
	s.mu.Lock()
	s.torrentBuffers["t1"] = &TorrentBuffer{TorrentID: "t1"}
	s.torrentBuffers["t2"] = &TorrentBuffer{TorrentID: "t2"}
	s.mu.Unlock()

	s.UnregisterTorrent("t1")
	_, exists := s.torrentBuffers["t1"]
	assert.False(t, exists)
	_, exists = s.torrentBuffers["t2"]
	assert.True(t, exists)
}

func TestService_Close_WithEntries(t *testing.T) {
	s := NewService(1024)
	s.mu.Lock()
	s.torrentBuffers["t1"] = &TorrentBuffer{TorrentID: "t1", BufferPercent: 10}
	s.torrentBuffers["t2"] = &TorrentBuffer{TorrentID: "t2", BufferPercent: 20}
	s.mu.Unlock()
	s.Close()
	s.mu.Lock()
	assert.Equal(t, 0, len(s.torrentBuffers))
	s.mu.Unlock()
}

func TestService_UnregisterTorrent_Nonexistent(t *testing.T) {
	s := NewService(1024)
	s.UnregisterTorrent("nonexistent")
	assert.NotNil(t, s)
}

func TestService_MaxCacheSize(t *testing.T) {
	s := NewService(500)
	assert.Equal(t, int64(500), s.maxCacheSize)
}

func TestService_Close_Idempotent(t *testing.T) {
	s := NewService(1024)
	s.Close()
	s.Close()
	assert.Equal(t, 0, len(s.torrentBuffers))
}

func TestService_PeriodicUpdate_IdempotentStop(t *testing.T) {
	s := NewService(1024)
	s.StopPeriodicUpdate()
	s.StopPeriodicUpdate()
	s.StartPeriodicUpdate(50 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	s.StopPeriodicUpdate()
	assert.NotNil(t, s)
}

func TestService_NoGoroutineLeak(t *testing.T) {
	goroutinesBefore := runtime.NumGoroutine()

	for i := 0; i < 5; i++ {
		s := NewService(1024)
		s.StartPeriodicUpdate(50 * time.Millisecond)
		time.Sleep(20 * time.Millisecond)
		s.Close()
		time.Sleep(20 * time.Millisecond)
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	goroutinesAfter := runtime.NumGoroutine()
	assert.LessOrEqual(t, goroutinesAfter, goroutinesBefore+2,
		"Buffer service goroutine leak: was %d, now %d", goroutinesBefore, goroutinesAfter)
}
