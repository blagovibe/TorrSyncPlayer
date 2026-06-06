// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package p2p предоставляет сервис для P2P соединений через WebRTC.
// Управляет комнатами, пирами и событиями в потокобезопасном режиме.
// Поддерживает JWT аутентификацию пиров при подключении.
package p2p

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/pion/webrtc/v4"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

const (
	eventChannelSize = constants.P2PEventChannelSize
	peerIDLength     = constants.PeerIDLength
	maxRooms         = 1000
)

type Peer struct {
	ID            string
	UserID        string
	Username      string
	Connection    *webrtc.PeerConnection
	DataChannel   *webrtc.DataChannel
	LastHeartbeat time.Time
	Authenticated bool
}

type Room struct {
	ID          string
	Name        string
	HostID      string
	HostUserID  string
	Password    string
	Peers       map[string]*Peer
	CreatedAt   time.Time
	RequireAuth bool
}

type DataChannelEvent struct {
	PeerID      string
	DataChannel *webrtc.DataChannel
}

type Service struct {
	mu              sync.RWMutex
	rooms           map[string]*Room
	peers           map[string]*Peer
	eventChan       chan models.P2PEvent
	dataChannelChan chan DataChannelEvent
	api             *webrtc.API
	config          webrtc.Configuration
	currentRoom     string
	localPeerID     string
	localUserID     string
	closeChan       chan struct{}
	closeOnce       sync.Once
	wg              sync.WaitGroup
	authService     *auth.AuthService
	closed          atomic.Bool
}

func NewService(authService *auth.AuthService) (*Service, error) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
					"stun:stun1.l.google.com:19302",
				},
			},
		},
	}

	settingEngine := webrtc.SettingEngine{}
	webrtcAPI := webrtc.NewAPI(
		webrtc.WithSettingEngine(settingEngine),
	)

	localPeerID, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate local peer ID: %w", err)
	}

	service := &Service{
		rooms:           make(map[string]*Room),
		peers:           make(map[string]*Peer),
		eventChan:       make(chan models.P2PEvent, eventChannelSize),
		dataChannelChan: make(chan DataChannelEvent, eventChannelSize),
		api:             webrtcAPI,
		config:          config,
		localPeerID:     localPeerID,
		closeChan:       make(chan struct{}),
	}

	service.wg.Add(1)
	go func() {
		defer service.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logger.Error("P2P: горутина handleDataChannelEvents завершилась с паникой", "error", r)
			}
		}()
		service.handleDataChannelEvents()
	}()

	logger.Info("P2P сервис инициализирован",
		"localPeerID", service.localPeerID,
		"stun_servers", len(config.ICEServers[0].URLs),
	)
	return service, nil
}

func (s *Service) SetLocalUserID(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localUserID = userID
	logger.Info("P2P: установлен ID пользователя", "userID", userID)
}

func (s *Service) CreateRoom(name, password string) (*models.RoomInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.rooms) >= maxRooms {
		logger.Warn("P2P: превышен лимит комнат", "max", maxRooms)
		return nil, errors.New(errors.ErrUnavailable, "превышено максимальное количество комнат")
	}

	roomID, err := generateID()
	if err != nil {
		logger.Error("P2P: ошибка генерации room ID", "error", err)
		return nil, fmt.Errorf("ошибка генерации room ID: %w", err)
	}

	var passwordHash string
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), constants.BcryptCost)
		if err != nil {
			logger.Error("P2P: ошибка хеширования пароля", "error", err, "roomID", roomID)
			return nil, fmt.Errorf("ошибка хеширования пароля: %w", err)
		}
		passwordHash = string(hash)
	}

	room := &Room{
		ID:          roomID,
		Name:        name,
		HostID:      s.localPeerID,
		HostUserID:  s.localUserID,
		Password:    passwordHash,
		Peers:       make(map[string]*Peer),
		CreatedAt:   time.Now(),
		RequireAuth: constants.P2PDefaultRoomAuth,
	}

	s.rooms[roomID] = room
	s.currentRoom = roomID

	logger.Info("P2P: комната создана",
		"roomID", roomID,
		"name", name,
		"hostID", s.localPeerID,
		"hostUserID", s.localUserID,
		"hasPassword", password != "",
	)

	s.emitEvent("room_created", room)

	return &models.RoomInfo{
		ID:        room.ID,
		Name:      room.Name,
		HostID:    room.HostID,
		PeerCount: len(room.Peers),
	}, nil
}

func (s *Service) JoinRoom(roomID, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[roomID]
	if !exists {
		logger.Warn("P2P: комната не найдена", "roomID", roomID)
		return errors.NotFound("комната", roomID)
	}

	if room.Password != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(room.Password), []byte(password)); err != nil {
			logger.Warn("P2P: неверный пароль комнаты", "roomID", roomID)
			return errors.Unauthorized("неверный пароль")
		}
	}

	s.currentRoom = roomID

	peerConnection, err := s.api.NewPeerConnection(s.config)
	if err != nil {
		logger.Error("P2P: ошибка создания peer connection", "error", err, "roomID", roomID)
		return fmt.Errorf("ошибка создания peer connection: %w", err)
	}

	peer := &Peer{
		ID:            s.localPeerID,
		UserID:        s.localUserID,
		Connection:    peerConnection,
		LastHeartbeat: time.Now(),
		Authenticated: s.localUserID != "",
	}

	room.Peers[s.localPeerID] = peer
	s.peers[s.localPeerID] = peer

	s.setupPeerConnection(peerConnection, roomID)

	logger.Info("P2P: присоединились к комнате",
		"roomID", roomID,
		"peerID", s.localPeerID,
		"userID", s.localUserID,
		"authenticated", peer.Authenticated,
	)

	s.emitEvent("peer_joined", map[string]string{
		"peerID": s.localPeerID,
		"roomID": roomID,
	})

	return nil
}

func (s *Service) JoinRoomWithToken(roomID, password, token string) error {
	claims, err := s.authService.ValidateToken(token)
	if err != nil {
		logger.Warn("P2P: невалидный JWT токен", "roomID", roomID, "error", err)
		return fmt.Errorf("аутентификация не удалась: невалидный токен")
	}

	s.SetLocalUserID(claims.UserID)

	logger.Info("P2P: пир аутентифицирован",
		"roomID", roomID,
		"userID", claims.UserID,
		"username", claims.Username,
	)

	return s.JoinRoom(roomID, password)
}

func (s *Service) AuthenticatePeer(peerID, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	peer, exists := s.peers[peerID]
	if !exists {
		return fmt.Errorf("пир не найден: %s", peerID)
	}

	claims, err := s.authService.ValidateToken(token)
	if err != nil {
		logger.Warn("P2P: невалидный JWT токен пира", "peerID", peerID, "error", err)
		return fmt.Errorf("аутентификация не удалась")
	}

	peer.UserID = claims.UserID
	peer.Username = claims.Username
	peer.Authenticated = true

	logger.Info("P2P: пир аутентифицирован",
		"peerID", peerID,
		"userID", claims.UserID,
		"username", claims.Username,
	)

	s.emitEvent("peer_authenticated", map[string]string{
		"peerID":   peerID,
		"userID":   claims.UserID,
		"username": claims.Username,
	})

	return nil
}

func (s *Service) LeaveRoom() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentRoom == "" {
		return errors.New(errors.ErrInvalidInput, "не подключены к комнате")
	}

	roomID := s.currentRoom
	room, exists := s.rooms[roomID]
	if !exists {
		logger.Warn("P2P: комната не найдены при выходе", "roomID", roomID)
		return errors.NotFound("комната", roomID)
	}

	if peer, exists := s.peers[s.localPeerID]; exists {
		if peer.DataChannel != nil {
			if err := peer.DataChannel.Close(); err != nil {
				logger.Warn("P2P: ошибка закрытия DataChannel", "error", err, "peerID", s.localPeerID)
			}
		}
		if peer.Connection != nil {
			if err := peer.Connection.Close(); err != nil {
				logger.Warn("P2P: ошибка закрытия PeerConnection", "error", err, "peerID", s.localPeerID)
			}
		}
		delete(s.peers, s.localPeerID)
	}

	delete(room.Peers, s.localPeerID)

	if len(room.Peers) == 0 {
		delete(s.rooms, roomID)
		logger.Info("P2P: комната удалена (пустая)", "roomID", roomID)
	}

	s.currentRoom = ""

	logger.Info("P2P: вышли из комнаты", "roomID", roomID, "peerID", s.localPeerID)

	s.emitEvent("peer_left", map[string]string{
		"peerID": s.localPeerID,
		"roomID": roomID,
	})

	return nil
}

func (s *Service) SendSignal(signal []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentRoom == "" {
		return errors.InvalidInput("не подключены к комнате")
	}

	peer, exists := s.peers[s.localPeerID]
	if !exists {
		return errors.InvalidInput("пир не найден")
	}

	if peer.DataChannel == nil {
		return errors.InvalidInput("data channel не создан")
	}

	err := peer.DataChannel.Send(signal)
	if err != nil {
		logger.Error("P2P: ошибка отправки сигнала", "error", err, "roomID", s.currentRoom)
		return errors.Wrap(errors.ErrInternal, "ошибка отправки сигнала", err)
	}

	logger.Debug("P2P: сигнал отправлен", "roomID", s.currentRoom, "signalSize", len(signal))
	return nil
}

func (s *Service) GetEvents() chan models.P2PEvent {
	return s.eventChan
}

func (s *Service) GetRoomInfo() (*models.RoomInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentRoom == "" {
		return nil, fmt.Errorf("не подключены к комнате")
	}

	room, exists := s.rooms[s.currentRoom]
	if !exists {
		return nil, fmt.Errorf("комната не найдена")
	}

	return &models.RoomInfo{
		ID:        room.ID,
		Name:      room.Name,
		HostID:    room.HostID,
		PeerCount: len(room.Peers),
	}, nil
}

func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		close(s.closeChan)
	})

	s.mu.Lock()
	s.closed.Store(true)

	for peerID, peer := range s.peers {
		if peer.DataChannel != nil {
			_ = peer.DataChannel.Close()
		}
		if peer.Connection != nil {
			_ = peer.Connection.Close()
		}
		logger.Debug("P2P: закрыто подключение пира", "peerID", peerID)
	}

	roomCount := len(s.rooms)
	s.rooms = make(map[string]*Room)
	s.peers = make(map[string]*Peer)
	s.currentRoom = ""
	s.mu.Unlock()

	close(s.eventChan)

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		logger.Info("P2P сервис остановлен", "closedRooms", roomCount)
	case <-time.After(5 * time.Second):
		logger.Warn("P2P: таймаут ожидания завершения горутины")
	}
	return nil
}

func (s *Service) setupPeerConnection(pc *webrtc.PeerConnection, roomID string) {
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		s.emitEvent("ice_candidate", candidate.ToJSON())
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		logger.Info("P2P: состояние подключения изменилось",
			"state", state.String(),
			"roomID", roomID,
		)

		switch state {
		case webrtc.PeerConnectionStateConnected:
			s.emitEvent("connected", nil)
		case webrtc.PeerConnectionStateDisconnected:
			s.emitEvent("disconnected", nil)
		case webrtc.PeerConnectionStateFailed:
			s.emitEvent("failed", nil)
		}
	})

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		logger.Info("P2P: получен data channel", "label", dc.Label(), "roomID", roomID)

		dc.OnOpen(func() {
			s.emitEvent("data_channel_open", dc.Label())
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			s.emitEvent("data_channel_message", msg.Data)
		})

		dc.OnClose(func() {
			s.emitEvent("data_channel_close", dc.Label())
		})

		select {
		case s.dataChannelChan <- DataChannelEvent{
			PeerID:      s.localPeerID,
			DataChannel: dc,
		}:
			logger.Debug("P2P: DataChannel отправлен в канал обработки", "peerID", s.localPeerID)
		default:
			logger.Warn("P2P: канал DataChannel полный, событие пропущено", "peerID", s.localPeerID)
		}
	})
}

func (s *Service) handleDataChannelEvents() {
	logger.Info("P2P: обработчик DataChannel событий запущен")
	for {
		select {
		case event := <-s.dataChannelChan:
			s.mu.Lock()
			if peer, exists := s.peers[event.PeerID]; exists {
				peer.DataChannel = event.DataChannel
				logger.Debug("P2P: DataChannel сохранён для пира", "peerID", event.PeerID)
			} else {
				logger.Warn("P2P: пир не найден для DataChannel", "peerID", event.PeerID)
			}
			s.mu.Unlock()
		case <-s.closeChan:
			logger.Info("P2P: обработчик DataChannel событий остановлен")
			return
		}
	}
}

func (s *Service) emitEvent(eventType string, data interface{}) {
	if s.closed.Load() {
		return
	}

	select {
	case s.eventChan <- models.P2PEvent{
		Type: eventType,
		Data: data,
	}:
	default:
		logger.Warn("P2P: канал событий полный, событие пропущено", "type", eventType)
	}
}

func generateID() (string, error) {
	bytes := make([]byte, peerIDLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
