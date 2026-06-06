// Package internal содержит интерфейсы для сервисов приложения.
// Интерфейсы определяют контракты между слоями приложения.
package internal

import (
	"context"
	"net/http"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

type TorrentService interface {
	AddMagnet(ctx context.Context, magnetURI string) (*models.TorrentInfo, error)
	RemoveTorrent(ctx context.Context, id string) error
	ListTorrents() []*models.TorrentInfo
	GetFiles(torrentID string) ([]models.FileInfo, error)
	SelectFile(torrentID string, fileIndex int) error
	ServeFile(w http.ResponseWriter, r *http.Request, torrentID string)
	UpdateBufferPosition(torrentID string, position int64)
	GetBufferInfo(torrentID string) (*models.BufferInfo, error)
	Close() error
}

type P2PService interface {
	CreateRoom(name, password string) (*models.RoomInfo, error)
	JoinRoom(roomID, password string) error
	JoinRoomWithToken(roomID, password, token string) error
	AuthenticatePeer(peerID, token string) error
	SetLocalUserID(userID string)
	LeaveRoom() error
	SendSignal(signal []byte) error
	GetEvents() chan models.P2PEvent
	GetRoomInfo() (*models.RoomInfo, error)
	Close() error
}

type SyncService interface {
	Play() models.SyncStatus
	Pause() models.SyncStatus
	Seek(position float64) (models.SyncStatus, error)
	GetStatus() models.SyncStatus
	SetDuration(duration float64) error
	SyncWithLatency(peerStatus models.SyncStatus, latencyMs int) models.SyncStatus
	UpdatePosition(position float64) error
	Close()
}
