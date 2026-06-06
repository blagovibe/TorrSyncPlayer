package buffer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

type CacheEntry struct {
	PieceIndex int
	Data       []byte
	Size       int64
	AccessTime time.Time
}

type TorrentBuffer struct {
	TorrentID       string
	FileIndex       int
	File            *torrent.File
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
	mu               sync.RWMutex
	torrentBuffers   map[string]*TorrentBuffer
	maxCacheSize     int64
	currentCacheSize int64
	cancelFunc       context.CancelFunc
	wg               sync.WaitGroup
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

	pieceSize := file.Torrent().Info().PieceLength
	totalPieces := file.Torrent().NumPieces()

	s.torrentBuffers[torrentID] = &TorrentBuffer{
		TorrentID:      torrentID,
		FileIndex:      0,
		File:           file,
		BufferPercent:  bufferPercent,
		BufferDuration: bufferDuration,
		MaxBufferSize:  maxBufferSize,
		PieceSize:      pieceSize,
		TotalPieces:    totalPieces,
		LastUpdate:     time.Now(),
	}

	logger.Info("Registered torrent for buffering",
		"torrentID", torrentID,
		"totalPieces", totalPieces,
		"pieceSize", pieceSize)
}

func (s *Service) UpdatePosition(torrentID string, position int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tb, exists := s.torrentBuffers[torrentID]
	if !exists {
		return
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
}

func (s *Service) updatePiecePriorities(tb *TorrentBuffer) {
	t := tb.File.Torrent()
	pieceSize := t.Info().PieceLength

	startPiece := int(tb.BufferStart / pieceSize)
	endPiece := int(tb.BufferEnd / pieceSize)

	nowEndPiece := startPiece + constants.BufferNowPieces
	highEndPiece := startPiece + constants.BufferHighPieces

	for i := 0; i < tb.TotalPieces; i++ {
		piece := t.Piece(i)

		if i >= startPiece && i <= endPiece {
			if i <= nowEndPiece {
				piece.SetPriority(torrent.PiecePriorityNow)
			} else if i <= highEndPiece {
				piece.SetPriority(torrent.PiecePriorityHigh)
			} else {
				piece.SetPriority(torrent.PiecePriorityNormal)
			}
		} else if i > endPiece && i <= endPiece+20 {
			piece.SetPriority(torrent.PiecePriorityReadahead)
		} else if i < startPiece {
			piece.SetPriority(torrent.PiecePriorityNone)
		} else {
			piece.SetPriority(torrent.PiecePriorityNone)
		}
	}
}

func (s *Service) GetBufferInfo(torrentID string) (*models.BufferInfo, error) {
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
	s.mu.Lock()
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.wg.Wait()
	}
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
