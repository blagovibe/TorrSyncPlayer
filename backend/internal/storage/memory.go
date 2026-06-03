// Package storage предоставляет in-memory хранилище для торрент-данных.
// Все данные хранятся в оперативной памяти без записи на диск.
package storage

import (
	"context"
	"sync"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// pieceKey уникальный идентификатор куска торрента.
type pieceKey struct {
	infoHash metainfo.Hash
	index    int
}

// memoryPieceImpl реализует storage.PieceImpl для хранения данных куска в RAM.
type memoryPieceImpl struct {
	mu       sync.RWMutex
	data     []byte
	length   int64
	complete bool
}

// ReadAt читает данные из куска.
func (p *memoryPieceImpl) ReadAt(b []byte, off int64) (int, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if off >= int64(len(p.data)) {
		return 0, nil
	}

	end := off + int64(len(b))
	if end > int64(len(p.data)) {
		end = int64(len(p.data))
	}
	if off >= end {
		return 0, nil
	}

	n := copy(b, p.data[off:end])
	return n, nil
}

// WriteAt записывает данные в кусок.
func (p *memoryPieceImpl) WriteAt(b []byte, off int64) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Инициализируем буфер если нужно
	if p.data == nil {
		p.data = make([]byte, p.length)
	}

	end := off + int64(len(b))
	if end > p.length {
		end = p.length
	}
	if off >= p.length {
		return 0, nil
	}

	n := copy(p.data[off:end], b)
	return n, nil
}

// MarkComplete помечает кусок как завершённый.
func (p *memoryPieceImpl) MarkComplete() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.complete = true
	return nil
}

// MarkNotComplete помечает кусок как незавершённый.
func (p *memoryPieceImpl) MarkNotComplete() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.complete = false
	return nil
}

// Completion возвращает статус завершения куска.
func (p *memoryPieceImpl) Completion() storage.Completion {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return storage.Completion{
		Complete: p.complete,
		Ok:       true,
	}
}

// memoryPieceProvider хранит куски одного торрента в памяти.
type memoryPieceProvider struct {
	mu          sync.RWMutex
	pieces      map[int]*memoryPieceImpl
	pieceLength int64
	infoHash    metainfo.Hash
}

// newMemoryPieceProvider создаёт новый провайдер кусков в памяти.
func newMemoryPieceProvider(infoHash metainfo.Hash, pieceLength int64) *memoryPieceProvider {
	return &memoryPieceProvider{
		pieces:      make(map[int]*memoryPieceImpl),
		pieceLength: pieceLength,
		infoHash:    infoHash,
	}
}

// Piece возвращает storage.PieceImpl для указанного куска.
func (p *memoryPieceProvider) Piece(piece metainfo.Piece) storage.PieceImpl {
	p.mu.Lock()
	defer p.mu.Unlock()

	index := piece.Index()
	if impl, exists := p.pieces[index]; exists {
		return impl
	}

	impl := &memoryPieceImpl{
		data:   nil,
		length: piece.Length(),
	}
	p.pieces[index] = impl
	return impl
}

// Close закрывает провайдер и освобождает память.
func (p *memoryPieceProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pieces = make(map[int]*memoryPieceImpl)
	return nil
}

// memoryStorage реализует storage.ClientImpl для хранения данных в RAM.
type memoryStorage struct {
	mu        sync.RWMutex
	providers map[metainfo.Hash]*memoryPieceProvider
	capacity  int64
	used      int64
}

// NewMemoryStorage создаёт новое in-memory хранилище для торрентов.
// capacity - максимальный размер хранилища в байтах (0 = без ограничений).
func NewMemoryStorage(capacity int64) storage.ClientImpl {
	logger.Info("Storage: создание in-memory хранилища", "capacity", capacity)
	return &memoryStorage{
		providers: make(map[metainfo.Hash]*memoryPieceProvider),
		capacity:  capacity,
	}
}

// OpenTorrent открывает торрент для хранения в памяти.
func (s *memoryStorage) OpenTorrent(ctx context.Context, info *metainfo.Info, infoHash metainfo.Hash) (storage.TorrentImpl, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	provider, exists := s.providers[infoHash]
	if !exists {
		provider = newMemoryPieceProvider(infoHash, int64(info.PieceLength))
		s.providers[infoHash] = provider
	}

	logger.Debug("Storage: открыт торрент в памяти", "infoHash", infoHash)

	return storage.TorrentImpl{
		Piece: func(p metainfo.Piece) storage.PieceImpl {
			return provider.Piece(p)
		},
		Close: func() error {
			return nil
		},
		Flush: func() error {
			return nil
		},
	}, nil
}

// GetUsed возвращает текущее использование хранилища.
func (s *memoryStorage) GetUsed() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.used
}

// GetCapacity возвращает ёмкость хранилища.
func (s *memoryStorage) GetCapacity() int64 {
	return s.capacity
}
