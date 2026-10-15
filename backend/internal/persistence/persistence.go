// SPDX-License-Identifier: MIT
package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// PersistenceVersion is the current format version for migration handling
const PersistenceVersion = 1

// Rooms and playback state are not persisted anymore: rooms meet over WebRTC
// and no server holds them (idea-0064). What survives a restart is the torrent
// registry below — whose torrent came from where, and who added it.

// TorrentOwnershipData persists torrent ownership (bug-0049).
// torrentID -> userID mapping so a restart knows who added each torrent.
type TorrentOwnershipData struct {
	Version int               `json:"version"`
	Owners  map[string]string `json:"owners"`
}

type Store struct {
	dir string
	mu  sync.RWMutex
}

func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}
	return &Store{dir: dataDir}, nil
}

// SaveTorrents persists torrent ownership atomically to torrents.json.
func (s *Store) SaveTorrents(data *TorrentOwnershipData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data.Version = PersistenceVersion

	path := filepath.Join(s.dir, "torrents.json")
	tmpPath := path + ".tmp"

	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal torrent ownership: %w", err)
	}

	if err := os.WriteFile(tmpPath, encoded, 0600); err != nil {
		return fmt.Errorf("failed to write torrents file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if rmErr := os.Remove(tmpPath); rmErr != nil {
			logger.Error("persistence: failed to remove temp file", "path", tmpPath, "error", rmErr)
		}
		return fmt.Errorf("failed to rename torrents file: %w", err)
	}

	return nil
}

// LoadTorrents loads torrent ownership from torrents.json, tolerating a missing
// or corrupted file by returning an empty collection.
func (s *Store) LoadTorrents() (*TorrentOwnershipData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.dir, "torrents.json")
	// #nosec G304 -- path is under the configured data directory
	data, err := os.ReadFile(path) //nolint:gosec // path is under data directory
	if err != nil {
		if os.IsNotExist(err) {
			return &TorrentOwnershipData{Version: PersistenceVersion, Owners: make(map[string]string)}, nil
		}
		return nil, fmt.Errorf("failed to read torrents file: %w", err)
	}

	var result TorrentOwnershipData
	if err := json.Unmarshal(data, &result); err != nil {
		logger.Warn("persistence: corrupted torrents.json, starting fresh", "error", err)
		return &TorrentOwnershipData{Version: PersistenceVersion, Owners: make(map[string]string)}, nil
	}

	if result.Version == 0 {
		result.Version = PersistenceVersion
	}
	if result.Owners == nil {
		result.Owners = make(map[string]string)
	}

	return &result, nil
}

// TorrentRegistryData persists the torrent registry (bug-0038).
// Maps infohash -> torrent metadata so the server can re-add torrents on restart.
type TorrentRegistryData struct {
	Version  int                    `json:"version"`
	Torrents map[string]TorrentInfo `json:"torrents"`
}

// TorrentInfo describes one torrent in the registry.
//
// MagnetURI and TorrentFile are the *source* the torrent was added from, and
// they are the only fields restoration can use. The name, size and file list
// below are a description of the torrent, not its identity: the infohash is the
// SHA-1 of the bencoded info dict, so a .torrent rebuilt from this summary —
// without the real piece hashes and the real piece length — hashes to something
// else entirely. Restoring such a torrent silently produced a different torrent
// under the same name, and every room bound to the original infohash lost its
// binding (bug-0053).
//
// Exactly one of MagnetURI / TorrentFile is set, matching how the torrent was
// added: a magnet carries its identity in the URI, an upload in the bytes.
type TorrentInfo struct {
	InfoHash  string `json:"infoHash"`
	MagnetURI string `json:"magnetUri,omitempty"`
	// TorrentFile is the original bencoded .torrent. encoding/json encodes
	// []byte as base64, so the JSON stays plain text.
	TorrentFile []byte     `json:"torrentFile,omitempty"`
	Name        string     `json:"name"`
	Size        int64      `json:"size"`
	FileCount   int        `json:"fileCount"`
	Files       []FileInfo `json:"files"`
}

type FileInfo struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
}

// SaveTorrentRegistry persists the torrent registry atomically to
// torrents_registry.json.
func (s *Store) SaveTorrentRegistry(data *TorrentRegistryData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data.Version = PersistenceVersion

	path := filepath.Join(s.dir, "torrents_registry.json")
	tmpPath := path + ".tmp"

	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal torrent registry: %w", err)
	}

	if err := os.WriteFile(tmpPath, encoded, 0600); err != nil {
		return fmt.Errorf("failed to write torrent registry file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if rmErr := os.Remove(tmpPath); rmErr != nil {
			logger.Error("persistence: failed to remove temp file", "path", tmpPath, "error", rmErr)
		}
		return fmt.Errorf("failed to rename torrent registry file: %w", err)
	}

	return nil
}

// LoadTorrentRegistry loads the torrent registry from torrents_registry.json,
// tolerating a missing or corrupted file by returning an empty collection.
func (s *Store) LoadTorrentRegistry() (*TorrentRegistryData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.dir, "torrents_registry.json")
	// #nosec G304 -- path is under the configured data directory
	data, err := os.ReadFile(path) //nolint:gosec // path is under data directory
	if err != nil {
		if os.IsNotExist(err) {
			return &TorrentRegistryData{Version: PersistenceVersion, Torrents: make(map[string]TorrentInfo)}, nil
		}
		return nil, fmt.Errorf("failed to read torrent registry file: %w", err)
	}

	var result TorrentRegistryData
	if err := json.Unmarshal(data, &result); err != nil {
		logger.Warn("persistence: corrupted torrents_registry.json, starting fresh", "error", err)
		return &TorrentRegistryData{Version: PersistenceVersion, Torrents: make(map[string]TorrentInfo)}, nil
	}

	if result.Version == 0 {
		result.Version = PersistenceVersion
	}
	if result.Torrents == nil {
		result.Torrents = make(map[string]TorrentInfo)
	}

	return &result, nil
}
