// Package buffer предоставляет сервис управления буферизацией для стриминга.
package buffer

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// CacheEntry запись в LRU-кэше
type CacheEntry struct {
	PieceIndex int
	Data       []byte
	Size       int64
	AccessTime time.Time
}

// TorrentBuffer буфер для конкретного торрента
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

// Service сервис управления буферизацией
type Service struct {
	mu               sync.RWMutex
	torrentBuffers   map[string]*TorrentBuffer
	maxCacheSize     int64
	currentCacheSize int64
	lruList          *list.List
	lruMap           map[int]*list.Element
}

// NewService создаёт новый сервис буферизации
func NewService(maxCacheSize int64) *Service {
	return &Service{
		torrentBuffers: make(map[string]*TorrentBuffer),
		maxCacheSize:   maxCacheSize,
		lruList:        list.New(),
		lruMap:         make(map[int]*list.Element),
	}
}

// RegisterTorrent регистрирует торрент для буферизации
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
		PieceSize:      int64(pieceSize),
		TotalPieces:    totalPieces,
		LastUpdate:     time.Now(),
	}

	logger.Info("Registered torrent for buffering: %s, pieces: %d, piece size: %d",
		torrentID, totalPieces, pieceSize)
}

// UpdatePosition обновляет текущую позицию воспроизведения
func (s *Service) UpdatePosition(torrentID string, position int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tb, exists := s.torrentBuffers[torrentID]
	if !exists {
		return
	}

	tb.CurrentPosition = position
	tb.LastUpdate = time.Now()

	// Вычисляем границы буфера
	fileSize := tb.File.Length()
	bufferSize := int64(float64(fileSize) * float64(tb.BufferPercent) / 100.0)

	// Ограничиваем максимальный размер буфера
	if bufferSize > tb.MaxBufferSize {
		bufferSize = tb.MaxBufferSize
	}

	tb.BufferStart = position
	tb.BufferEnd = position + bufferSize

	if tb.BufferEnd > fileSize {
		tb.BufferEnd = fileSize
	}

	// Обновляем приоритеты кусков
	s.updatePiecePriorities(tb)
}

// updatePiecePriorities обновляет приоритеты кусков на основе текущей позиции
func (s *Service) updatePiecePriorities(tb *TorrentBuffer) {
	t := tb.File.Torrent()
	pieceSize := int64(t.Info().PieceLength)

	// Вычисляем индексы кусков для буфера
	startPiece := int(tb.BufferStart / pieceSize)
	endPiece := int(tb.BufferEnd / pieceSize)

	// Вычисляем индексы кусков для немедленной загрузки (первые ~10 кусков)
	nowEndPiece := startPiece + 10

	// Вычисляем индексы кусков для высокого приоритета (~50 кусков)
	highEndPiece := startPiece + 50

	for i := 0; i < tb.TotalPieces; i++ {
		piece := t.Piece(i)

		if i >= startPiece && i <= endPiece {
			// В пределах буфера
			if i <= nowEndPiece {
				// Немедленная загрузка
				piece.SetPriority(torrent.PiecePriorityNow)
			} else if i <= highEndPiece {
				// Высокий приоритет
				piece.SetPriority(torrent.PiecePriorityHigh)
			} else {
				// Обычный приоритет
				piece.SetPriority(torrent.PiecePriorityNormal)
			}
		} else if i > endPiece && i <= endPiece+20 {
			// Предзагрузка (20 кусков за пределами буфера)
			piece.SetPriority(torrent.PiecePriorityReadahead)
		} else if i < startPiece {
			// Уже проигранные куски - не загружать
			piece.SetPriority(torrent.PiecePriorityNone)
		} else {
			// Далёкие куски - низкий приоритет
			piece.SetPriority(torrent.PiecePriorityNone)
		}
	}
}

// GetBufferInfo возвращает информацию о состоянии буфера
func (s *Service) GetBufferInfo(torrentID string) (*models.BufferInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tb, exists := s.torrentBuffers[torrentID]
	if !exists {
		return nil, fmt.Errorf("torrent not found: %s", torrentID)
	}

	// Подсчитываем загруженные байты в буфере
	t := tb.File.Torrent()
	pieceSize := int64(t.Info().PieceLength)
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

// UnregisterTorrent удаляет торрент из буферизации
func (s *Service) UnregisterTorrent(torrentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.torrentBuffers, torrentID)
	logger.Info("Unregistered torrent from buffering: %s", torrentID)
}

// StartPeriodicUpdate запускает периодическое обновление приоритетов
func (s *Service) StartPeriodicUpdate(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			s.mu.Lock()
			for _, tb := range s.torrentBuffers {
				s.updatePiecePriorities(tb)
			}
			s.mu.Unlock()
		}
	}()
}
