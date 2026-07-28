// SPDX-License-Identifier: MIT

package buffer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// torrentFile abstracts *torrent.File for testability.
type torrentFile interface {
	Torrent() torrentTorrent
	Length() int64
}

// torrentTorrent abstracts *torrent.Torrent for testability.
type torrentTorrent interface {
	Info() *metainfo.Info
	NumPieces() int
	Piece(i int) torrentPiece
}

// torrentPiece abstracts *torrent.Piece for testability.
type torrentPiece interface {
	SetPriority(prio torrent.PiecePriority)
	State() torrent.PieceState
}

// realTorrent wraps *torrent.Torrent to satisfy torrentTorrent.
type realTorrent struct {
	*torrent.Torrent
}

func (r *realTorrent) Piece(i int) torrentPiece {
	return r.Torrent.Piece(i)
}

// realPiece wraps *torrent.Piece to satisfy torrentPiece.
type realPiece struct {
	*torrent.Piece
}

func (r *realPiece) State() torrent.PieceState {
	return r.Piece.State()
}

// realFile wraps *torrent.File to satisfy torrentFile.
type realFile struct {
	file *torrent.File
}

func (r *realFile) Torrent() torrentTorrent {
	return &realTorrent{Torrent: r.file.Torrent()}
}

func (r *realFile) Length() int64 {
	return r.file.Length()
}

// wrapFile converts *torrent.File to torrentFile.
func wrapFile(f *torrent.File) torrentFile {
	return &realFile{file: f}
}

type TorrentBuffer struct {
	TorrentID       string
	FileIndex       int
	File            torrentFile
	CurrentPosition int64
	BufferPercent   int
	BufferDuration  int
	MaxBufferSize   int64
	PieceSize       int64
	TotalPieces     int
	BufferStart     int64
	BufferEnd       int64
	LastUpdate      time.Time
}

type Service struct {
	mu             sync.RWMutex
	torrentBuffers map[string]*TorrentBuffer
	maxCacheSize   int64
	cancelFunc     context.CancelFunc
	wg             sync.WaitGroup
}

func NewService(maxCacheSize int64) *Service {
	return &Service{
		torrentBuffers: make(map[string]*TorrentBuffer),
		maxCacheSize:   maxCacheSize,
	}
}

func (s *Service) RegisterTorrent(torrentID string, file *torrent.File, bufferPercent, bufferDuration int, maxBufferSize int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	wrapped := wrapFile(file)
	pieceSize := wrapped.Torrent().Info().PieceLength
	totalPieces := wrapped.Torrent().NumPieces()

	s.torrentBuffers[torrentID] = &TorrentBuffer{
		TorrentID:      torrentID,
		FileIndex:      0,
		File:           wrapped,
		BufferPercent:  bufferPercent,
		BufferDuration: bufferDuration,
		MaxBufferSize:   maxBufferSize,
		PieceSize:      pieceSize,
		TotalPieces:    totalPieces,
		LastUpdate:     time.Now(),
	}

	logger.Info("Registered torrent for buffering",
		"torrentID", torrentID,
		"totalPieces", totalPieces,
		"pieceSize", pieceSize)
}

// SetPosition sets the buffer position for a torrent.
// Returns an error if the torrent is not found.
// Alias for UpdatePosition for interface compatibility.
func (s *Service) SetPosition(ctx context.Context, torrentID string, position int64) error {
	_ = ctx // Context is reserved for future cancellation support
	return s.UpdatePosition(ctx, torrentID, position)
}

// UpdatePosition updates the current playback position for buffering.
func (s *Service) UpdatePosition(ctx context.Context, torrentID string, position int64) error {
	_ = ctx // Context is reserved for future cancellation support
	s.mu.Lock()
	defer s.mu.Unlock()

	tb, exists := s.torrentBuffers[torrentID]
	if !exists {
		return fmt.Errorf("torrent not found: %s", torrentID)
	}

	tb.CurrentPosition = position
	tb.LastUpdate = time.Now()

	fileSize := tb.File.Length()
	bufferSize := int64(float64(fileSize) * float64(tb.BufferPercent) / 100.0)

	if bufferSize > tb.MaxBufferSize {
		bufferSize = tb.MaxBufferSize
	}

	tb.BufferStart = position
	tb.BufferEnd = position + bufferSize

	if tb.BufferEnd > fileSize {
		tb.BufferEnd = fileSize
	}

	s.updatePiecePriorities(tb)

	return nil
}

func (s *Service) updatePiecePriorities(tb *TorrentBuffer) {
	t := tb.File.Torrent()
	pieceSize := t.Info().PieceLength

	startPiece := int(tb.BufferStart / pieceSize)
	endPiece := int(tb.BufferEnd / pieceSize)

	nowEndPiece := startPiece + constants.BufferNowPieces
	highEndPiece := startPiece + constants.BufferHighPieces
	readAheadEnd := endPiece + constants.BufferReadAheadPieces

	// Scan only the relevant range instead of all pieces
	low := 0
	high := tb.TotalPieces - 1
	if readAheadEnd < high {
		high = readAheadEnd
	}
	if startPiece-1 > low {
		low = startPiece - 1
	}

	for i := low; i <= high; i++ {
		piece := t.Piece(i)

		if i >= startPiece && i <= endPiece {
			if i <= nowEndPiece {
				piece.SetPriority(torrent.PiecePriorityNow)
			} else if i <= highEndPiece {
				piece.SetPriority(torrent.PiecePriorityHigh)
			} else {
				piece.SetPriority(torrent.PiecePriorityNormal)
			}
		} else if i > endPiece && i <= readAheadEnd {
			piece.SetPriority(torrent.PiecePriorityReadahead)
		} else {
			piece.SetPriority(torrent.PiecePriorityNone)
		}
	}

	// Reset priority for pieces outside the range
	for i := 0; i < low; i++ {
		t.Piece(i).SetPriority(torrent.PiecePriorityNone)
	}
	for i := high + 1; i < tb.TotalPieces; i++ {
		t.Piece(i).SetPriority(torrent.PiecePriorityNone)
	}
}

func (s *Service) GetBufferInfo(ctx context.Context, torrentID string) (*models.BufferInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tb, exists := s.torrentBuffers[torrentID]
	if !exists {
		return nil, fmt.Errorf("torrent not found: %s", torrentID)
	}

	t := tb.File.Torrent()
	pieceSize := t.Info().PieceLength
	startPiece := int(tb.BufferStart / pieceSize)
	endPiece := int(tb.BufferEnd / pieceSize)

	var bufferedBytes int64
	for i := startPiece; i <= endPiece && i < tb.TotalPieces; i++ {
		if t.Piece(i).State().Complete {
			bufferedBytes += pieceSize
		}
	}

	bufferSize := tb.BufferEnd - tb.BufferStart
	bufferedPercent := 0.0
	if bufferSize > 0 {
		bufferedPercent = float64(bufferedBytes) / float64(bufferSize) * 100.0
	}

	return &models.BufferInfo{
		TorrentID:       torrentID,
		FileIndex:       tb.FileIndex,
		CurrentPosition: tb.CurrentPosition,
		BufferStart:     tb.BufferStart,
		BufferEnd:       tb.BufferEnd,
		BufferSize:      bufferSize,
		BufferedBytes:   bufferedBytes,
		BufferedPercent: bufferedPercent,
		IsBuffering:     bufferedPercent < 95.0,
	}, nil
}

func (s *Service) UnregisterTorrent(torrentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.torrentBuffers, torrentID)
	logger.Info("Unregistered torrent from buffering", "torrentID", torrentID)
}

func (s *Service) StartPeriodicUpdate(interval time.Duration) {
	s.StopPeriodicUpdate()

	s.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFunc = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.mu.Lock()
				for _, tb := range s.torrentBuffers {
					s.updatePiecePriorities(tb)
				}
				s.mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *Service) StopPeriodicUpdate() {
	s.mu.Lock()
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Service) Close() {
	s.StopPeriodicUpdate()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.torrentBuffers = make(map[string]*TorrentBuffer)
}
