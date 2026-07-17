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
	AddMagnet(ctx context.Context, magnetURI string) (*models.TorrentInfo, error)
	AddTorrent(ctx context.Context, torrentData io.Reader) (*models.TorrentInfo, error)
	RemoveTorrent(ctx context.Context, id string) error
	ListTorrents(ctx context.Context) []*models.TorrentInfo
	GetFiles(ctx context.Context, torrentID string) ([]models.FileInfo, error)
	SelectFile(ctx context.Context, torrentID string, fileIndex int) error
	ServeFile(w http.ResponseWriter, r *http.Request, torrentID string)
	UpdateBufferPosition(ctx context.Context, torrentID string, position int64) error
	GetBufferInfo(ctx context.Context, torrentID string) (*models.BufferInfo, error)
	Close() error
}

type P2PService interface {
	// Session-scoped operations - userID identifies the user's session
	CreateRoom(ctx context.Context, userID, name, password string) (*models.RoomInfo, error)
	JoinRoom(ctx context.Context, userID, roomID, password string) error
	LeaveRoom(ctx context.Context, userID string) error
	SendSignal(ctx context.Context, userID string, signal []byte) error
	GetEvents(userID string) chan models.P2PEvent
	GetRoomInfo(ctx context.Context, userID string) (*models.RoomInfo, error)
	BroadcastSync(roomID string, syncData interface{})
	Close() error
}

type SyncService interface {
	Play(roomID string) models.SyncStatus
	Pause(roomID string) models.SyncStatus
	Seek(roomID string, position float64) (models.SyncStatus, error)
	GetStatus(roomID string) models.SyncStatus
	Close()
}
