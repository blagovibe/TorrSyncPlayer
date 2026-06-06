package buffer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	s := NewService(1024 * 1024)
	assert.NotNil(t, s)
	assert.Equal(t, int64(1024*1024), s.maxCacheSize)
}

func TestService_GetBufferInfo_NotFound(t *testing.T) {
	s := NewService(1024)
	_, err := s.GetBufferInfo("nonexistent")
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

func TestCacheEntry(t *testing.T) {
	entry := CacheEntry{
		PieceIndex: 42,
		Data:       []byte("test"),
		Size:       4,
		AccessTime: time.Now(),
	}
	assert.Equal(t, 42, entry.PieceIndex)
	assert.Equal(t, int64(4), entry.Size)
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
	s.UpdatePosition("nonexistent", 1000)
	assert.Equal(t, 0, len(s.torrentBuffers))
}

func TestService_ConcurrentAccess(t *testing.T) {
	s := NewService(1024)
	done := make(chan struct{})
	go func() {
		s.UpdatePosition("test", 100)
		close(done)
	}()
	go func() {
		_, _ = s.GetBufferInfo("test")
	}()
	<-done
	require.NotNil(t, s)
}
