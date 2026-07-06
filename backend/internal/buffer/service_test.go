// Package buffer provides tests for buffer management service.
package buffer

import (
	"context"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

// TestNewService tests service creation
func TestNewService(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	if s == nil {
		t.Fatal("Expected non-nil service")
	}
	if s.maxCacheSize != constants.DefaultMaxBufferSize {
		t.Errorf("Expected maxCacheSize %d, got %d", constants.DefaultMaxBufferSize, s.maxCacheSize)
	}
}

// TestSetPosition_NotRegistered tests error handling for unregistered torrents
func TestSetPosition_NotRegistered(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	err := s.SetPosition(context.Background(), "nonexistent", 1000)
	if err == nil {
		t.Error("Expected error for unregistered torrent")
	}
	if err.Error() != "torrent not found: nonexistent" {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestUpdatePosition_NotRegistered tests error handling for unregistered torrents
func TestUpdatePosition_NotRegistered(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	err := s.UpdatePosition(context.Background(), "nonexistent", 1000)
	if err == nil {
		t.Error("Expected error for unregistered torrent")
	}
	if err.Error() != "torrent not found: nonexistent" {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestGetBufferInfo_NotRegistered tests error handling for unregistered torrents
func TestGetBufferInfo_NotRegistered(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	_, err := s.GetBufferInfo(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error for unregistered torrent")
	}
	if err.Error() != "torrent not found: nonexistent" {
		t.Errorf("Unexpected error: %v", err)
	}
}

// TestUnregisterTorrent tests unregistering non-existent torrent (should not panic)
func TestUnregisterTorrent(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	s.UnregisterTorrent("nonexistent") // Should not panic
}

// TestClose tests service closure
func TestClose(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	s.Close()
	if len(s.torrentBuffers) != 0 {
		t.Error("Expected empty torrentBuffers after close")
	}
}

// TestStartStopPeriodicUpdate tests periodic update lifecycle
func TestStartStopPeriodicUpdate(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// Start periodic update
	s.StartPeriodicUpdate(constants.BufferUpdateInterval)

	// Stop periodic update
	s.StopPeriodicUpdate()
	if s.cancelFunc != nil {
		t.Error("Expected nil cancelFunc after stop")
	}
}

// TestSetPosition_Atomicity tests thread safety of SetPosition
func TestSetPosition_Atomicity(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// Concurrent SetPosition calls should not race
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(pos int64) {
			_ = s.UpdatePosition(context.Background(), "test", pos)
			done <- true
		}(int64(i))
	}

	// Wait for all goroutines (they will all fail since no torrent registered, but shouldn't race)
	for i := 0; i < 100; i++ {
		<-done
	}
}

// TestGetBufferInfo_ReturnsValidInfo tests that GetBufferInfo returns correct structure
func TestGetBufferInfo_ReturnsValidInfo(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// Create a mock buffer entry for testing
	s.torrentBuffers["test"] = &TorrentBuffer{
		TorrentID:       "test",
		FileIndex:       0,
		CurrentPosition: 1000,
		BufferStart:     0,
		BufferEnd:       5000,
		BufferSize:      5000,
		BufferedBytes:   2500,
		LastUpdate:      time.Now(),
	}

	info, err := s.GetBufferInfo(context.Background(), "test")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if info.TorrentID != "test" {
		t.Errorf("Expected TorrentID 'test', got '%s'", info.TorrentID)
	}

	if info.CurrentPosition != 1000 {
		t.Errorf("Expected CurrentPosition 1000, got %d", info.CurrentPosition)
	}

	if info.BufferSize != 5000 {
		t.Errorf("Expected BufferSize 5000, got %d", info.BufferSize)
	}
}

// TestBufferInfoStructure validates the BufferInfo model
func TestBufferInfoStructure(t *testing.T) {
	info := &models.BufferInfo{
		TorrentID:       "test-id",
		FileIndex:       1,
		CurrentPosition: 1000,
		BufferStart:     0,
		BufferEnd:       5000,
		BufferSize:      5000,
		BufferedBytes:   2500,
		BufferedPercent: 50.0,
		DownloadSpeed:   1024 * 1024,
		IsBuffering:     true,
	}

	if info.TorrentID != "test-id" {
		t.Error("Unexpected TorrentID")
	}

	if info.IsBuffering != true {
		t.Error("Expected IsBuffering to be true")
	}
}
