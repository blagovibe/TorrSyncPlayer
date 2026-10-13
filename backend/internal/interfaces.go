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

type P2PService interface {
	// Session-scoped operations - userID identifies the user's session
	CreateRoom(ctx context.Context, userID, name, password, torrentID string) (*models.RoomInfo, error)
	JoinRoom(ctx context.Context, userID, roomID, password, torrentID string) error
	LeaveRoom(ctx context.Context, userID string) error
	SendSignal(ctx context.Context, userID string, signal []byte) error
	GetEvents(userID string) chan models.P2PEvent
	GetRoomInfo(ctx context.Context, userID string) (*models.RoomInfo, error)
	IsHost(ctx context.Context, userID, roomID string) (bool, error)
	BroadcastSync(roomID string, event models.SyncEvent)
	// TouchPeer records that the user is still connected, refreshing the
	// heartbeat the idle pruner reads. Driven by the SSE stream (bug-0021).
	TouchPeer(ctx context.Context, userID string) error
	// TorrentsVisibleTo reports which torrents a peer may read: those bound to
	// the room it participates in. The access token is per-process and cannot
	// tell a room guest from the machine's owner (bug-0049).
	TorrentsVisibleTo(ctx context.Context, userID string) map[string]bool
	// SetPeerBuffering records that a peer reported itself as buffering or
	// caught up, so the roster can name it (idea-0017).
	SetPeerBuffering(ctx context.Context, userID string, buffering bool)
	// BroadcastPeerList sends the room's current roster to everyone in it.
	BroadcastPeerList(roomID string)
	Close() error
}

// HostChecker checks if a user is the host of a room.
type HostChecker interface {
	IsHost(ctx context.Context, userID, roomID string) (bool, error)
}

type SyncService interface {
	Play(ctx context.Context, roomID, userID string) (models.SyncStatus, error)
	Pause(ctx context.Context, roomID, userID string) (models.SyncStatus, error)
	Seek(ctx context.Context, roomID, userID string, position float64) (models.SyncStatus, error)
	Buffering(ctx context.Context, roomID, userID string, buffering bool) (models.SyncStatus, error)
	GetStatus(roomID string) models.SyncStatus
	Close()
}
