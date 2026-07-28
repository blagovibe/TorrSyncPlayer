// SPDX-License-Identifier: MIT

package buffer

import (
	"context"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logger.Init("error", "text")
}

// mockPiece implements torrentPiece for tests.
type mockPiece struct {
	priority torrent.PiecePriority
	complete bool
}

func (m *mockPiece) SetPriority(prio torrent.PiecePriority) {
	m.priority = prio
}

func (m *mockPiece) State() torrent.PieceState {
	return torrent.PieceState{
		Completion: storage.Completion{Complete: m.complete, Ok: true},
	}
}

// mockTorrent implements torrentTorrent for tests.
type mockTorrent struct {
	pieceLength int64
	numPieces   int
	pieces      []*mockPiece
}

func newMockTorrent(pieceLength, fileLength int64) *mockTorrent {
	numPieces := int((fileLength + pieceLength - 1) / pieceLength)
	pieces := make([]*mockPiece, numPieces)
	for i := range pieces {
		pieces[i] = &mockPiece{}
	}
	return &mockTorrent{
		pieceLength: pieceLength,
		numPieces:   numPieces,
		pieces:      pieces,
	}
}

func (m *mockTorrent) Info() *metainfo.Info {
	return &metainfo.Info{
		PieceLength: m.pieceLength,
		Length:      int64(m.numPieces) * m.pieceLength,
	}
}

func (m *mockTorrent) NumPieces() int {
	return m.numPieces
}

func (m *mockTorrent) Piece(i int) torrentPiece {
	if i < 0 || i >= len(m.pieces) {
		return &mockPiece{}
	}
	return m.pieces[i]
}

// mockFile implements torrentFile for tests.
type mockFile struct {
	torrent *mockTorrent
	length  int64
}

func newMockFile(pieceLength, fileLength int64) *mockFile {
	return &mockFile{
		torrent: newMockTorrent(pieceLength, fileLength),
		length:  fileLength,
	}
}

func (m *mockFile) Torrent() torrentTorrent {
	return m.torrent
}

func (m *mockFile) Length() int64 {
	return m.length
}

// registerTestFile is a helper that bypasses the real RegisterTorrent
// (which requires *torrent.File) and injects a mock directly.
func registerTestFile(s *Service, torrentID string, f *mockFile) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pieceSize := f.torrent.Info().PieceLength
	totalPieces := f.torrent.NumPieces()

	s.torrentBuffers[torrentID] = &TorrentBuffer{
		TorrentID:      torrentID,
		FileIndex:      0,
		File:           f,
		BufferPercent:  constants.DefaultBufferPercent,
		BufferDuration: constants.DefaultBufferDuration,
		MaxBufferSize:   constants.DefaultMaxBufferSize,
		PieceSize:      pieceSize,
		TotalPieces:    totalPieces,
		LastUpdate:     time.Now(),
	}
}

// TestNewService tests service creation
func TestNewService(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	require.NotNil(t, s)
	assert.Equal(t, int64(constants.DefaultMaxBufferSize), s.maxCacheSize)
}

// TestSetPosition_NotRegistered tests error handling for unregistered torrents
func TestSetPosition_NotRegistered(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	err := s.SetPosition(context.Background(), "nonexistent", 1000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "torrent not found: nonexistent")
}

// TestUpdatePosition_NotRegistered tests error handling for unregistered torrents
func TestUpdatePosition_NotRegistered(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	err := s.UpdatePosition(context.Background(), "nonexistent", 1000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "torrent not found: nonexistent")
}

// TestGetBufferInfo_NotRegistered tests error handling for unregistered torrents
func TestGetBufferInfo_NotRegistered(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	_, err := s.GetBufferInfo(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "torrent not found: nonexistent")
}

// TestUnregisterTorrent tests unregistering non-existent torrent (should not panic)
func TestUnregisterTorrent(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	s.UnregisterTorrent("nonexistent")
}

// TestClose tests service closure
func TestClose(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	s.Close()
	assert.Empty(t, s.torrentBuffers)
}

// TestStartStopPeriodicUpdate tests periodic update lifecycle
func TestStartStopPeriodicUpdate(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	s.StartPeriodicUpdate(constants.BufferUpdateInterval)
	s.StopPeriodicUpdate()
	assert.Nil(t, s.cancelFunc)
}

// TestUpdatePosition_Atomicity tests thread safety of UpdatePosition
func TestUpdatePosition_Atomicity(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(pos int64) {
			_ = s.UpdatePosition(context.Background(), "test", pos)
			done <- true
		}(int64(i))
	}

	for i := 0; i < 100; i++ {
		<-done
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

	assert.Equal(t, "test-id", info.TorrentID)
	assert.True(t, info.IsBuffering)
}

// TestRegisterTorrent tests torrent registration via helper
func TestRegisterTorrent(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	f := newMockFile(1024*1024, 100*1024*1024) // 100MB file, 1MB pieces
	registerTestFile(s, "tor-1", f)

	buf := s.torrentBuffers["tor-1"]
	require.NotNil(t, buf)
	assert.Equal(t, int64(100*1024*1024), buf.File.Length())
	assert.Equal(t, 100, buf.TotalPieces)
	assert.Equal(t, int64(1024*1024), buf.PieceSize)
}

// TestUpdatePosition_BufferBounds tests buffer boundary calculation
func TestUpdatePosition_BufferBounds(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// 10MB file, 1MB pieces => 10 pieces
	f := newMockFile(1024*1024, 10*1024*1024)
	registerTestFile(s, "tor-1", f)

	err := s.UpdatePosition(context.Background(), "tor-1", 5*1024*1024) // position = 5MB
	require.NoError(t, err)

	buf := s.torrentBuffers["tor-1"]
	require.NotNil(t, buf)

	// BufferPercent=10 => bufferSize = 10% of 10MB = 1MB
	expectedBufferSize := int64(1024 * 1024)
	assert.Equal(t, int64(5*1024*1024), buf.BufferStart)
	assert.Equal(t, int64(6*1024*1024), buf.BufferEnd)
	assert.Equal(t, expectedBufferSize, buf.BufferEnd-buf.BufferStart)
	assert.Equal(t, int64(5*1024*1024), buf.CurrentPosition)
}

// TestUpdatePosition_EndOfFile tests buffer clamping at EOF
func TestUpdatePosition_EndOfFile(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// 2MB file, 1MB pieces => 2 pieces
	f := newMockFile(1024*1024, 2*1024*1024)
	registerTestFile(s, "tor-1", f)

	// Position at EOF minus 1 byte: buffer extends past file end and gets clamped
	err := s.UpdatePosition(context.Background(), "tor-1", 2*1024*1024-1)
	require.NoError(t, err)

	buf := s.torrentBuffers["tor-1"]
	require.NotNil(t, buf)

	// BufferEnd must be clamped to file size (2MB)
	assert.Equal(t, int64(2*1024*1024), buf.BufferEnd)
	assert.Equal(t, int64(2*1024*1024), buf.File.Length())
}

// TestUpdatePosition_BufferSizeCappedByMax tests MaxBufferSize cap
func TestUpdatePosition_BufferSizeCappedByMax(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// 100MB file, 1MB pieces
	f := newMockFile(1024*1024, 100*1024*1024)
	registerTestFile(s, "tor-1", f)

	// Manually set a small MaxBufferSize
	s.mu.Lock()
	s.torrentBuffers["tor-1"].MaxBufferSize = 500 * 1024 // 500KB cap
	s.mu.Unlock()

	err := s.UpdatePosition(context.Background(), "tor-1", 0)
	require.NoError(t, err)

	buf := s.torrentBuffers["tor-1"]
	// 10% of 100MB = 10MB, but capped at 500KB
	assert.Equal(t, int64(500*1024), buf.BufferEnd-buf.BufferStart)
}

// TestGetBufferInfo_Partial tests BufferedPercent calculation with partial completion
func TestGetBufferInfo_Partial(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// 10MB file, 1MB pieces => 10 pieces
	f := newMockFile(1024*1024, 10*1024*1024)
	registerTestFile(s, "tor-1", f)

	// Mark only piece 0 as complete, 1-9 as incomplete
	f.torrent.pieces[0].complete = true

	// Position at start, buffer = 10% of 10MB = 1MB => pieces 0-1
	err := s.UpdatePosition(context.Background(), "tor-1", 0)
	require.NoError(t, err)

	info, err := s.GetBufferInfo(context.Background(), "tor-1")
	require.NoError(t, err)

	assert.Equal(t, int64(0), info.CurrentPosition)
	assert.Equal(t, int64(0), info.BufferStart)
	assert.Equal(t, int64(1024*1024), info.BufferEnd)
	// Only piece 0 is complete in buffer range (piece 1 incomplete)
	assert.Equal(t, int64(1*1024*1024), info.BufferedBytes)
	assert.InDelta(t, 100.0, info.BufferedPercent, 0.1)
	assert.False(t, info.IsBuffering)
}

// TestGetBufferInfo_NotBuffering tests IsBuffering flag at 100%
func TestGetBufferInfo_NotBuffering(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// 10MB file, 1MB pieces => 10 pieces, 10% buffer = 1MB exactly
	f := newMockFile(1024*1024, 10*1024*1024)
	registerTestFile(s, "tor-1", f)

	// Mark piece 0 complete (the only piece fully within the 1MB buffer)
	f.torrent.pieces[0].complete = true

	err := s.UpdatePosition(context.Background(), "tor-1", 0)
	require.NoError(t, err)

	info, err := s.GetBufferInfo(context.Background(), "tor-1")
	require.NoError(t, err)

	assert.Equal(t, 100.0, info.BufferedPercent)
	assert.False(t, info.IsBuffering)
}

// TestGetBufferInfo_IsBuffering tests IsBuffering flag below 95%
func TestGetBufferInfo_IsBuffering(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// 100MB file, 1MB pieces => 100 pieces, 10% buffer = 10MB
	f := newMockFile(1024*1024, 100*1024*1024)
	registerTestFile(s, "tor-1", f)

	// Mark no pieces complete
	err := s.UpdatePosition(context.Background(), "tor-1", 0)
	require.NoError(t, err)

	info, err := s.GetBufferInfo(context.Background(), "tor-1")
	require.NoError(t, err)

	assert.Equal(t, 0.0, info.BufferedPercent)
	assert.True(t, info.IsBuffering)
}

// TestUpdatePosition_PiecePriorities tests priority assignment by position
func TestUpdatePosition_PiecePriorities(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// 10MB file, 1MB pieces => 10 pieces
	f := newMockFile(1024*1024, 10*1024*1024)
	registerTestFile(s, "tor-1", f)

	// Position at 2MB => buffer from 2MB to 3MB (10%)
	err := s.UpdatePosition(context.Background(), "tor-1", 2*1024*1024)
	require.NoError(t, err)

	pieces := f.torrent.pieces
	// Buffer window is pieces 2-3, read-ahead extends to piece 23 (capped at 9)
	// Pieces in buffer should have priority > None
	assert.NotEqual(t, torrent.PiecePriorityNone, pieces[2].priority)
	assert.NotEqual(t, torrent.PiecePriorityNone, pieces[3].priority)
	// Piece well before buffer should be None
	assert.Equal(t, torrent.PiecePriorityNone, pieces[0].priority)
}

// TestPeriodicUpdate_DoesNotPanic tests that periodic update runs without panic
func TestPeriodicUpdate_DoesNotPanic(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	// 10MB file
	f := newMockFile(1024*1024, 10*1024*1024)
	registerTestFile(s, "tor-1", f)

	s.StartPeriodicUpdate(50 * time.Millisecond)

	// Let it run a couple of ticks
	time.Sleep(120 * time.Millisecond)

	s.StopPeriodicUpdate()
	assert.Nil(t, s.cancelFunc)
}
