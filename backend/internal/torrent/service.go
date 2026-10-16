// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package torrent provides the torrent service.
// Manages adding, removing and streaming torrents via anacrolix/torrent.
// Uses structured logging with operation context.
package torrent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"path"

	"strings"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/persistence"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/storage"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// Torrent service constants
const (
	gracefulShutdownTimeout = constants.TorrentGracefulShutdownTimeout
	magnetMetadataTimeout   = constants.MagnetMetadataTimeout
	maxTorrents             = constants.MaxTorrents
	maxStreamFileSize       = constants.MaxStreamFileSize
	maxTorrentFileSize      = constants.MaxTorrentFileSize
	// restoreTimeout bounds one registry entry during startup restoration.
	// The backend blocks until metadata resolves, so a peer that has gone
	// offline since the last run would otherwise hold startup open; the bound
	// is longer than magnetMetadataTimeout to leave room for the re-announce
	// a fresh peer needs.
	restoreTimeout = constants.MagnetMetadataTimeout * 2
)

// Service torrent management service.
// Provides methods for adding, removing and streaming torrents.
// Thread-safe thanks to sync.RWMutex.
type Service struct {
	mu       sync.RWMutex
	backend  TorrentBackend
	torrents map[string]TorrentHandle
	// selectedFiles maps torrentID -> userID -> fileIndex (per-peer selection, bug-0048)
	selectedFiles map[string]map[string]int
	// torrentOwners maps torrentID -> the userID that added it (bug-0049).
	// Removal is refused for anyone else, so a guest sharing the process-wide
	// access token cannot delete another peer's data.
	torrentOwners map[string]string
	// visibility answers whether a peer's room grants it access to a torrent.
	// nil disables room scoping (single-user setups and tests); see canRead.
	visibility TorrentVisibility
	// persistence persists torrent ownership (bug-0051).
	persistence *persistence.Store
	// torrentSources remembers how each torrent was added, keyed by torrent
	// ID. Restoration replays the original source rather than rebuilding the
	// torrent from a summary of it: the infohash is a hash of the metadata,
	// so anything reconstructed hashes differently and comes back as a
	// different torrent (bug-0053).
	torrentSources map[string]torrentSource
	bufferService  *buffer.Service
	storage        storage.ClientImplCloser // non-nil only for disk-backed storage
	streamWG       sync.WaitGroup           // tracks active ServeFile streams
}

// TorrentVisibility answers which torrents a peer may read through the room it
// participates in. Implemented by p2p.Service (bug-0049).
type TorrentVisibility interface {
	TorrentsVisibleTo(ctx context.Context, userID string) map[string]bool
}

// SetVisibility wires the room-scoped access check. Without it the service
// falls back to owner-only, which is correct for a machine that is not shared.
func (s *Service) SetVisibility(v TorrentVisibility) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visibility = v
}

// SetPersistence wires the torrent ownership persistence (bug-0051) and
// torrent registry persistence (bug-0038). Both are loaded immediately so a
// restart restores who added each torrent and re-adds all known torrents.
func (s *Service) SetPersistence(store *persistence.Store) {
	s.mu.Lock()
	s.persistence = store
	s.mu.Unlock()

	if store == nil {
		return
	}

	// Load ownership (bug-0049)
	ownData, err := store.LoadTorrents()
	if err != nil {
		logger.Warn("Torrent: failed to load ownership", "error", err)
	} else {
		s.mu.Lock()
		for tid, uid := range ownData.Owners {
			s.torrentOwners[tid] = uid
		}
		s.mu.Unlock()
		logger.Info("Torrent: restored ownership from persistence", "count", len(ownData.Owners))
	}

	// Load torrent registry and re-add torrents (bug-0038)
	regData, err := store.LoadTorrentRegistry()
	if err != nil {
		logger.Warn("Torrent: failed to load torrent registry", "error", err)
		return
	}

	if len(regData.Torrents) == 0 {
		return
	}

	// Restoration blocks until each backend resolves metadata, so it runs on a
	// bounded context: a peer that went offline since the last run must not be
	// able to hold startup open indefinitely.
	logger.Info("Torrent: restoring torrents from registry", "count", len(regData.Torrents))

	var restored int
	for _, info := range regData.Torrents {
		ctx, cancel := context.WithTimeout(context.Background(), restoreTimeout)
		handle, err := s.readdTorrentFromRegistry(ctx, info)
		cancel()
		if err != nil {
			logger.Warn("Torrent: failed to re-add torrent from registry",
				"infoHash", info.InfoHash, "error", err)
			continue
		}

		// The restored handle must land on the infohash the registry was keyed
		// by. If it does not, the entry describes a torrent that no longer
		// exists and restoring it would only create an orphan.
		tid := handle.ID()
		if tid != info.InfoHash {
			logger.Warn("Torrent: registry restore produced a different infohash, dropping entry",
				"expected", info.InfoHash, "got", tid)
			s.backend.Drop(tid)
			continue
		}

		s.mu.Lock()
		s.torrents[tid] = handle
		s.mu.Unlock()

		restored++
		logger.Info("Torrent: restored from registry", "torrentID", tid, "name", info.Name)
	}

	logger.Info("Torrent: registry restore complete", "restored", restored, "of", len(regData.Torrents))
}

// torrentSource is the exact input a torrent was added from. Exactly one of
// the two fields is set: a magnet carries its identity in the URI, an upload in
// the bytes. Re-adding from the source reproduces the original infohash by
// construction, which rebuilding the metadata would not.
type torrentSource struct {
	magnetURI string
	torrent   []byte
}

// readdTorrentFromRegistry re-adds a persisted torrent from the source it was
// originally added from.
//
// This used to rebuild a .torrent from the stored name, size and file list,
// with an empty "pieces" field and a hardcoded piece length. That produces a
// torrent with a *different* infohash, so after a restart the torrent was no
// longer the one rooms are bound to and the ownership map no longer described
// it — a silent loss of exactly the guarantees bug-0047 and bug-0049 added
// (bug-0053).
func (s *Service) readdTorrentFromRegistry(ctx context.Context, info persistence.TorrentInfo) (TorrentHandle, error) {
	switch {
	case info.MagnetURI != "":
		return s.backend.AddMagnet(ctx, info.MagnetURI)
	case len(info.TorrentFile) > 0:
		return s.backend.AddTorrentFile(ctx, bytes.NewReader(info.TorrentFile))
	default:
		// A registry written before bug-0053 carries no source. Re-adding it
		// would mean inventing an identity, and an invented identity is worse
		// than an absent torrent: it looks restored while being unbound from
		// every room that referenced it.
		return nil, errNoRestorableSource
	}
}

// errNoRestorableSource marks a registry entry that predates source-based
// restoration and therefore cannot be re-added.
var errNoRestorableSource = errors.New(errors.ErrInvalidInput, "registry entry has no restorable source")

// SaveRegistry persists both ownership (bug-0049) and torrent registry (bug-0038) to disk.
func (s *Service) SaveRegistry() error {
	if s.persistence == nil {
		return nil
	}

	// Ownership (bug-0049)
	s.mu.RLock()
	owners := make(map[string]string, len(s.torrentOwners))
	for tid, uid := range s.torrentOwners {
		owners[tid] = uid
	}

	// Torrent registry (bug-0038)
	torrents := make(map[string]persistence.TorrentInfo, len(s.torrents))
	for tid, handle := range s.torrents {
		files := make([]persistence.FileInfo, handle.FileCount())
		for i := 0; i < handle.FileCount(); i++ {
			files[i] = persistence.FileInfo{
				Index: i,
				Name:  handle.FileName(i),
				Size:  handle.FileLength(i),
			}
		}
		source := s.torrentSources[tid]
		torrents[tid] = persistence.TorrentInfo{
			InfoHash:    tid,
			MagnetURI:   source.magnetURI,
			TorrentFile: source.torrent,
			Name:        handle.Name(),
			Size:        handle.TotalLength(),
			FileCount:   handle.FileCount(),
			Files:       files,
		}
	}
	s.mu.RUnlock()

	// Save ownership
	ownData := &persistence.TorrentOwnershipData{Owners: owners}
	if err := s.persistence.SaveTorrents(ownData); err != nil {
		logger.Warn("Torrent: failed to persist ownership", "error", err)
		return err
	}

	// Save registry
	regData := &persistence.TorrentRegistryData{Torrents: torrents}
	if err := s.persistence.SaveTorrentRegistry(regData); err != nil {
		logger.Warn("Torrent: failed to persist torrent registry", "error", err)
		return err
	}

	return nil
}

// canRead reports whether userID may read torrentID.
//
// The access token is per-process: "presents the token" cannot distinguish a
// room guest from the machine's owner. Reading therefore needs one of two
// provenances — the peer added the torrent itself, or the torrent is bound to
// the room the peer is currently in (bug-0047).
//
// An unowned torrent is readable by anyone. It can only come from persistence
// written before ownership was tracked, and refusing would make such torrents
// permanently unreadable — the same trade-off as RemoveTorrent, for the same
// reason.
//
// Both the ownership map and the visibility provider are read WITHOUT s.mu
// held across the call: the visibility provider takes its own lock, and nesting
// the two would invite a lock-order deadlock with any path that goes the other
// way.
func (s *Service) canRead(ctx context.Context, userID, torrentID string) bool {
	s.mu.RLock()
	owner, hasOwner := s.torrentOwners[torrentID]
	visibility := s.visibility
	s.mu.RUnlock()

	if !hasOwner {
		return true
	}
	if owner == userID {
		return true
	}
	if visibility == nil {
		return false
	}
	return visibility.TorrentsVisibleTo(ctx, userID)[torrentID]
}

// CanRead is the public wrapper for canRead (implements TorrentService interface).
func (s *Service) CanRead(ctx context.Context, userID, torrentID string) bool {
	return s.canRead(ctx, userID, torrentID)
}

// MagnetFor returns a magnet URI the caller can add to its own backend.
//
// Every participant pulls the media from the swarm itself, so a guest that knows
// only a room's infohash has nothing to download — the infohash identifies a
// torrent without carrying one. This is how the torrent leaves.
//
// The magnet is produced from the source the torrent was added from, not
// rebuilt from its metadata: for a .torrent upload the magnet is derived from
// the real info dict, which is what makes the guest's copy the *same* torrent
// (bug-0053).
func (s *Service) MagnetFor(_ context.Context, userID, torrentID string) (string, error) {
	if !s.canRead(context.Background(), userID, torrentID) {
		return "", errors.New(errors.ErrForbidden, "access denied")
	}

	s.mu.RLock()
	source, exists := s.torrentSources[torrentID]
	s.mu.RUnlock()

	if !exists {
		return "", errors.New(errors.ErrNotFound, "torrent not found")
	}

	switch {
	case source.magnetURI != "":
		return source.magnetURI, nil
	case len(source.torrent) > 0:
		mi, err := metainfo.Load(bytes.NewReader(source.torrent))
		if err != nil {
			return "", fmt.Errorf("failed to read stored torrent: %w", err)
		}
		if _, err := mi.UnmarshalInfo(); err != nil {
			return "", fmt.Errorf("failed to read torrent info: %w", err)
		}
		// The hash is left to the MetaInfo: it derives the magnet from the very
		// info dict the hash was computed over, so there is no way for the two
		// to disagree.
		magnet, err := mi.MagnetV2()
		if err != nil {
			return "", fmt.Errorf("failed to derive magnet: %w", err)
		}
		return magnet.String(), nil
	default:
		// Added before sources were recorded, or restored from a registry
		// written before bug-0053. An invented identity would be worse than an
		// honest refusal.
		return "", errors.New(errors.ErrNotFound, "torrent has no restorable source")
	}
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

// NewServiceWithOptions creates a new torrent service with extended options.
// Allows configuring network parameters for testing. By default torrent
// pieces are kept in memory; set opts.Storage to StorageDisk (with a DataDir)
// to persist them to disk.
func NewServiceWithOptions(bufferService *buffer.Service, opts ServiceOptions) (*Service, error) {
	// Configure torrent client
	cfg := torrent.NewDefaultClientConfig()

	svc := &Service{
		torrents:       make(map[string]TorrentHandle),
		selectedFiles:  make(map[string]map[string]int),
		torrentOwners:  make(map[string]string),
		torrentSources: make(map[string]torrentSource),
		bufferService:  bufferService,
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
	// Torrent client's default listen port is a fixed 42069. Every participant
	// runs their own backend, so two instances on one machine is ordinary — and
	// the second one died with "address already in use" and took the whole
	// server down with it. Port 0 makes anacrolix pick a free port and share it
	// across TCP and uTP, which is what every other library here does by
	// default. An explicitly configured port still wins.
	if opts.ListenPort != 0 {
		cfg.ListenPort = opts.ListenPort
	} else {
		cfg.ListenPort = 0
	}

	backend, err := newLocalBackend(cfg)
	if err != nil {
		logger.Error("Torrent: failed to create torrent client", "error", err)
		return nil, fmt.Errorf("failed to create torrent client: %w", err)
	}

	svc.backend = backend
	logger.Info("Torrent: service initialized")

	return svc, nil
}

// NewServiceWithBackend creates a torrent service on top of a preconfigured
// backend. It is how remoting mode (idea-0023) swaps the local anacrolix client
// for a remote htorrent gateway without changing any behaviour above the seam.
func NewServiceWithBackend(bufferService *buffer.Service, backend TorrentBackend) (*Service, error) {
	if backend == nil {
		return nil, errors.New(errors.ErrInvalidInput, "backend is required")
	}
	return &Service{
		backend:        backend,
		torrents:       make(map[string]TorrentHandle),
		selectedFiles:  make(map[string]map[string]int),
		torrentOwners:  make(map[string]string),
		torrentSources: make(map[string]torrentSource),
		bufferService:  bufferService,
	}, nil
}

// AddMagnet adds a torrent via magnet link.
// Parameter ctx - context for operation cancellation.
// Parameter magnetURI - magnet link for the torrent.
// Waits for metadata reception (timeout via context).
// Returns torrent information or an error.
func (s *Service) AddMagnet(ctx context.Context, userID, magnetURI string) (*models.TorrentInfo, error) {
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

	// The backend blocks until metadata arrives or magnetMetadataTimeout
	// elapses, and drops the torrent itself on failure.
	handle, err := s.backend.AddMagnet(ctx, magnetURI)
	if err != nil {
		logger.Error("Torrent: failed to add torrent", "magnetHash", magnetHash, "error", err)
		return nil, fmt.Errorf("failed to add torrent: %w", err)
	}

	torrentID := handle.ID()

	s.mu.Lock()
	s.torrents[torrentID] = handle
	s.torrentOwners[torrentID] = userID
	// Remember the magnet so a restart can replay it verbatim (bug-0053).
	s.torrentSources[torrentID] = torrentSource{magnetURI: magnetURI}
	s.mu.Unlock()

	if err := s.SaveRegistry(); err != nil {
		logger.Warn("Torrent: failed to persist ownership", "error", err)
	}

	info := s.torrentToInfo(handle)

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
		"files", handle.FileCount(),
	)

	return info, nil
}

// AddTorrent adds a torrent from a torrent file (metadata).
// Parameter ctx - context for operation cancellation.
// Parameter torrentData - reader containing torrent file content (bencoded .torrent).
// Waits for metadata reception (timeout via context).
// Returns torrent information or an error.
func (s *Service) AddTorrent(ctx context.Context, userID string, torrentData io.Reader) (*models.TorrentInfo, error) {
	s.mu.RLock()
	overLimit := len(s.torrents) >= maxTorrents
	s.mu.RUnlock()
	if overLimit {
		logger.Warn("Torrent: torrent limit exceeded", "max", maxTorrents)
		return nil, errors.New(errors.ErrUnavailable, "maximum number of torrents exceeded")
	}

	logger.Info("Torrent: adding torrent from file")

	// Keep the original bytes so a restart can replay them verbatim: the
	// infohash is the hash of these bytes' info dict, and nothing shorter
	// identifies the torrent (bug-0053). The size is bounded because an
	// uploaded .torrent is metadata, not media.
	raw, err := io.ReadAll(io.LimitReader(torrentData, maxTorrentFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read torrent file: %w", err)
	}
	if int64(len(raw)) > maxTorrentFileSize {
		return nil, errors.New(errors.ErrInvalidInput, "torrent file too large")
	}

	// Parsing the bencoded metadata and waiting for it to resolve both belong
	// to the backend, so a gateway can accept the same call shape.
	handle, err := s.backend.AddTorrentFile(ctx, bytes.NewReader(raw))
	if err != nil {
		logger.Error("Torrent: failed to add torrent from file", "error", err)
		return nil, fmt.Errorf("failed to add torrent from file: %w", err)
	}

	torrentID := handle.ID()

	s.mu.Lock()
	s.torrents[torrentID] = handle
	s.torrentOwners[torrentID] = userID
	s.torrentSources[torrentID] = torrentSource{torrent: raw}
	s.mu.Unlock()

	if err := s.SaveRegistry(); err != nil {
		logger.Warn("Torrent: failed to persist ownership", "error", err)
	}

	info := s.torrentToInfo(handle)

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
		"files", handle.FileCount(),
	)

	return info, nil
}

// RemoveTorrent removes a torrent by ID.
// Stops the download and removes the torrent from the client.
// Waits for active ServeFile streams to finish before returning.
// Returns an error if the torrent is not found.
func (s *Service) RemoveTorrent(ctx context.Context, id, userID string) error {
	// First mark the torrent for removal under lock
	s.mu.Lock()
	handle, exists := s.torrents[id]
	if !exists {
		s.mu.Unlock()
		logger.Warn("Torrent: torrent not found for removal", "torrentID", id)
		return errors.NotFound("torrent", id)
	}

	// Ownership gate (bug-0049): the access token is per-process, so anyone
	// who reached this server shares its authority. Without this check a guest
	// could DELETE any torrent of the host — irreversible, silent loss of
	// someone else's data.
	//
	// An unknown owner means the torrent predates this bookkeeping (restored
	// from persistence). Removal stays permitted there: refusing would leave
	// such torrents undeletable, which is a worse failure than the risk.
	owner, hasOwner := s.torrentOwners[id]
	if hasOwner && owner != userID {
		s.mu.Unlock()
		logger.Warn("Torrent: removal refused, not the owner",
			"torrentID", id, "owner", owner, "requester", userID)
		return errors.New(errors.ErrForbidden, "only the peer that added a torrent may remove it")
	}

	torrentName := handle.Name()

	// Remove from maps immediately so new streams can't start
	delete(s.torrents, id)
	delete(s.selectedFiles, id)
	delete(s.torrentOwners, id)
	delete(s.torrentSources, id)
	s.mu.Unlock()

	if err := s.SaveRegistry(); err != nil {
		logger.Warn("Torrent: failed to persist ownership after removal", "error", err)
	}

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

	s.backend.Drop(id)

	// Reclaim on-disk piece data for disk-backed storage to avoid leaks. The name,
	// not the id: anacrolix writes <dataDir>/<Info.BestName()>, so removing
	// <dataDir>/<id> removed nothing and said so (bug-0077).
	if s.storage != nil {
		if fs, ok := s.storage.(*storage.FileStorage); ok {
			if err := fs.RemoveTorrent(torrentName); err != nil {
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

// ListTorrents returns the torrents userID may read: the ones it added plus
// those bound to its current room (bug-0049).
//
// Filtering happens here rather than in the handler because the ownership map
// lives on the Service; the handler could not compute "mine" on its own.
func (s *Service) ListTorrents(ctx context.Context, userID string) []*models.TorrentInfo {
	s.mu.RLock()
	handles := make(map[string]TorrentHandle, len(s.torrents))
	for id, t := range s.torrents {
		handles[id] = t
	}
	s.mu.RUnlock()

	result := make([]*models.TorrentInfo, 0, len(handles))
	for id, t := range handles {
		if !s.canRead(ctx, userID, id) {
			continue
		}
		result = append(result, s.torrentToInfo(t))
	}

	logger.Debug("Torrent: retrieved torrent list",
		"userID", userID, "total", len(handles), "visible", len(result))
	return result
}

// GetFiles returns a list of torrent files.
// Parameter torrentID - torrent identifier.
// Returns an array of files with indices, names, sizes and paths.
// Returns an error if the torrent is not found or metadata is not received.
func (s *Service) GetFiles(ctx context.Context, userID, torrentID string) ([]models.FileInfo, error) {
	// Access is checked before the read lock so the visibility provider is not
	// called while s.mu is held (see canRead).
	if !s.canRead(ctx, userID, torrentID) {
		logger.Warn("Torrent: file list refused, peer may not read this torrent",
			"torrentID", torrentID, "userID", userID)
		return nil, errors.New(errors.ErrForbidden, "access denied")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	handle, exists := s.torrents[torrentID]
	if !exists {
		logger.Warn("Torrent: torrent not found for getting files", "torrentID", torrentID)
		return nil, errors.NotFound("torrent", torrentID)
	}

	total := handle.FileCount()
	result := make([]models.FileInfo, 0, total)

	for i := 0; i < total; i++ {
		length := handle.FileLength(i)

		// Validate file size
		if err := validation.ValidateFileSize(length); err != nil {
			logger.Warn("Torrent: invalid file size",
				"torrentID", torrentID,
				"fileIndex", i,
				"error", err,
			)
		}

		result = append(result, models.FileInfo{
			Index: i,
			Name:  sanitizeFilename(handle.FileName(i)),
			Size:  length,
		})
	}

	logger.Debug("Torrent: retrieved file list", "torrentID", torrentID, "fileCount", len(result))
	return result, nil
}

// SelectFile selects a file for streaming for a specific user (per-peer, bug-0048).
// Parameter torrentID - torrent identifier.
// Parameter userID - caller's client ID (X-Client-ID).
// Parameter fileIndex - file index in the torrent.
// Sets download priority for the selected file for this user.
// Returns an error if the torrent is not found or index is invalid.
func (s *Service) SelectFile(ctx context.Context, torrentID, userID string, fileIndex int) error {
	// Room-scoped read access (bug-0049 / deviation-0039): only participants
	// in the room bound to this torrent may change its selection.
	if !s.canRead(ctx, userID, torrentID) {
		logger.Warn("Torrent: file selection refused, peer may not access this torrent",
			"torrentID", torrentID, "userID", userID)
		return errors.New(errors.ErrForbidden, "access denied")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	handle, exists := s.torrents[torrentID]
	if !exists {
		logger.Warn("Torrent: torrent not found for file selection", "torrentID", torrentID, "fileIndex", fileIndex, "userID", userID)
		return errors.NotFound("torrent", torrentID)
	}

	total := handle.FileCount()

	// Validate file index
	if err := validation.ValidateFileIndex(fileIndex, total-1); err != nil {
		logger.Warn("Torrent: invalid file index",
			"torrentID", torrentID,
			"userID", userID,
			"fileIndex", fileIndex,
			"totalFiles", total,
			"error", err,
		)
		return errors.Wrap(errors.ErrInvalidInput, "invalid file index", err)
	}

	// Narrow piece priority to the selected file (global per torrent, fine for anacrolix)
	if err := handle.SelectForStreaming(fileIndex); err != nil {
		logger.Warn("Torrent: failed to select file", "torrentID", torrentID, "userID", userID, "fileIndex", fileIndex, "error", err)
		return err
	}

	// Store per-user selection (bug-0048)
	if s.selectedFiles[torrentID] == nil {
		s.selectedFiles[torrentID] = make(map[string]int)
	}
	s.selectedFiles[torrentID][userID] = fileIndex

	// Register in buffer service. Only a backend that owns piece priorities can
	// be steered; a gateway does its own prioritisation and reports no target.
	if s.bufferService != nil {
		if file, ok := handle.BufferTarget(fileIndex).(*torrent.File); ok {
			s.bufferService.RegisterTorrent(
				torrentID,
				file,
				constants.DefaultBufferPercent,
				constants.DefaultBufferDuration,
				constants.DefaultMaxBufferSize,
			)
		}
	}

	logger.Info("Torrent: file selected for streaming",
		"torrentID", torrentID,
		"userID", userID,
		"fileIndex", fileIndex,
		"fileName", handle.FileName(fileIndex),
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
func (s *Service) GetBufferInfo(ctx context.Context, userID, torrentID string) (*models.BufferInfo, error) {
	// Same room-scoped rule as every other read (bug-0049): buffer state is
	// still a statement about someone else's torrent.
	if !s.canRead(ctx, userID, torrentID) {
		logger.Warn("Torrent: buffer info refused, peer may not read this torrent",
			"torrentID", torrentID, "userID", userID)
		return nil, errors.New(errors.ErrForbidden, "access denied")
	}

	if s.bufferService == nil {
		return nil, errors.New(errors.ErrUnavailable, "buffer service not initialized")
	}
	return s.bufferService.GetBufferInfo(ctx, torrentID)
}

// ServeFile handles HTTP streaming of a torrent file for a specific user.
// Supports Range requests for seeking via http.ServeContent.
// Automatically determines Content-Type from file extension.
// Returns 400 if file is not selected, 404 if torrent is not found.
//
// Uses streamWG to prevent TOCTOU races where a torrent might be removed
// between validation and actual streaming. RemoveTorrent waits for active streams.
// userID is the caller's client ID (X-Client-ID from request context).
func (s *Service) ServeFile(w http.ResponseWriter, r *http.Request, torrentID, userID string) {
	// Resolve the torrent and its selection under the read lock, and register
	// the stream in the same critical section. RemoveTorrent takes the write
	// lock before dropping, so it cannot observe a registered stream and miss
	// it, nor drop the torrent between validation and reading.
	// Room-scoped read access (bug-0049). Checked before the read lock so the
	// visibility provider is not called while s.mu is held.
	//
	// The answer is a flat 404, not 403: a guest who is not in the room should
	// not learn that the torrent exists at all.
	if !s.canRead(r.Context(), userID, torrentID) {
		logger.Warn("Torrent: stream refused, peer may not read this torrent",
			"torrentID", torrentID, "userID", userID)
		http.Error(w, "Torrent not found", http.StatusNotFound)
		return
	}

	s.mu.RLock()
	handle, exists := s.torrents[torrentID]
	if !exists {
		s.mu.RUnlock()
		logger.Warn("Torrent: torrent not found for streaming", "torrentID", torrentID, "userID", userID)
		http.Error(w, "Torrent not found", http.StatusNotFound)
		return
	}

	userSelection, hasTorrentSelection := s.selectedFiles[torrentID]
	if !hasTorrentSelection {
		s.mu.RUnlock()
		logger.Warn("Torrent: no file selected for streaming", "torrentID", torrentID, "userID", userID)
		http.Error(w, "File not selected for streaming", http.StatusBadRequest)
		return
	}

	fileIndex, hasSelection := userSelection[userID]
	if !hasSelection {
		s.mu.RUnlock()
		logger.Warn("Torrent: file not selected for streaming by this user", "torrentID", torrentID, "userID", userID)
		http.Error(w, "File not selected for streaming", http.StatusBadRequest)
		return
	}

	if fileIndex >= handle.FileCount() {
		total := handle.FileCount()
		s.mu.RUnlock()
		logger.Warn("Torrent: invalid file index during streaming",
			"torrentID", torrentID,
			"userID", userID,
			"fileIndex", fileIndex,
			"totalFiles", total,
		)
		http.Error(w, "Invalid file index", http.StatusBadRequest)
		return
	}

	s.streamWG.Add(1)
	s.mu.RUnlock()

	defer s.streamWG.Done()

	s.backend.ServeStream(w, r, handle, fileIndex)
}

// ServeSubtitles serves any file of a torrent by index — the subtitle track
// lives next to the film rather than in it, so unlike ServeFile it needs no
// selection: a peer that never selected anything still reads the subtitles
// of what the room watches. Every other gate is ServeFile's: the torrent
// must exist and be readable (flat 404 otherwise), the index must name a
// file. mpv fetches the answer over HTTP with the same access-token header,
// so no ticket machinery is involved.
func (s *Service) ServeSubtitles(w http.ResponseWriter, r *http.Request, torrentID, userID string, fileIndex int) {
	if !s.canRead(r.Context(), userID, torrentID) {
		logger.Warn("Torrent: subtitles refused, peer may not read this torrent",
			"torrentID", torrentID, "userID", userID)
		http.Error(w, "Torrent not found", http.StatusNotFound)
		return
	}

	s.mu.RLock()
	handle, exists := s.torrents[torrentID]
	if !exists {
		s.mu.RUnlock()
		logger.Warn("Torrent: torrent not found for subtitles", "torrentID", torrentID, "userID", userID)
		http.Error(w, "Torrent not found", http.StatusNotFound)
		return
	}

	if fileIndex < 0 || fileIndex >= handle.FileCount() {
		total := handle.FileCount()
		s.mu.RUnlock()
		logger.Warn("Torrent: invalid file index for subtitles",
			"torrentID", torrentID,
			"userID", userID,
			"fileIndex", fileIndex,
			"totalFiles", total,
		)
		http.Error(w, "Invalid file index", http.StatusBadRequest)
		return
	}

	s.streamWG.Add(1)
	s.mu.RUnlock()

	defer s.streamWG.Done()

	s.backend.ServeStream(w, r, handle, fileIndex)
}

// Close closes the torrent service with graceful shutdown.
// Stops the torrent client and releases resources.
// Uses a context with timeout for waiting on active downloads.
// After calling Close, the service cannot be used.
func (s *Service) Close() error {
	// Flush before taking the lock. SaveRegistry reads the state under
	// s.mu.RLock, so calling it with s.mu already held deadlocks: RWMutex is
	// not reentrant and this is the third time this project has tripped over
	// it (bug-0052). Snapshot-then-save also means the write cannot interleave
	// with a concurrent add and persist a half-updated map.
	if err := s.SaveRegistry(); err != nil {
		logger.Warn("Torrent: failed to persist ownership on close", "error", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.storage != nil {
		if err := s.storage.Close(); err != nil {
			logger.Warn("Torrent: error closing storage", "error", err)
		}
		s.storage = nil
	}

	if s.backend != nil {
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
			s.backend.Close()
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

// torrentToInfo renders a handle for the torrent list. Metadata is always
// resolved by the time a handle exists, so there is no separate "loading"
// branch to guard here.
func (s *Service) torrentToInfo(handle TorrentHandle) *models.TorrentInfo {
	size := handle.TotalLength()

	info := &models.TorrentInfo{
		ID:    handle.ID(),
		Name:  handle.Name(),
		Size:  size,
		Peers: handle.ActivePeers(),
	}

	if size == 0 {
		info.Progress = 0
	} else {
		info.Progress = float64(handle.BytesCompleted()) / float64(size)
	}

	switch {
	case handle.Complete():
		info.Status = "seeding"
	case handle.BytesCompleted() > 0:
		info.Status = "downloading"
	default:
		info.Status = "loading"
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
