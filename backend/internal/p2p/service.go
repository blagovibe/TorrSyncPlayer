// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package p2p provides the P2P service via WebRTC.
// Manages rooms, peers and events in a thread-safe manner.
// Supports JWT peer authentication on connection.
package p2p

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/pion/webrtc/v4"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/utils"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

const (
	eventChannelSize       = constants.P2PEventChannelSize
	peerIDLength           = constants.PeerIDLength
	maxRooms               = 1000
	p2pCloseTimeoutDefault = 5 * time.Second
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
	doneChan        chan struct{}
	closeOnce       sync.Once
	wg              sync.WaitGroup
	authService     *auth.AuthService
	closed          atomic.Bool
}

func loadICEServers() []webrtc.ICEServer {
	servers := []webrtc.ICEServer{
		{
			URLs: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
			},
		},
	}

	if turnURL := os.Getenv("TURN_URL"); turnURL != "" {
		turnServer := webrtc.ICEServer{
			URLs: []string{turnURL},
		}
		if username := os.Getenv("TURN_USERNAME"); username != "" {
			turnServer.Username = username
		}
		if credential := os.Getenv("TURN_CREDENTIAL"); credential != "" {
			turnServer.Credential = credential
		}
		servers = append(servers, turnServer)
		logger.Info("P2P: TURN server configured", "url", turnURL)
	}

	return servers
}

func NewService(authService *auth.AuthService) (*Service, error) {
	config := webrtc.Configuration{
		ICEServers: loadICEServers(),
	}

	settingEngine := webrtc.SettingEngine{}
	webrtcAPI := webrtc.NewAPI(
		webrtc.WithSettingEngine(settingEngine),
	)

	localPeerID, err := utils.GenerateID(peerIDLength)
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
		doneChan:        make(chan struct{}),
	}

	service.wg.Add(1)
	go func() {
		defer service.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logger.Error("P2P: handleDataChannelEvents goroutine exited with panic", "error", r)
			}
		}()
		service.handleDataChannelEvents()
	}()

	logger.Info("P2P service initialized",
		"localPeerID", service.localPeerID,
		"stun_servers", len(config.ICEServers[0].URLs),
	)
	return service, nil
}

func (s *Service) SetLocalUserID(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localUserID = userID
	logger.Info("P2P: user ID set", "userID", userID)
}

func (s *Service) CreateRoom(ctx context.Context, name, password string) (*models.RoomInfo, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.rooms) >= maxRooms {
		logger.Warn("P2P: room limit exceeded", "max", maxRooms)
		return nil, errors.New(errors.ErrUnavailable, "maximum number of rooms exceeded")
	}

	roomID, err := utils.GenerateID(peerIDLength)
	if err != nil {
		logger.Error("P2P: room ID generation error", "error", err)
		return nil, fmt.Errorf("room ID generation error: %w", err)
	}

	var passwordHash string
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), constants.BcryptCost)
		if err != nil {
			logger.Error("P2P: password hashing error", "error", err, "roomID", roomID)
			return nil, fmt.Errorf("password hashing error: %w", err)
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

	logger.Info("P2P: room created",
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

func (s *Service) JoinRoom(ctx context.Context, roomID, password string) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[roomID]
	if !exists {
		logger.Warn("P2P: room not found", "roomID", roomID)
		return errors.NotFound("room", roomID)
	}

	if room.Password != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(room.Password), []byte(password)); err != nil {
			logger.Warn("P2P: invalid room password", "roomID", roomID)
			return errors.Unauthorized("invalid password")
		}
	}

	s.currentRoom = roomID

	peerConnection, err := s.api.NewPeerConnection(s.config)
	if err != nil {
		logger.Error("P2P: error creating peer connection", "error", err, "roomID", roomID)
		return fmt.Errorf("error creating peer connection: %w", err)
	}

	s.setupPeerConnection(peerConnection, roomID)

	peer := &Peer{
		ID:            s.localPeerID,
		UserID:        s.localUserID,
		Connection:    peerConnection,
		LastHeartbeat: time.Now(),
		Authenticated: s.localUserID != "",
	}

	room.Peers[s.localPeerID] = peer
	s.peers[s.localPeerID] = peer

	metrics.GetInstance().PeerJoined()

	logger.Info("P2P: joined room",
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

func (s *Service) JoinRoomWithToken(ctx context.Context, roomID, password, token string) error {
	_ = ctx
	claims, err := s.authService.ValidateTokenWithRevocation(token)
	if err != nil {
		logger.Warn("P2P: invalid or revoked JWT token", "roomID", roomID, "error", err)
		return fmt.Errorf("authentication failed: invalid token")
	}

	s.SetLocalUserID(claims.UserID)

	logger.Info("P2P: peer authenticated",
		"roomID", roomID,
		"userID", claims.UserID,
		"username", claims.Username,
	)

	return s.JoinRoom(ctx, roomID, password)
}

func (s *Service) AuthenticatePeer(ctx context.Context, peerID, token string) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	peer, exists := s.peers[peerID]
	if !exists {
		return fmt.Errorf("peer not found: %s", peerID)
	}

	claims, err := s.authService.ValidateTokenWithRevocation(token)
	if err != nil {
		logger.Warn("P2P: invalid or revoked peer JWT token", "peerID", peerID, "error", err)
		return fmt.Errorf("authentication failed")
	}

	peer.UserID = claims.UserID
	peer.Username = claims.Username
	peer.Authenticated = true

	logger.Info("P2P: peer authenticated",
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

func (s *Service) LeaveRoom(ctx context.Context) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentRoom == "" {
		return errors.New(errors.ErrInvalidInput, "not connected to a room")
	}

	roomID := s.currentRoom
	room, exists := s.rooms[roomID]
	if !exists {
		logger.Warn("P2P: room not found on leave", "roomID", roomID)
		return errors.NotFound("room", roomID)
	}

	if peer, exists := s.peers[s.localPeerID]; exists {
		if peer.DataChannel != nil {
			if err := peer.DataChannel.Close(); err != nil {
				logger.Warn("P2P: error closing DataChannel", "error", err, "peerID", s.localPeerID)
			}
		}
		if peer.Connection != nil {
			if err := peer.Connection.Close(); err != nil {
				logger.Warn("P2P: error closing PeerConnection", "error", err, "peerID", s.localPeerID)
			}
		}
		delete(s.peers, s.localPeerID)
	}

	delete(room.Peers, s.localPeerID)

	if len(room.Peers) == 0 {
		delete(s.rooms, roomID)
		metrics.GetInstance().RoomClosed()
		logger.Info("P2P: room deleted (empty)", "roomID", roomID)
	}

	metrics.GetInstance().PeerLeft()
	s.currentRoom = ""

	logger.Info("P2P: left room", "roomID", roomID, "peerID", s.localPeerID)

	s.emitEvent("peer_left", map[string]string{
		"peerID": s.localPeerID,
		"roomID": roomID,
	})

	return nil
}

func (s *Service) SendSignal(ctx context.Context, signal []byte) error {
	_ = ctx
	if len(signal) > constants.MaxSignalSize {
		return errors.InvalidInput(fmt.Sprintf("signal exceeds maximum size: %d > %d", len(signal), constants.MaxSignalSize))
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentRoom == "" {
		return errors.InvalidInput("not connected to a room")
	}

	peer, exists := s.peers[s.localPeerID]
	if !exists {
		return errors.InvalidInput("peer not found")
	}

	if peer.DataChannel == nil {
		return errors.InvalidInput("data channel not created")
	}

	err := peer.DataChannel.Send(signal)
	if err != nil {
		logger.Error("P2P: error sending signal", "error", err, "roomID", s.currentRoom)
		return errors.Wrap(errors.ErrInternal, "error sending signal", err)
	}

	logger.Debug("P2P: signal sent", "roomID", s.currentRoom, "signalSize", len(signal))
	return nil
}

func (s *Service) GetEvents() chan models.P2PEvent {
	return s.eventChan
}

func (s *Service) GetRoomInfo(ctx context.Context) (*models.RoomInfo, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentRoom == "" {
		return nil, fmt.Errorf("not connected to a room")
	}

	room, exists := s.rooms[s.currentRoom]
	if !exists {
		return nil, fmt.Errorf("room not found")
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

	// Collect connections for closing under lock,
	// but close them after releasing the lock
	// to avoid deadlock: WebRTC OnClose/OnError callbacks
	// call emitEvent, which tries to acquire RLock.
	type peerConnections struct {
		dataChannel io.Closer
		connection  io.Closer
		id          string
	}
	var toClose []peerConnections
	var roomCount int

	s.mu.Lock()
	s.closed.Store(true)

	for peerID, peer := range s.peers {
		conns := peerConnections{id: peerID}
		if peer.DataChannel != nil {
			conns.dataChannel = peer.DataChannel
		}
		if peer.Connection != nil {
			conns.connection = peer.Connection
		}
		toClose = append(toClose, conns)
	}

	roomCount = len(s.rooms)
	s.rooms = make(map[string]*Room)
	s.peers = make(map[string]*Peer)
	s.currentRoom = ""

	// Close the done channel to signal emitEvent to stop sending.
	// eventChan is left open so consumers can drain remaining events.
	close(s.doneChan)
	s.mu.Unlock()

	// Close connections outside lock — WebRTC callbacks must not
	// acquire mu and cause a deadlock.
	for _, conns := range toClose {
		if conns.dataChannel != nil {
			_ = conns.dataChannel.Close()
		}
		if conns.connection != nil {
			_ = conns.connection.Close()
		}
		logger.Debug("P2P: peer connection closed", "peerID", conns.id)
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		logger.Info("P2P service stopped", "closedRooms", roomCount)
	case <-time.After(p2pCloseTimeoutDefault):
		logger.Warn("P2P: timeout waiting for goroutine to finish")
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
		logger.Info("P2P: connection state changed",
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
		logger.Info("P2P: data channel received", "label", dc.Label(), "roomID", roomID)

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
			logger.Debug("P2P: DataChannel sent to processing channel", "peerID", s.localPeerID)
		default:
			logger.Warn("P2P: DataChannel channel full, event dropped", "peerID", s.localPeerID)
		}
	})
}

func (s *Service) handleDataChannelEvents() {
	logger.Info("P2P: DataChannel event handler started")
	for {
		select {
		case event := <-s.dataChannelChan:
			s.mu.Lock()
			if peer, exists := s.peers[event.PeerID]; exists {
				peer.DataChannel = event.DataChannel
				logger.Debug("P2P: DataChannel saved for peer", "peerID", event.PeerID)
			} else {
				logger.Warn("P2P: peer not found for DataChannel", "peerID", event.PeerID)
			}
			s.mu.Unlock()
		case <-s.closeChan:
			logger.Info("P2P: DataChannel event handler stopped")
			return
		}
	}
}

func (s *Service) emitEvent(eventType string, data interface{}) {
	if s.closed.Load() {
		return
	}

	select {
	case <-s.doneChan:
		return
	default:
	}

	select {
	case s.eventChan <- models.P2PEvent{
		Type: eventType,
		Data: data,
	}:
	default:
		logger.Warn("P2P: event channel full, event dropped",
			"type", eventType,
			"currentRoom", s.currentRoom,
			"roomCount", len(s.rooms),
		)
	}
}
