// SPDX-License-Identifier: MIT

// Package storage provides torrent data storage backends.
// In-memory storage keeps everything in RAM; file storage persists pieces to
// disk under a base directory (one sub-directory per torrent, named by its
// info-hash). File storage is optional and only used when explicitly enabled
// via configuration, leaving the in-memory path as the secure-by-default.
package storage

import (
	"github.com/anacrolix/torrent/storage"
)

// ClientImplCloser is re-exported from the underlying anacrolix storage so
// other packages can reference the closable storage interface without
// importing anacrolix directly.
type ClientImplCloser = storage.ClientImplCloser

// NewFileStorage creates a disk-backed storage that writes torrent pieces to
// sub-directories of baseDir. It returns a ClientImplCloser; callers must
// invoke Close to release resources.
func NewFileStorage(baseDir string) storage.ClientImplCloser {
	return storage.NewFile(baseDir)
}
