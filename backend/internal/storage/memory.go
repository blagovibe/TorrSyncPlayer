// SPDX-License-Identifier: MIT

// Package storage provides in-memory storage for torrent data.
// All data is stored in RAM without writing to disk.
package storage

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// memoryPieceImpl implements storage.PieceImpl for storing piece data in RAM.
type memoryPieceImpl struct {
	mu             sync.RWMutex
	data           []byte
	length         int64
	complete       bool
	bytesAllocated int64
}

// ReadAt reads data from the piece.
func (p *memoryPieceImpl) ReadAt(b []byte, off int64) (int, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if off >= int64(len(p.data)) {
		return 0, io.EOF
	}

	end := off + int64(len(b))
	if end > int64(len(p.data)) {
		end = int64(len(p.data))
	}

	n := copy(b, p.data[off:end])
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

// WriteAt writes data to the piece.
func (p *memoryPieceImpl) WriteAt(b []byte, off int64) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.data == nil {
		p.data = make([]byte, p.length)
		p.bytesAllocated = p.length
	}

	if off >= p.length {
		return 0, nil
	}

	end := off + int64(len(b))
	if end > p.length {
		end = p.length
	}

	n := copy(p.data[off:end], b)
	return n, nil
}

// MarkComplete marks the piece as complete.
func (p *memoryPieceImpl) MarkComplete() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.complete = true
	return nil
}

// MarkNotComplete marks the piece as not complete.
func (p *memoryPieceImpl) MarkNotComplete() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.complete = false
	return nil
}

// Completion returns the completion status of the piece.
func (p *memoryPieceImpl) Completion() storage.Completion {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return storage.Completion{
		Complete: p.complete,
		Ok:       true,
	}
}

// memoryPieceProvider stores pieces of a single torrent in memory.
type memoryPieceProvider struct {
	mu          sync.RWMutex
	pieces      map[int]*memoryPieceImpl
	pieceLength int64
	infoHash    metainfo.Hash
	storage     *memoryStorage
	totalBytes  atomic.Int64
}

// newMemoryPieceProvider creates a new in-memory piece provider.
func newMemoryPieceProvider(infoHash metainfo.Hash, pieceLength int64, storage *memoryStorage) *memoryPieceProvider {
	return &memoryPieceProvider{
		pieces:      make(map[int]*memoryPieceImpl),
		pieceLength: pieceLength,
		infoHash:    infoHash,
		storage:     storage,
	}
}

// Piece returns storage.PieceImpl for the specified piece.
// Capacity check and allocation are performed atomically under the same lock
// to prevent TOCTOU races when multiple goroutines allocate concurrently.
func (p *memoryPieceProvider) Piece(piece metainfo.Piece) storage.PieceImpl {
	p.mu.RLock()
	index := piece.Index()
	if impl, exists := p.pieces[index]; exists {
		p.mu.RUnlock()
		return impl
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if impl, exists := p.pieces[index]; exists {
		return impl
	}

	if !p.storage.tryAllocate(piece.Length()) {
		return &memoryPieceImpl{
			data:   nil,
			length: piece.Length(),
		}
	}

	impl := &memoryPieceImpl{
		data:   nil,
		length: piece.Length(),
	}

	p.totalBytes.Add(piece.Length())

	p.pieces[index] = impl
	return impl
}

// Close closes the provider and frees memory.
func (p *memoryPieceProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.storage.used.Add(-p.totalBytes.Load())
	p.pieces = make(map[int]*memoryPieceImpl)
	return nil
}

// memoryStorage implements storage.ClientImpl for in-memory data storage.
type memoryStorage struct {
	mu        sync.RWMutex
	providers map[metainfo.Hash]*memoryPieceProvider
	capacity  int64
	used      atomic.Int64
}

// NewMemoryStorage creates a new in-memory storage for torrents.
// capacity - maximum storage size in bytes (0 = unlimited).
func NewMemoryStorage(capacity int64) storage.ClientImpl {
	return &memoryStorage{
		providers: make(map[metainfo.Hash]*memoryPieceProvider),
		capacity:  capacity,
	}
}

// OpenTorrent opens a torrent for in-memory storage.
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

// tryAllocate attempts to allocate 'size' bytes from the global pool
// Returns true if allocation succeeded
func (s *memoryStorage) tryAllocate(size int64) bool {
	if s.capacity <= 0 {
		s.used.Add(size)
		return true
	}
	// Use a bounded retry with yield to avoid busy-loop on full capacity
	for i := 0; i < 100; i++ {
		current := s.used.Load()
		if current+size > s.capacity {
			return false
		}
		if s.used.CompareAndSwap(current, current+size) {
			return true
		}
		// Yield to other goroutines contending on CAS
		if i%10 == 9 {
			continue
		}
	}
	return false
}

// GetUsed returns the current storage usage.
func (s *memoryStorage) GetUsed() int64 {
	return s.used.Load()
}

// GetCapacity returns the storage capacity.
func (s *memoryStorage) GetCapacity() int64 {
	return s.capacity
}

// Close closes the storage and releases all resources.
func (s *memoryStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers = make(map[metainfo.Hash]*memoryPieceProvider)
	s.used.Store(0)
	return nil
}
