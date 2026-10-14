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

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"

	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
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

// RemoveTorrent deletes the on-disk data of the torrent with the given name.
//
// The name, not the infohash, and that distinction is the whole point. anacrolix
// lays a torrent down as <baseDir>/<Info.BestName()>/<File.BestPath()...> — flat,
// with no directory per infohash — so removing baseDir/<infohash> removed a path
// that is never created. os.RemoveAll on a missing path returns nil, so the
// removal reported success and the data stayed: the API answered 200, the
// torrent left the list, and the gigabytes remained on disk.
//
// The sentinel name means the torrent claims no directory of its own: anacrolix
// writes its files straight into the base directory, where they sit mixed with
// every other torrent's. Nothing here can be removed without taking those too,
// so nothing is.
func (f *FileStorage) RemoveTorrent(name string) error {
	if name == "" || name == metainfo.NoName {
		logger.Warn("Storage: torrent has no directory of its own, nothing to remove", "name", name)
		return nil
	}
	return os.RemoveAll(filepath.Join(f.baseDir, name))
}
