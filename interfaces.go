package main

import "context"

// Initializer интерфейс для инициализации сервисов
type Initializer interface {
	Init(ctx context.Context) error
}

// TorrentServiceInterface определяет контракт для работы с торрентами
type TorrentServiceInterface interface {
	Initializer
	AddTorrentByMagnet(magnetURI string) (*TorrentInfo, error)
	AddTorrentByFile(filePath string) (*TorrentInfo, error)
	RemoveTorrent(hash string) error
	PauseTorrent(hash string) error
	ResumeTorrent(hash string) error
	GetTorrentInfo(hash string) (*TorrentInfo, error)
	GetAllTorrents() []*TorrentInfo
	GetStreamURL(hash string, filePath string) string
	GetFile(hash string, filePath string) (*TorrentFile, error)
	StartHTTPServer() error
	StopHTTPServer() error
	SetStreamPort(port int)
}

// P2PServiceInterface определяет контракт для P2P соединений
type P2PServiceInterface interface {
	Initializer
	CreateRoom() (string, error)
	CreateRoomWithPassword(password string) (string, error)
	JoinRoom(roomID string) error
	JoinRoomWithPassword(roomID string, password string) error
	LeaveRoom() error
	SendMessage(msg P2PMessage) error
	BroadcastMessage(msg P2PMessage) error
	GetPeers() []PeerInfo
	GetRoomID() string
	IsHost() bool
	HasRoomPassword() bool
	SetRoomPassword(password string) error
	VerifyRoomPassword(password string) bool
	Close() error
}

// SyncServiceInterface определяет контракт для синхронизации
type SyncServiceInterface interface {
	Initializer
	Play(position float64) error
	Pause() error
	Seek(position float64) error
	GetPlaybackState() PlaybackState
	GetSyncStats() SyncStats
	SetLatencyCompensation(ms int)
	OnStateChange(handler func(PlaybackState))
	OnSyncStats(handler func(SyncStats))
	SetP2PService(p2p P2PServiceInterface)
}
