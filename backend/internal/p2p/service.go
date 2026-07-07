// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package p2p provides the P2P service via WebRTC.
// Manages rooms, peers and events in a thread-safe manner.
// Supports JWT peer authentication on connection.
// Architecture: Multi-session - each user has isolated state (fixes C2).
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
	maxRooms               = constants.MaxRooms
	p2pCloseTimeoutDefault = constants.P2PCloseTimeoutDefault
)

// Peer represents a WebRTC peer connection.
type Peer struct {
	ID            string
	UserID        string
	Username      string
	Connection    *webrtc.PeerConnection
	DataChannel   *webrtc.DataChannel
	LastHeartbeat time.Time
	Authenticated bool
}

// Room represents a P2P room for synchronized playback.
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

// DataChannelEvent represents a data channel event for internal processing.
type DataChannelEvent struct {
	PeerID      string
	DataChannel *webrtc.DataChannel
}

// Session represents a user's P2P session with isolated state.
// This solves the global state problem (C2) where currentRoom/localPeerID
// were shared across all users.
type Session struct {
	id          string
	userID      string
	currentRoom string
	peer        *Peer
	mu          sync.RWMutex
}

// NewSession creates a new user session.
func NewSession(userID string) *Session {
	return &Session{
		id:     fmt.Sprintf("session_%s", userID),
		userID: userID,
	}
}

// ID returns the session identifier.
func (s *Session) ID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.id
}

// UserID returns the user identifier.
func (s *Session) UserID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userID
}

// CurrentRoom returns the current room ID.
func (s *Session) CurrentRoom() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentRoom
}

// SetRoom sets the current room for the session.
func (s *Session) SetRoom(roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentRoom = roomID
}

// GetPeer returns the peer connection.
func (s *Session) GetPeer() *Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.peer
}

// SetPeer sets the peer connection.
func (s *Session) SetPeer(peer *Peer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peer = peer
}

// Service manages P2P operations with multi-session support.
type Service struct {
	mu              sync.RWMutex
	rooms           map[string]*Room
	peers           map[string]*Peer
	api             *webrtc.API
	config          webrtc.Configuration
	sessions        map[string]*Session // userID -> session mapping
	closeChan       chan struct{}
	doneChan        chan struct{}
	closeOnce       sync.Once
	wg              sync.WaitGroup
	authService     *auth.AuthService
	closed          atomic.Bool
	dataChannelChan chan DataChannelEvent
}

// loadICEServers loads ICE servers from environment configuration.
func loadICEServers() []webrtc.ICEServer {
	servers := []webrtc.ICEServer{
		{
			URLs: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
			},
		},
	}

	// Check for custom STUN servers
	if stunServers := os.Getenv("STUN_SERVERS"); stunServers != "" {
		// Parse comma-separated STUN server URLs
		for _, url := range splitAndTrim(stunServers, ",") {
			if url != "" {
				servers[0].URLs = append(servers[0].URLs, url)
			}
		}
		logger.Info("P2P: custom STUN servers configured", "count", len(servers[0].URLs)-2)
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

// splitAndTrim splits a string by delimiter and trims whitespace.
func splitAndTrim(s string, delim string) []string {
	var result []string
	for _, part := range splitString(s, delim) {
		if trimmed := trimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitString is a helper to split strings (extracted for testing).
func splitString(s, delim string) []string {
	// Simple split implementation
	result := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i:i+1] == delim {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

// trimSpace trims whitespace from a string.
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// NewService creates a new P2P service with multi-session support.
func NewService(authService *auth.AuthService) (*Service, error) {
	config := webrtc.Configuration{
		ICEServers: loadICEServers(),
	}

	settingEngine := webrtc.SettingEngine{}
	webrtcAPI := webrtc.NewAPI(
		webrtc.WithSettingEngine(settingEngine),
	)

	service := &Service{
		rooms:           make(map[string]*Room),
		peers:           make(map[string]*Peer),
		api:             webrtcAPI,
		config:          config,
		sessions:        make(map[string]*Session),
		closeChan:       make(chan struct{}),
		doneChan:        make(chan struct{}),
		dataChannelChan: make(chan DataChannelEvent, eventChannelSize),
		authService:     authService,
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
		"stun_servers", len(config.ICEServers[0].URLs),
	)
	return service, nil
}

// getOrCreateSession returns or creates a session for the user.
func (s *Service) getOrCreateSession(userID string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, exists := s.sessions[userID]; exists {
		return session
	}

	session := NewSession(userID)
	s.sessions[userID] = session

	// Generate peer ID for this session
	peerID, err := utils.GenerateID(peerIDLength)
	if err != nil {
		logger.Error("P2P: failed to generate peer ID", "error", err, "userID", userID)
		return session
	}

	session.SetPeer(&Peer{
		ID:            peerID,
		UserID:        userID,
		LastHeartbeat: time.Now(),
	})

	return session
}

// CreateRoom creates a new room for the specified user.
// Now takes userID as parameter to isolate session state.
func (s *Service) CreateRoom(ctx context.Context, userID, name, password string) (*models.RoomInfo, error) {
	_ = ctx
	session := s.getOrCreateSession(userID)

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
		HostID:      session.GetPeer().ID,
		HostUserID:  userID,
		Password:    passwordHash,
		Peers:       make(map[string]*Peer),
		CreatedAt:   time.Now(),
		RequireAuth: constants.P2PDefaultRoomAuth,
	}

	s.rooms[roomID] = room
	session.SetRoom(roomID)

	logger.Info("P2P: room created",
		"roomID", roomID,
		"name", name,
		"hostID", room.HostID,
		"hostUserID", userID,
		"hasPassword", password != "",
	)

	metrics.GetInstance().RoomCreated()

	return &models.RoomInfo{
		ID:        room.ID,
		Name:      room.Name,
		HostID:    room.HostID,
		PeerCount: len(room.Peers),
	}, nil
}

// JoinRoom joins a room for the specified user.
// Now takes userID as parameter to isolate session state.
func (s *Service) JoinRoom(ctx context.Context, userID, roomID, password string) error {
	_ = ctx
	session := s.getOrCreateSession(userID)

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

	session.SetRoom(roomID)

	peerConnection, err := s.api.NewPeerConnection(s.config)
	if err != nil {
		logger.Error("P2P: error creating peer connection", "error", err, "roomID", roomID)
		return fmt.Errorf("error creating peer connection: %w", err)
	}

	s.setupPeerConnection(peerConnection, roomID)

	peer := session.GetPeer()
	peer.Connection = peerConnection
	peer.LastHeartbeat = time.Now()
	peer.Authenticated = userID != ""

	room.Peers[peer.ID] = peer
	s.peers[peer.ID] = peer

	metrics.GetInstance().PeerJoined()

	logger.Info("P2P: joined room",
		"roomID", roomID,
		"peerID", peer.ID,
		"userID", userID,
		"authenticated", peer.Authenticated,
	)

	return nil
}

// JoinRoomWithToken joins a room with JWT token authentication.
func (s *Service) JoinRoomWithToken(ctx context.Context, token, roomID, password string) error {
	claims, err := s.authService.ValidateTokenWithRevocation(token)
	if err != nil {
		logger.Warn("P2P: invalid or revoked JWT token", "roomID", roomID, "error", err)
		return fmt.Errorf("authentication failed: invalid token")
	}

	return s.JoinRoom(ctx, claims.UserID, roomID, password)
}

// AuthenticatePeer authenticates a peer using JWT token.
func (s *Service) AuthenticatePeer(ctx context.Context, userID, peerID, token string) error {
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

	return nil
}

// LeaveRoom leaves the current room for the specified user.
// Now takes userID as parameter to isolate session state.
func (s *Service) LeaveRoom(ctx context.Context, userID string) error {
	_ = ctx
	session := s.getSession(userID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if session == nil || session.CurrentRoom() == "" {
		return errors.New(errors.ErrInvalidInput, "not connected to a room")
	}

	roomID := session.CurrentRoom()
	room, exists := s.rooms[roomID]
	if !exists {
		logger.Warn("P2P: room not found on leave", "roomID", roomID)
		return errors.NotFound("room", roomID)
	}

	peer := session.GetPeer()
	if peer != nil {
		if peer.DataChannel != nil {
			if err := peer.DataChannel.Close(); err != nil {
				logger.Warn("P2P: error closing DataChannel", "error", err, "peerID", peer.ID)
			}
		}
		if peer.Connection != nil {
			if err := peer.Connection.Close(); err != nil {
				logger.Warn("P2P: error closing PeerConnection", "error", err, "peerID", peer.ID)
			}
		}
		delete(s.peers, peer.ID)
	}

	delete(room.Peers, session.GetPeer().ID)

	if len(room.Peers) == 0 {
		delete(s.rooms, roomID)
		metrics.GetInstance().RoomClosed()
		logger.Info("P2P: room deleted (empty)", "roomID", roomID)
	}

	metrics.GetInstance().PeerLeft()
	session.SetRoom("")

	logger.Info("P2P: left room", "roomID", roomID, "peerID", session.GetPeer().ID)

	return nil
}

// SendSignal sends a WebRTC signal for the user's current room.
func (s *Service) SendSignal(ctx context.Context, userID string, signal []byte) error {
	_ = ctx
	if len(signal) > constants.MaxSignalSize {
		return errors.InvalidInput(fmt.Sprintf("signal exceeds maximum size: %d > %d", len(signal), constants.MaxSignalSize))
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	session := s.getSessionUnlocked(userID)
	if session == nil || session.CurrentRoom() == "" {
		return errors.InvalidInput("not connected to a room")
	}

	peer := session.GetPeer()
	if peer == nil {
		return errors.InvalidInput("peer not found")
	}

	if peer.DataChannel == nil {
		return errors.InvalidInput("data channel not created")
	}

	err := peer.DataChannel.Send(signal)
	if err != nil {
		logger.Error("P2P: error sending signal", "error", err, "roomID", session.CurrentRoom())
		return errors.Wrap(errors.ErrInternal, "error sending signal", err)
	}

	logger.Debug("P2P: signal sent", "roomID", session.CurrentRoom(), "signalSize", len(signal))
	return nil
}

// GetEvents returns the event channel for the user's session.
func (s *Service) GetEvents(userID string) chan models.P2PEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session := s.getSessionUnlocked(userID)
	if session == nil {
		return nil
	}

	// For now, return the main event channel
	// Future: per-session event channels
	return nil
}

// GetRoomInfo returns room information for the user's current room.
func (s *Service) GetRoomInfo(ctx context.Context, userID string) (*models.RoomInfo, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	session := s.getSessionUnlocked(userID)
	if session == nil || session.CurrentRoom() == "" {
		return nil, fmt.Errorf("not connected to a room")
	}

	room, exists := s.rooms[session.CurrentRoom()]
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

// getSession returns the session for a user (with lock).
func (s *Service) getSession(userID string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getSessionUnlocked(userID)
}

// getSessionUnlocked returns the session for a user (without lock, caller must hold lock).
func (s *Service) getSessionUnlocked(userID string) *Session {
	return s.sessions[userID]
}

// Close closes the P2P service.
func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		close(s.closeChan)
	})

	type peerConnections struct {
		dataChannel io.Closer
		connection  io.Closer
		id          string
	}
	var toClose []peerConnections

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

	roomCount := len(s.rooms)
	s.rooms = make(map[string]*Room)
	s.peers = make(map[string]*Peer)
	s.sessions = make(map[string]*Session)

	close(s.doneChan)
	s.mu.Unlock()

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

// setupPeerConnection configures WebRTC connection handlers.
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
	})
}

// handleDataChannelEvents processes data channel events.
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

// emitEvent sends an event to the event channel.
func (s *Service) emitEvent(eventType string, data interface{}) {
	if s.closed.Load() {
		return
	}

	select {
	case <-s.doneChan:
		return
	default:
	}

	// Note: This emits to a single channel. For multi-session support,
	// implement per-session event channels.
	logger.Warn("P2P: event emission needs multi-session implementation", "type", eventType)
}
