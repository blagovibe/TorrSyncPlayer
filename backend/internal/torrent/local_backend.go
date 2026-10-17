// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/validation"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// localBackend drives torrents with an in-process anacrolix client. It is the
// default, and owns every piece of torrent machinery that has no counterpart on
// a remote gateway: the piece store, piece priorities and file readers.
type localBackend struct {
	client *torrent.Client
}

func newLocalBackend(cfg *torrent.ClientConfig) (*localBackend, error) {
	c, err := torrent.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &localBackend{client: c}, nil
}

func (b *localBackend) AddMagnet(ctx context.Context, magnetURI string) (TorrentHandle, error) {
	t, err := b.client.AddMagnet(magnetURI)
	if err != nil {
		return nil, err
	}
	if err := awaitMetadata(ctx, t); err != nil {
		t.Drop()
		return nil, err
	}
	return &localHandle{t: t}, nil
}

func (b *localBackend) AddTorrentFile(ctx context.Context, r io.Reader) (TorrentHandle, error) {
	mi, err := metainfo.Load(r)
	if err != nil {
		return nil, err
	}
	t, err := b.client.AddTorrent(mi)
	if err != nil {
		return nil, err
	}
	if err := awaitMetadata(ctx, t); err != nil {
		t.Drop()
		return nil, err
	}
	return &localHandle{t: t}, nil
}

// awaitMetadata blocks until the torrent's metadata arrives or the deadline
// passes. The timeout is layered on top of the caller's context because callers
// commonly pass one without a deadline, and a metadata fetch that never
// finishes would otherwise hold the request open forever.
func awaitMetadata(ctx context.Context, t *torrent.Torrent) error {
	ctx, cancel := context.WithTimeout(ctx, magnetMetadataTimeout)
	defer cancel()

	select {
	case <-t.GotInfo():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Drop stops the torrent and releases its pieces. Callers must have already
// drained active streams.
//
// anacrolix offers no lookup-by-hash on the client in this version, so the
// torrent is found by scanning the live set. The set is bounded by maxTorrents,
// so this stays cheap and, unlike a parse-based lookup, cannot fail on a
// malformed id and leak the torrent.
func (b *localBackend) Drop(torrentID string) {
	for _, t := range b.client.Torrents() {
		if t.InfoHash().HexString() == torrentID {
			t.Drop()
			return
		}
	}
	logger.Warn("Torrent: no torrent to drop for id", "torrentID", torrentID)
}

func (b *localBackend) Close() {
	b.client.Close()
}

// ServeStream streams one file of a local torrent with Range support.
//
// The caller (Service) has already validated the handle and index and has
// registered the stream in its wait group, so this only owns the reader and the
// HTTP response.
func (b *localBackend) ServeStream(w http.ResponseWriter, r *http.Request, handle TorrentHandle, fileIndex int) {
	h, ok := handle.(*localHandle)
	if !ok {
		logger.Error("Torrent: local backend received a foreign handle")
		http.Error(w, "Invalid torrent handle", http.StatusInternalServerError)
		return
	}

	files := h.t.Files()
	if fileIndex < 0 || fileIndex >= len(files) {
		logger.Warn("Torrent: invalid file index during streaming",
			"torrentID", h.ID(), "fileIndex", fileIndex, "totalFiles", len(files))
		http.Error(w, "Invalid file index", http.StatusBadRequest)
		return
	}

	file := files[fileIndex]
	if file.Length() > maxStreamFileSize {
		logger.Warn("Torrent: file exceeds maximum streaming size",
			"torrentID", h.ID(), "fileSize", file.Length(), "maxSize", maxStreamFileSize)
		http.Error(w, "File too large for streaming", http.StatusBadRequest)
		return
	}

	reader := file.NewReader()

	// Close exactly once: http.ServeContent may close the reader itself.
	closer := &readCloserWithClose{ReadSeekCloser: reader}
	defer func() {
		if err := closer.Close(); err != nil {
			logger.Warn("Torrent: error closing stream reader",
				"torrentID", h.ID(), "error", err)
		}
	}()

	safeName := sanitizeFilename(file.DisplayPath())

	logger.Info("Torrent: starting stream",
		"torrentID", h.ID(),
		"fileIndex", fileIndex,
		"fileName", safeName,
		"fileSize", file.Length(),
	)

	http.ServeContent(w, r, safeName, time.Now(), closer)
}

// ── handle ────────────────────────────────────────────────────────────────

type localHandle struct {
	t *torrent.Torrent
}

func (h *localHandle) ID() string            { return h.t.InfoHash().HexString() }
func (h *localHandle) Name() string          { return h.t.Info().BestName() }
func (h *localHandle) TotalLength() int64    { return h.t.Info().TotalLength() }
func (h *localHandle) BytesCompleted() int64 { return h.t.BytesCompleted() }
func (h *localHandle) Complete() bool        { return h.t.Complete().Bool() }

// ActivePeers counts live connections, not discovered addresses: a peer
// that never connected sends no bytes, and the window must not show it.
func (h *localHandle) ActivePeers() int { return h.t.Stats().ActivePeers }
func (h *localHandle) FileCount() int   { return len(h.t.Files()) }

func (h *localHandle) FileName(index int) string {
	files := h.t.Files()
	if index < 0 || index >= len(files) {
		return ""
	}
	return files[index].DisplayPath()
}

func (h *localHandle) FileLength(index int) int64 {
	files := h.t.Files()
	if index < 0 || index >= len(files) {
		return 0
	}
	return files[index].Length()
}

// SelectForStreaming drops priority for every other file so the client does not
// fetch content nobody is watching.
func (h *localHandle) SelectForStreaming(fileIndex int) error {
	files := h.t.Files()
	if err := validation.ValidateFileIndex(fileIndex, len(files)-1); err != nil {
		return errors.Wrap(errors.ErrInvalidInput, "invalid file index", err)
	}
	for _, f := range files {
		f.SetPriority(torrent.PiecePriorityNone)
	}
	files[fileIndex].SetPriority(torrent.PiecePriorityNormal)
	return nil
}

// BufferTarget hands buffer.Service the *torrent.File it needs to steer piece
// priorities. Local mode is exactly where that is meaningful.
func (h *localHandle) BufferTarget(fileIndex int) any {
	files := h.t.Files()
	if fileIndex < 0 || fileIndex >= len(files) {
		return nil
	}
	return files[fileIndex]
}

var _ io.ReadSeekCloser = (*readCloserWithClose)(nil)
