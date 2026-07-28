// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package torrent provides the torrent service.
// Manages adding, removing and streaming torrents via anacrolix/torrent.
// Uses structured logging with operation context.
package torrent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/storage"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// Torrent service constants
const (
	gracefulShutdownTimeout = constants.TorrentGracefulShutdownTimeout
	maxTorrents             = constants.MaxTorrents
	maxStreamFileSize       = constants.MaxStreamFileSize
	maxTorrentFileSize      = constants.MaxTorrentFileSize
)

// Service torrent management service.
// Provides methods for adding, removing and streaming torrents.
// Thread-safe thanks to sync.RWMutex.
type Service struct {
	mu            sync.RWMutex
	client        *torrent.Client
	torrents      map[string]*torrent.Torrent
	selectedFiles map[string]int // torrentID -> fileIndex
	bufferService *buffer.Service
	storage       storage.ClientImplCloser // non-nil only for disk-backed storage
	streamWG      sync.WaitGroup           // tracks active ServeFile streams
}

// StorageType identifies the torrent piece storage backend.
type StorageType string

const (
	// StorageMemory keeps all torrent pieces in RAM (secure-by-default).
	StorageMemory StorageType = "memory"
	// StorageDisk persists torrent pieces to disk under DataDir.
	StorageDisk StorageType = "disk"
)

// ServiceOptions contains options for configuring the torrent service
type ServiceOptions struct {
	// NoDHT disables DHT (for tests)
	NoDHT bool
	// DisableUTP disables UTP (for tests)
	DisableUTP bool
	// DisableTCP disables TCP (for tests)
	DisableTCP bool
	// ListenPort listen port (0 = random)
	ListenPort int
	// MemoryStorageCapacity maximum in-memory storage size (0 = unlimited)
	MemoryStorageCapacity int64
	// Storage selects the piece storage backend. Defaults to StorageMemory.
	Storage StorageType
	// DataDir base directory for disk-backed storage (ignored unless Storage == StorageDisk).
	DataDir string
}

// readCloserWithClose wraps io.ReadSeekCloser to guarantee idempotent Close.
// Prevents double-close issues when the reader is passed to http.ServeContent
// (which may call Close) and also closed via defer in ServeFile.
type readCloserWithClose struct {
	io.ReadSeekCloser
	closed bool
	mu     sync.Mutex
}

// Close closes the reader only once (idempotent close)
func (r *readCloserWithClose) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.ReadSeekCloser.Close()
}

// NewService creates a new torrent service with in-memory storage.
// Parameter bufferService - buffer service (can be nil).
// Returns an initialized service or an error.
func NewService(bufferService *buffer.Service) (*Service, error) {
	return NewServiceWithOptions(bufferService, ServiceOptions{})
}

// NewServiceWithOptions creates a new torrent service with extended options.
// Allows configuring network parameters for testing. By default torrent
// pieces are kept in memory; set opts.Storage to StorageDisk (with a DataDir)
// to persist them to disk.
func NewServiceWithOptions(bufferService *buffer.Service, opts ServiceOptions) (*Service, error) {
	// Configure torrent client
	cfg := torrent.NewDefaultClientConfig()

	svc := &Service{
		torrents:      make(map[string]*torrent.Torrent),
		selectedFiles: make(map[string]int),
		bufferService: bufferService,
	}

	switch opts.Storage {
	case StorageDisk:
		if opts.DataDir == "" {
			return nil, fmt.Errorf("disk storage requested but DataDir is empty")
		}
		fileStorage := storage.NewFileStorage(opts.DataDir)
		cfg.DefaultStorage = fileStorage
		svc.storage = fileStorage
		logger.Info("Torrent: using disk-backed storage", "dataDir", opts.DataDir)
	default:
		// Secure-by-default: in-memory storage.
		cfg.DefaultStorage = storage.NewMemoryStorage(opts.MemoryStorageCapacity)
		logger.Info("Torrent: using in-memory storage", "capacity", opts.MemoryStorageCapacity)
	}

	cfg.NoUpload = false
	cfg.Seed = true

	// Apply testing options
	if opts.NoDHT {
		cfg.NoDHT = true
	}
	if opts.DisableUTP {
		cfg.DisableUTP = true
	}
	if opts.DisableTCP {
		cfg.DisableTCP = true
	}
	if opts.ListenPort != 0 {
		cfg.ListenPort = opts.ListenPort
	}

	client, err := torrent.NewClient(cfg)
	if err != nil {
		logger.Error("Torrent: failed to create torrent client", "error", err)
		return nil, fmt.Errorf("failed to create torrent client: %w", err)
	}

	svc.client = client
	logger.Info("Torrent: service initialized")

	return svc, nil
}

// AddMagnet adds a torrent via magnet link.
// Parameter ctx - context for operation cancellation.
// Parameter magnetURI - magnet link for the torrent.
// Waits for metadata reception (timeout via context).
// Returns torrent information or an error.
func (s *Service) AddMagnet(ctx context.Context, magnetURI string) (*models.TorrentInfo, error) {
	// Validate magnet URI
	if err := validation.ValidateMagnetURI(magnetURI); err != nil {
		logger.Warn("Torrent: invalid magnet URI", "error", err)
		return nil, fmt.Errorf("invalid magnet URI: %w", err)
	}

	// Log only hash for security (not the full magnet link)
	hash := sha256.Sum256([]byte(magnetURI))
	magnetHash := fmt.Sprintf("%x", hash[:8])

	s.mu.RLock()
	overLimit := len(s.torrents) >= maxTorrents
	s.mu.RUnlock()
	if overLimit {
		logger.Warn("Torrent: torrent limit exceeded", "max", maxTorrents)
		return nil, errors.New(errors.ErrUnavailable, "maximum number of torrents exceeded")
	}

	logger.Info("Torrent: adding torrent", "magnetHash", magnetHash)

	t, err := s.client.AddMagnet(magnetURI)
	if err != nil {
		logger.Error("Torrent: failed to add torrent", "magnetHash", magnetHash, "error", err)
		return nil, fmt.Errorf("failed to add torrent: %w", err)
	}

	// Wait for metadata reception
	select {
	case <-t.GotInfo():
		logger.Debug("Torrent: metadata received", "magnetHash", magnetHash)
	case <-ctx.Done():
		t.Drop()
		logger.Warn("Torrent: timeout waiting for metadata", "magnetHash", magnetHash, "error", ctx.Err())
		return nil, fmt.Errorf("timeout waiting for metadata: %w", ctx.Err())
	}

	torrentID := t.InfoHash().HexString()

	s.mu.Lock()
	s.torrents[torrentID] = t
	s.mu.Unlock()

	info := s.torrentToInfo(t)

	// Validate torrent name
	if err := validation.ValidateTorrentName(info.Name); err != nil {
		logger.Warn("Torrent: invalid torrent name", "torrentID", torrentID, "error", err)
		// Do not abort the operation, just log
	}

	// Validate file size
	if err := validation.ValidateFileSize(info.Size); err != nil {
		logger.Warn("Torrent: invalid torrent size", "torrentID", torrentID, "size", info.Size, "error", err)
	}

	logger.Info("Torrent: torrent added",
		"torrentID", torrentID,
		"name", info.Name,
		"size", info.Size,
		"files", len(t.Files()),
	)

	return info, nil
}

// AddTorrent adds a torrent from a torrent file (metadata).
// Parameter ctx - context for operation cancellation.
// Parameter torrentData - reader containing torrent file content (bencoded .torrent).
// Waits for metadata reception (timeout via context).
// Returns torrent information or an error.
func (s *Service) AddTorrent(ctx context.Context, torrentData io.Reader) (*models.TorrentInfo, error) {
	s.mu.RLock()
	overLimit := len(s.torrents) >= maxTorrents
	s.mu.RUnlock()
	if overLimit {
		logger.Warn("Torrent: torrent limit exceeded", "max", maxTorrents)
		return nil, errors.New(errors.ErrUnavailable, "maximum number of torrents exceeded")
	}

	logger.Info("Torrent: adding torrent from file")

	// Parse the torrent file using metainfo.Load
	mi, err := metainfo.Load(torrentData)
	if err != nil {
		logger.Error("Torrent: failed to parse torrent file", "error", err)
		return nil, fmt.Errorf("failed to parse torrent file: %w", err)
	}

	t, err := s.client.AddTorrent(mi)
	if err != nil {
		logger.Error("Torrent: failed to add torrent from file", "error", err)
		return nil, fmt.Errorf("failed to add torrent from file: %w", err)
	}

	// Wait for metadata reception
	select {
	case <-t.GotInfo():
		logger.Debug("Torrent: metadata received from file")
	case <-ctx.Done():
		t.Drop()
		logger.Warn("Torrent: timeout waiting for metadata", "error", ctx.Err())
		return nil, fmt.Errorf("timeout waiting for metadata: %w", ctx.Err())
	}

	torrentID := t.InfoHash().HexString()

	s.mu.Lock()
	s.torrents[torrentID] = t
	s.mu.Unlock()

	info := s.torrentToInfo(t)

	// Validate torrent name
	if err := validation.ValidateTorrentName(info.Name); err != nil {
		logger.Warn("Torrent: invalid torrent name", "torrentID", torrentID, "error", err)
		// Do not abort the operation, just log
	}

	// Validate file size
	if err := validation.ValidateFileSize(info.Size); err != nil {
		logger.Warn("Torrent: invalid torrent size", "torrentID", torrentID, "size", info.Size, "error", err)
	}

	logger.Info("Torrent: torrent added from file",
		"torrentID", torrentID,
		"name", info.Name,
		"size", info.Size,
		"files", len(t.Files()),
	)

	return info, nil
}

// RemoveTorrent removes a torrent by ID.
// Stops the download and removes the torrent from the client.
// Waits for active ServeFile streams to finish before returning.
// Returns an error if the torrent is not found.
func (s *Service) RemoveTorrent(ctx context.Context, id string) error {
	// First mark the torrent for removal under lock
	s.mu.Lock()
	t, exists := s.torrents[id]
	if !exists {
		s.mu.Unlock()
		logger.Warn("Torrent: torrent not found for removal", "torrentID", id)
		return errors.NotFound("torrent", id)
	}

	torrentName := ""
	if t.Info() != nil {
		torrentName = t.Info().BestName()
	}

	// Remove from maps immediately so new streams can't start
	delete(s.torrents, id)
	delete(s.selectedFiles, id)
	s.mu.Unlock()

	// Wait for any active streams to finish
	done := make(chan struct{})
	go func() {
		s.streamWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		logger.Warn("Torrent: timeout waiting for active streams to finish", "torrentID", id)
	}

	t.Drop()

	// Reclaim on-disk piece data for disk-backed storage to avoid leaks.
	if s.storage != nil {
		if fs, ok := s.storage.(*storage.FileStorage); ok {
			if err := fs.RemoveTorrent(id); err != nil {
				logger.Warn("Torrent: failed to remove disk storage for torrent", "torrentID", id, "error", err)
			}
		}
	}

	// Remove from buffer service
	if s.bufferService != nil {
		s.bufferService.UnregisterTorrent(id)
	}

	logger.Info("Torrent: torrent removed", "torrentID", id, "name", torrentName)
	return nil
}

// ListTorrents returns a list of all torrents.
// Returns an array with information about each torrent (ID, name, progress, status).
func (s *Service) ListTorrents(_ context.Context) []*models.TorrentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.TorrentInfo, 0, len(s.torrents))
	for _, t := range s.torrents {
		result = append(result, s.torrentToInfo(t))
	}

	logger.Debug("Torrent: retrieved torrent list", "count", len(result))
	return result
}

// GetFiles returns a list of torrent files.
// Parameter torrentID - torrent identifier.
// Returns an array of files with indices, names, sizes and paths.
// Returns an error if the torrent is not found or metadata is not received.
func (s *Service) GetFiles(ctx context.Context, torrentID string) ([]models.FileInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, exists := s.torrents[torrentID]
	if !exists {
		logger.Warn("Torrent: torrent not found for getting files", "torrentID", torrentID)
		return nil, errors.NotFound("torrent", torrentID)
	}

	if t.Info() == nil {
		logger.Warn("Torrent: torrent metadata not yet received", "torrentID", torrentID)
		return nil, errors.New(errors.ErrUnavailable, "torrent metadata not yet received")
	}

	files := t.Files()
	result := make([]models.FileInfo, 0, len(files))

	for i, f := range files {
		// Validate file size
		if err := validation.ValidateFileSize(f.Length()); err != nil {
			logger.Warn("Torrent: invalid file size",
				"torrentID", torrentID,
				"fileIndex", i,
				"error", err,
			)
		}

		result = append(result, models.FileInfo{
			Index: i,
			Name:  sanitizeFilename(f.DisplayPath()),
			Size:  f.Length(),
		})
	}

	logger.Debug("Torrent: retrieved file list", "torrentID", torrentID, "fileCount", len(result))
	return result, nil
}

// SelectFile selects a file for streaming.
// Parameter torrentID - torrent identifier.
// Parameter fileIndex - file index in the torrent.
// Sets download priority for the selected file.
// Returns an error if the torrent is not found or index is invalid.
func (s *Service) SelectFile(ctx context.Context, torrentID string, fileIndex int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.torrents[torrentID]
	if !exists {
		logger.Warn("Torrent: torrent not found for file selection", "torrentID", torrentID, "fileIndex", fileIndex)
		return errors.NotFound("torrent", torrentID)
	}

	files := t.Files()

	// Validate file index
	if err := validation.ValidateFileIndex(fileIndex, len(files)-1); err != nil {
		logger.Warn("Torrent: invalid file index",
			"torrentID", torrentID,
			"fileIndex", fileIndex,
			"totalFiles", len(files),
			"error", err,
		)
		return errors.Wrap(errors.ErrInvalidInput, "invalid file index", err)
	}

	// Cancel download of all files
	for _, f := range files {
		f.SetPriority(torrent.PiecePriorityNone)
	}

	// Select the desired file
	files[fileIndex].SetPriority(torrent.PiecePriorityNormal)
	s.selectedFiles[torrentID] = fileIndex

	// Register in buffer service
	if s.bufferService != nil {
		s.bufferService.RegisterTorrent(
			torrentID,
			files[fileIndex],
			constants.DefaultBufferPercent,
			constants.DefaultBufferDuration,
			constants.DefaultMaxBufferSize,
		)
	}

	logger.Info("Torrent: file selected for streaming",
		"torrentID", torrentID,
		"fileIndex", fileIndex,
		"fileName", files[fileIndex].DisplayPath(),
	)
	return nil
}

// UpdateBufferPosition updates the current playback position for buffering.
// Parameter torrentID - torrent identifier.
// Parameter position - position in bytes.
// Returns an error if the buffer service fails.
func (s *Service) UpdateBufferPosition(ctx context.Context, torrentID string, position int64) error {
	if s.bufferService != nil {
		return s.bufferService.UpdatePosition(ctx, torrentID, position)
	}
	return nil
}

// GetBufferInfo returns buffer state information.
// Parameter torrentID - torrent identifier.
// Returns buffer information or an error.
func (s *Service) GetBufferInfo(ctx context.Context, torrentID string) (*models.BufferInfo, error) {
	if s.bufferService == nil {
		return nil, errors.New(errors.ErrUnavailable, "buffer service not initialized")
	}
	return s.bufferService.GetBufferInfo(ctx, torrentID)
}

// ServeFile handles HTTP streaming of a torrent file.
// Supports Range requests for seeking via http.ServeContent.
// Automatically determines Content-Type from file extension.
// Returns 400 if file is not selected, 404 if torrent is not found.
//
// Uses streamWG to prevent TOCTOU races where a torrent might be removed
// between validation and actual streaming. RemoveTorrent waits for active streams.
func (s *Service) ServeFile(w http.ResponseWriter, r *http.Request, torrentID string) {
	s.mu.RLock()
	t, exists := s.torrents[torrentID]
	if !exists {
		s.mu.RUnlock()
		logger.Warn("Torrent: torrent not found for streaming", "torrentID", torrentID)
		http.Error(w, "Torrent not found", http.StatusNotFound)
		return
	}

	fileIndex, hasSelection := s.selectedFiles[torrentID]
	if !hasSelection {
		s.mu.RUnlock()
		logger.Warn("Torrent: file not selected for streaming", "torrentID", torrentID)
		http.Error(w, "File not selected for streaming", http.StatusBadRequest)
		return
	}

	// Get file list while lock is active
	files := t.Files()
	if fileIndex >= len(files) {
		s.mu.RUnlock()
		logger.Warn("Torrent: invalid file index during streaming",
			"torrentID", torrentID,
			"fileIndex", fileIndex,
			"totalFiles", len(files),
		)
		http.Error(w, "Invalid file index", http.StatusBadRequest)
		return
	}

	file := files[fileIndex]

	if file.Length() > maxStreamFileSize {
		s.mu.RUnlock()
		logger.Warn("Torrent: file exceeds maximum streaming size",
			"torrentID", torrentID,
			"fileSize", file.Length(),
			"maxSize", maxStreamFileSize,
		)
		http.Error(w, "File too large for streaming", http.StatusBadRequest)
		return
	}

	reader := file.NewReader()
	s.streamWG.Add(1)
	s.mu.RUnlock()

	closer := &readCloserWithClose{ReadSeekCloser: reader}
	var streamErr error
	defer func() {
		closeErr := closer.Close()
		s.streamWG.Done()
		if closeErr != nil && streamErr == nil {
			logger.Warn("Torrent: error closing stream reader",
				"torrentID", torrentID, "error", closeErr)
		}
	}()

	safeName := sanitizeFilename(file.DisplayPath())

	logger.Info("Torrent: starting stream",
		"torrentID", torrentID,
		"fileIndex", fileIndex,
		"fileName", safeName,
		"fileSize", file.Length(),
	)

	http.ServeContent(w, r, safeName, time.Now(), closer)
}

// Close closes the torrent service with graceful shutdown.
// Stops the torrent client and releases resources.
// Uses a context with timeout for waiting on active downloads.
// After calling Close, the service cannot be used.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.storage != nil {
		if err := s.storage.Close(); err != nil {
			logger.Warn("Torrent: error closing storage", "error", err)
		}
		s.storage = nil
	}

	if s.client != nil {
		// Create context with timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
		defer cancel()

		// Channel for shutdown signaling
		done := make(chan struct{})

		torrentCount := len(s.torrents)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("Torrent: close goroutine exited with panic", "error", r)
				}
			}()
			s.client.Close()
			close(done)
		}()

		// Wait for completion or timeout
		select {
		case <-done:
			logger.Info("Torrent: service stopped gracefully", "torrentCount", torrentCount)
		case <-ctx.Done():
			logger.Warn("Torrent: service stopped with timeout", "torrentCount", torrentCount)
		}
	}

	return nil
}

func (s *Service) torrentToInfo(t *torrent.Torrent) *models.TorrentInfo {
	info := &models.TorrentInfo{
		ID:     t.InfoHash().HexString(),
		Status: "loading",
	}

	if t.Info() != nil {
		info.Name = t.Info().BestName()
		info.Size = t.Info().TotalLength()
		if info.Size == 0 {
			info.Progress = 0
		} else {
			info.Progress = float64(t.BytesCompleted()) / float64(info.Size)
		}

		if t.Complete().Bool() {
			info.Status = "seeding"
		} else if t.BytesCompleted() > 0 {
			info.Status = "downloading"
		}
	}

	return info
}

// sanitizeFilename cleans a filename from potentially dangerous characters and paths.
// Prevents path traversal and CRLF injections.
func sanitizeFilename(name string) string {
	name = path.Base(name)
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\x00", "")
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	return name
}
