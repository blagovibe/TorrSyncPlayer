// Package internal contains interfaces for application services.
// Interfaces define contracts between application layers.
package internal

import (
	"context"
	"io"
	"net/http"
	"time"

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
	JoinRoomWithToken(ctx context.Context, token, roomID, password string) error
	AuthenticatePeer(ctx context.Context, userID, peerID, token string) error
	LeaveRoom(ctx context.Context, userID string) error
	SendSignal(ctx context.Context, userID string, signal []byte) error
	GetEvents(userID string) chan models.P2PEvent
	GetRoomInfo(ctx context.Context, userID string) (*models.RoomInfo, error)
	Close() error
}

// P2PSessionManager manages P2P user sessions with isolated state.
type P2PSessionManager interface {
	CreateSession(userID string) string
	GetSession(userID string) *P2PSession
	RemoveSession(sessionID string)
}

// P2PSession represents a user's P2P session with isolated state.
type P2PSession interface {
	ID() string
	UserID() string
	CurrentRoom() string
	SetRoom(roomID string)
	// WebRTC operations
	PeerConnection() interface{} // *webrtc.PeerConnection
	DataChannel() interface{}    // *webrtc.DataChannel
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

// AuthServiceInterface defines the contract for authentication services.
type AuthServiceInterface interface {
	GenerateToken(user *models.User) (string, error)
	ValidateToken(tokenString string) (*models.Claims, error)
	ValidateTokenWithRevocation(tokenString string) (*models.Claims, error)
	GenerateRefreshToken(user *models.User) (string, error) // H1: Added refresh token support
	ValidateRefreshToken(tokenString string) (*models.Claims, error)
	SetTokenTTL(ttl time.Duration)
	SetIssuer(issuer string)
	SetAudience(audience ...string)
	Stop()
}

// BufferServiceInterface defines the contract for buffer management services.
type BufferServiceInterface interface {
	SetPosition(ctx context.Context, torrentID string, position int64) error
	GetBufferInfo(ctx context.Context, torrentID string) (*models.BufferInfo, error)
	Close()
}
