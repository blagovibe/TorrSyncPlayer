package main

import (
	"time"

	"github.com/pion/webrtc/v4"
)

// TorrentInfo информация о торренте
type TorrentInfo struct {
	Hash          string        `json:"hash"`
	Name          string        `json:"name"`
	Size          int64         `json:"size"`
	Progress      float64       `json:"progress"`
	Peers         int           `json:"peers"`
	Seeds         int           `json:"seeds"`
	DownloadSpeed float64       `json:"downloadSpeed"`
	UploadSpeed   float64       `json:"uploadSpeed"`
	Status        string        `json:"status"`
	Files         []TorrentFile `json:"files"`
	Speed         int64         `json:"speed"`
}

// TorrentFile файл в торренте
type TorrentFile struct {
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Size     int64   `json:"size"`
	Offset   int64   `json:"offset"`
	Progress float64 `json:"progress"`
}

// TorrentFileInfo информация о файле в торренте (для обратной совместимости)
type TorrentFileInfo = TorrentFile

// PeerInfo информация о пире
type PeerInfo struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IsHost    bool      `json:"isHost"`
	IsLeader  bool      `json:"isLeader"`
	Connected bool      `json:"connected"`
	LastSeen  time.Time `json:"lastSeen"`
}

// PeerData внутренние данные о пире
type PeerData struct {
	PeerConnection *webrtc.PeerConnection
	DataChannel    *webrtc.DataChannel
	IsHost         bool
	LastSeen       time.Time
	Connected      bool
}

// P2PMessageType тип сообщения P2P
type P2PMessageType string

const (
	MsgPlay        P2PMessageType = "play"
	MsgPause       P2PMessageType = "pause"
	MsgSeek        P2PMessageType = "seek"
	MsgHeartbeat   P2PMessageType = "heartbeat"
	MsgState       P2PMessageType = "state"
	MsgChat        P2PMessageType = "chat"
	MsgTorrentInfo P2PMessageType = "torrent_info"
)

// P2PMessage сообщение P2P
type P2PMessage struct {
	Type      P2PMessageType `json:"type"`
	Timestamp int64          `json:"timestamp"`
	Data      interface{}    `json:"data,omitempty"`
}

// PlaybackState состояние воспроизведения
type PlaybackState struct {
	IsPlaying    bool    `json:"isPlaying"`
	Position     float64 `json:"position"`
	Duration     float64 `json:"duration"`
	Speed        float64 `json:"speed"`
	Timestamp    int64   `json:"timestamp"`
	PlaybackRate float64 `json:"playbackRate"`
}

// SyncStats статистика синхронизации
type SyncStats struct {
	Latency          int     `json:"latency"`
	Drift            float64 `json:"drift"`
	SyncAccuracy     float64 `json:"syncAccuracy"`
	RebufferingCount int     `json:"rebufferingCount"`
	RTT              float64 `json:"rtt"`
	LastSyncTime     int64   `json:"lastSyncTime"`
	SyncTolerance    float64 `json:"syncTolerance"`
	CorrectionCount  int     `json:"correctionCount"`
}

// SyncCommand команда синхронизации
type SyncCommand struct {
	Type      string      `json:"type"`
	Timestamp int64       `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// RoomConfig конфигурация комнаты
type RoomConfig struct {
	ID          string `json:"id"`
	HasPassword bool   `json:"hasPassword"`
	MaxPeers    int    `json:"maxPeers"`
}
