// SPDX-License-Identifier: MIT

package buffer

import (
	"context"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	apperrors "github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
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

	// FileOffset is where the selected file begins inside the torrent. Piece
	// numbers are torrent-wide, so every translation between the playback
	// position (file-relative, as the client reports it) and a piece index has
	// to add this — without it the priorities of a second file in a multi-file
	// torrent land on the first file's pieces (bug-0150).
	FileOffset int64
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

	// Where the chosen file sits inside the torrent, and which one it is. A
	// position is in file bytes while a piece number is torrent-wide, so the
	// offset is the whole difference between steering the file being watched
	// and steering the first file in the release (bug-0150).
	fileIndex, fileOffset := 0, int64(0)
	for i, f := range file.Torrent().Files() {
		if f == file {
			fileIndex = i
			break
		}
		fileOffset += f.Length()
	}

	s.torrentBuffers[torrentID] = &TorrentBuffer{
		TorrentID:      torrentID,
		FileIndex:      fileIndex,
		FileOffset:     fileOffset,
		File:           wrapped,
		BufferPercent:  bufferPercent,
		BufferDuration: bufferDuration,
		MaxBufferSize:  maxBufferSize,
		PieceSize:      pieceSize,
		TotalPieces:    totalPieces,
		LastUpdate:     time.Now(),
	}

	logger.Info("Registered torrent for buffering",
		"torrentID", torrentID,
		"fileIndex", fileIndex,
		"fileOffset", fileOffset,
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
		return apperrors.NotFound("torrent", torrentID)
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

	// File bytes to torrent pieces: the offset goes in before the division.
	// The buffer is measured inside the file, the pieces are numbered inside
	// the torrent, and skipping the offset steered the first file's pieces
	// whenever the film was not the first file in the release (bug-0150).
	startPiece := int((tb.FileOffset + tb.BufferStart) / pieceSize)
	endPiece := int((tb.FileOffset + tb.BufferEnd) / pieceSize)

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
		return nil, apperrors.NotFound("torrent", torrentID)
	}

	t := tb.File.Torrent()
	pieceSize := t.Info().PieceLength
	// Same translation as in updatePiecePriorities: the count has to be about
	// the pieces of the file being watched, not about whatever piece numbers
	// the file's own byte offsets happen to land on (bug-0150).
	startPiece := int((tb.FileOffset + tb.BufferStart) / pieceSize)
	endPiece := int((tb.FileOffset + tb.BufferEnd) / pieceSize)

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
