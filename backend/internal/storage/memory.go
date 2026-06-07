// Package storage предоставляет in-memory хранилище для торрент-данных.
// Все данные хранятся в оперативной памяти без записи на диск.
package storage

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// memoryPieceImpl реализует storage.PieceImpl для хранения данных куска в RAM.
type memoryPieceImpl struct {
	mu             sync.RWMutex
	data           []byte
	length         int64
	complete       bool
	bytesAllocated int64
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
	p.bytesAllocated += int64(n)
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
	storage     *memoryStorage
	totalBytes  atomic.Int64
}

// newMemoryPieceProvider создаёт новый провайдер кусков в памяти.
func newMemoryPieceProvider(infoHash metainfo.Hash, pieceLength int64, storage *memoryStorage) *memoryPieceProvider {
	return &memoryPieceProvider{
		pieces:      make(map[int]*memoryPieceImpl),
		pieceLength: pieceLength,
		infoHash:    infoHash,
		storage:     storage,
	}
}

// Piece возвращает storage.PieceImpl для указанного куска.
func (p *memoryPieceProvider) Piece(piece metainfo.Piece) storage.PieceImpl {
	p.mu.RLock()
	index := piece.Index()
	if impl, exists := p.pieces[index]; exists {
		p.mu.RUnlock()
		return impl
	}
	p.mu.RUnlock()

	if p.storage.capacity > 0 {
		currentUsed := p.storage.used.Load()
		if currentUsed+piece.Length() > p.storage.capacity {
			return &memoryPieceImpl{
				data:   nil,
				length: piece.Length(),
			}
		}
	}

	impl := &memoryPieceImpl{
		data:   nil,
		length: piece.Length(),
	}

	p.storage.used.Add(piece.Length())
	p.totalBytes.Add(piece.Length())

	p.mu.Lock()
	p.pieces[index] = impl
	p.mu.Unlock()
	return impl
}

// Close закрывает провайдер и освобождает память.
func (p *memoryPieceProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.storage.used.Add(-p.totalBytes.Load())
	p.pieces = make(map[int]*memoryPieceImpl)
	return nil
}

// memoryStorage реализует storage.ClientImpl для хранения данных в RAM.
type memoryStorage struct {
	mu        sync.RWMutex
	providers map[metainfo.Hash]*memoryPieceProvider
	capacity  int64
	used      atomic.Int64
}

// NewMemoryStorage создаёт новое in-memory хранилище для торрентов.
// capacity - максимальный размер хранилища в байтах (0 = без ограничений).
func NewMemoryStorage(capacity int64) storage.ClientImpl {
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
		provider = newMemoryPieceProvider(infoHash, info.PieceLength, s)
		s.providers[infoHash] = provider
	}

	return storage.TorrentImpl{
		Piece: func(p metainfo.Piece) storage.PieceImpl {
			return provider.Piece(p)
		},
		Close: func() error {
			return provider.Close()
		},
	}, nil
}

// GetUsed возвращает текущее использование хранилища.
func (s *memoryStorage) GetUsed() int64 {
	return s.used.Load()
}

// GetCapacity возвращает ёмкость хранилища.
func (s *memoryStorage) GetCapacity() int64 {
	return s.capacity
}

// Close закрывает хранилище и освобождает все ресурсы.
func (s *memoryStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers = make(map[metainfo.Hash]*memoryPieceProvider)
	s.used.Store(0)
	return nil
}
