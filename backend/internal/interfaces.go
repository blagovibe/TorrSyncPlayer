// Package internal contains interfaces for application services.
// Interfaces define contracts between application layers.
package internal

import (
	"context"
	"net/http"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

type TorrentService interface {
	AddMagnet(ctx context.Context, magnetURI string) (*models.TorrentInfo, error)
	RemoveTorrent(ctx context.Context, id string) error
	ListTorrents(ctx context.Context) []*models.TorrentInfo
	GetFiles(ctx context.Context, torrentID string) ([]models.FileInfo, error)
	SelectFile(ctx context.Context, torrentID string, fileIndex int) error
	ServeFile(w http.ResponseWriter, r *http.Request, torrentID string)
	UpdateBufferPosition(ctx context.Context, torrentID string, position int64)
	GetBufferInfo(ctx context.Context, torrentID string) (*models.BufferInfo, error)
	Close() error
}

type P2PService interface {
	CreateRoom(ctx context.Context, name, password string) (*models.RoomInfo, error)
	JoinRoom(ctx context.Context, roomID, password string) error
	JoinRoomWithToken(ctx context.Context, roomID, password, token string) error
	AuthenticatePeer(ctx context.Context, peerID, token string) error
	SetLocalUserID(userID string)
	LeaveRoom(ctx context.Context) error
	SendSignal(ctx context.Context, signal []byte) error
	GetEvents() chan models.P2PEvent
	GetRoomInfo(ctx context.Context) (*models.RoomInfo, error)
	Close() error
}

type SyncService interface {
	Play(ctx context.Context) models.SyncStatus
	Pause(ctx context.Context) models.SyncStatus
	Seek(ctx context.Context, position float64) (models.SyncStatus, error)
	GetStatus(ctx context.Context) models.SyncStatus
	SetDuration(ctx context.Context, duration float64) error
	SyncWithLatency(ctx context.Context, peerStatus models.SyncStatus, latencyMs int) models.SyncStatus
	UpdatePosition(ctx context.Context, position float64) error
	Close()
}
