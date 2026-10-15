// Package internal contains interfaces for application services.
// Interfaces define contracts between application layers.
package internal

import (
	"context"
	"io"
	"net/http"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

type TorrentService interface {
	// AddMagnet adds a torrent and records userID as its owner.
	AddMagnet(ctx context.Context, userID, magnetURI string) (*models.TorrentInfo, error)
	// AddTorrent adds a torrent from metadata and records userID as its owner.
	AddTorrent(ctx context.Context, userID string, torrentData io.Reader) (*models.TorrentInfo, error)
	// RemoveTorrent drops a torrent. Refused unless userID added it (bug-0049).
	RemoveTorrent(ctx context.Context, id, userID string) error
	// ListTorrents returns the torrents userID may read (bug-0049).
	ListTorrents(ctx context.Context, userID string) []*models.TorrentInfo
	// GetFiles lists a torrent's files; refused for a torrent userID may not
	// read (bug-0049).
	GetFiles(ctx context.Context, userID, torrentID string) ([]models.FileInfo, error)
	// CanRead reports whether userID may read torrentID (bug-0049 / deviation-0039).
	CanRead(ctx context.Context, userID, torrentID string) bool

	// MagnetFor returns a magnet URI that identifies torrentID, so a peer that
	// knows only an infohash can still add the torrent to its own backend.
	// Every participant pulls the media from the swarm themselves, and the
	// infohash identifies a torrent without carrying one.
	MagnetFor(ctx context.Context, userID, torrentID string) (string, error)

	// SelectFile selects a file for streaming for a specific user (per-peer).
	// userID is the caller's client ID (X-Client-ID header).
	SelectFile(ctx context.Context, torrentID, userID string, fileIndex int) error
	// ServeFile streams the file selected by the calling user.
	// userID is extracted from the request context (set by TokenMiddleware).
	ServeFile(w http.ResponseWriter, r *http.Request, torrentID, userID string)
	UpdateBufferPosition(ctx context.Context, torrentID string, position int64) error
	GetBufferInfo(ctx context.Context, userID, torrentID string) (*models.BufferInfo, error)
	Close() error
}
