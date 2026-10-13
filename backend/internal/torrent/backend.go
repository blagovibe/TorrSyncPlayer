// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"io"
	"net/http"
)

// TorrentBackend is the pluggable torrent data plane.
//
// Two implementations exist (idea-0023):
//   - localBackend — in-process anacrolix/torrent client, the default
//   - remoteBackend — an htorrent gateway reached over HTTPS, for remoting
//
// Everything above this seam is transport-agnostic: the Service validates,
// records selection, tracks streams and reports progress without knowing which
// implementation is in use.
type TorrentBackend interface {
	// AddMagnet registers a magnet link and returns once its metadata is
	// known. It blocks until metadata arrives or ctx expires.
	AddMagnet(ctx context.Context, magnetURI string) (TorrentHandle, error)

	// AddTorrentFile registers a bencoded .torrent from r. Like AddMagnet it
	// waits for metadata before returning.
	AddTorrentFile(ctx context.Context, r io.Reader) (TorrentHandle, error)

	// Drop stops the torrent and releases its pieces. Called only after
	// active streams have drained.
	Drop(torrentID string)

	// ServeStream streams one file of one torrent to an HTTP client, honouring
	// Range requests and setting Content-Type from the file name. The Service
	// has already resolved the handle and validated the index; implementations
	// must write their own error responses.
	ServeStream(w http.ResponseWriter, r *http.Request, handle TorrentHandle, fileIndex int)

	// Close releases the backend. Must be safe to call more than once.
	Close()
}

// TorrentHandle is a transport-neutral view of one torrent.
//
// It exposes only what the Service actually consumes; the file-reading and
// piece-priority machinery stays behind SelectForStreaming and ServeStream so
// that a remote gateway never has to pretend to own a local piece store.
type TorrentHandle interface {
	// ID is the infohash as lowercase hex, stable across backends so that
	// room and playback state stay valid across a restart.
	ID() string

	// Name, TotalLength, BytesCompleted and Complete drive the torrent list.
	Name() string
	TotalLength() int64
	BytesCompleted() int64
	Complete() bool

	// FileCount, FileName and FileLength describe the contents. FileName is
	// the basename only — sanitised by the caller where it becomes a header.
	FileCount() int
	FileName(index int) string
	FileLength(index int) int64

	// SelectForStreaming narrows piece priority to a single file. A gateway
	// has no such control and treats this as a no-op.
	SelectForStreaming(fileIndex int) error

	// BufferTarget returns the piece-priority handle that buffer.Service needs
	// in order to steer the download window, or nil when this backend owns
	// priorities itself and the local buffer has nothing to manage.
	BufferTarget(fileIndex int) any
}
