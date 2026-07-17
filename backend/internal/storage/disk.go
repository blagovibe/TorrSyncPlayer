// SPDX-License-Identifier: MIT

// Package storage provides torrent data storage backends.
// In-memory storage keeps everything in RAM; file storage persists pieces to
// disk under a base directory (one sub-directory per torrent, named by its
// info-hash). File storage is optional and only used when explicitly enabled
// via configuration, leaving the in-memory path as the secure-by-default.
package storage

import (
	"os"
	"path/filepath"

	"github.com/anacrolix/torrent/storage"
)

// ClientImplCloser is re-exported from the underlying anacrolix storage so
// other packages can reference the closable storage interface without
// importing anacrolix directly.
type ClientImplCloser = storage.ClientImplCloser

// FileStorage is a disk-backed storage that tracks its base directory so it
// can remove per-torrent piece data on demand.
type FileStorage struct {
	storage.ClientImplCloser
	baseDir string
}

// NewFileStorage creates a disk-backed storage that writes torrent pieces to
// sub-directories of baseDir. It returns a *FileStorage; callers must invoke
// Close to release resources and may call RemoveTorrent to reclaim disk space.
func NewFileStorage(baseDir string) *FileStorage {
	return &FileStorage{
		ClientImplCloser: storage.NewFile(baseDir),
		baseDir:          baseDir,
	}
}

// RemoveTorrent deletes the on-disk piece directory for the given torrent
// info-hash, reclaiming space left behind after the torrent is dropped from
// the client. It is a no-op if the directory does not exist.
func (f *FileStorage) RemoveTorrent(infoHash string) error {
	dir := filepath.Join(f.baseDir, infoHash)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(dir)
}
