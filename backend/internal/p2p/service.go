// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package p2p provides the room and real-time event service for synchronized
// playback. Rooms and peer-to-peer synchronization are brokered by the server:
// all events (peer roster, WebRTC-style signals, playback sync commands) are
// relayed over Server-Sent Events (SSE) to per-user session channels. The
// service is intentionally server-centric — there is no direct peer-to-peer
// data path; clients connect to the backend over REST (commands) and SSE
// (events). This keeps the topology simple, NAT-friendly and debuggable.
//
// Manages rooms, peers and events in a thread-safe manner.
// Architecture: Multi-session - each user has isolated state (fixes C2).
package p2p

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/metrics"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/persistence"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/utils"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

const (
	eventChannelSize       = constants.P2PEventChannelSize
	peerIDLength           = constants.PeerIDLength
	maxRooms               = constants.MaxRooms
	p2pCloseTimeoutDefault = constants.P2PCloseTimeoutDefault
)

// Peer represents a participant in a room. Connectivity is fully brokered by
// the server over SSE; there is no direct peer-to-peer data channel.
type Peer struct {
	ID            string
	UserID        string
	Username      string
	LastHeartbeat time.Time
}

// Room represents a room for synchronized playback.
type Room struct {
	ID         string
	Name       string
	HostID     string
	HostUserID string
	Password   string
	Peers      map[string]*Peer
	CreatedAt  time.Time
}

// Session represents a user's P2P session with isolated state.
// This solves the global state problem (C2) where currentRoom/localPeerID
// were shared across all users.
type Session struct {
	id          string
	userID      string
	currentRoom string
	peer        *Peer
	eventChan   chan models.P2PEvent // per-session event channel for SSE
	mu          sync.RWMutex
}

// NewSession creates a new user session.
func NewSession(userID string) *Session {
	return &Session{
		id:        fmt.Sprintf("session_%s", userID),
		userID:    userID,
		eventChan: make(chan models.P2PEvent, eventChannelSize),
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

// GetEventChan returns the event channel for the session.
func (s *Session) GetEventChan() chan models.P2PEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.eventChan
}

// Service manages room and event operations with multi-session support.
// All real-time delivery is brokered by the server over SSE.
type Service struct {
	mu            sync.RWMutex
	rooms         map[string]*Room
	peers         map[string]*Peer
	sessions      map[string]*Session // userID -> session mapping
	closeChan     chan struct{}
	doneChan      chan struct{}
	closeOnce     sync.Once
	wg            sync.WaitGroup
	authService   *auth.AuthService
	closed        atomic.Bool
	persistence   *persistence.Store
	saveDebouncer *utils.Debouncer
	pruneTicker   *time.Ticker
}

// NewService creates a new room/event service with multi-session support.
// Real-time delivery is brokered by the server over SSE; no direct
// peer-to-peer data path is established.
func NewService(authService *auth.AuthService) (*Service, error) {
	service := &Service{
		rooms:       make(map[string]*Room),
		peers:       make(map[string]*Peer),
		sessions:    make(map[string]*Session),
		closeChan:   make(chan struct{}),
		doneChan:    make(chan struct{}),
		authService: authService,
	}
	service.saveDebouncer = utils.NewDebouncer(constants.P2PDebounceInterval, service.flushRooms)

	// Periodically prune peers that stopped sending heartbeats so idle
	// sessions don't accumulate in long-lived rooms.
	service.pruneTicker = time.NewTicker(constants.PeerPruneInterval)
	service.wg.Add(1)
	go service.pruneLoop()

	logger.Info("P2P service initialized (server-brokered SSE transport)")
	return service, nil
}

// pruneLoop runs the idle-peer sweep until the service is closed.
func (s *Service) pruneLoop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.closeChan:
			return
		case <-s.pruneTicker.C:
			s.pruneIdlePeers()
		}
	}
}

// pruneIdlePeers removes peers whose last heartbeat is older than
// PeerIdleTimeout, emitting a peer_left event for each so clients can update
// their roster. Safe to call concurrently; acquires the lock internally.
func (s *Service) pruneIdlePeers() {
	if s.closed.Load() {
		return
	}

	cutoff := time.Now().Add(-constants.PeerIdleTimeout)

	type pruned struct {
		roomID string
		peer   *Peer
	}

	s.mu.RLock()
	var stale []pruned
	for roomID, room := range s.rooms {
		for _, peer := range room.Peers {
			if peer.LastHeartbeat.Before(cutoff) {
				stale = append(stale, pruned{roomID: roomID, peer: peer})
			}
		}
	}
	s.mu.RUnlock()

	if len(stale) == 0 {
		return
	}

	for _, p := range stale {
		s.mu.Lock()
		room, exists := s.rooms[p.roomID]
		if !exists {
			s.mu.Unlock()
			continue
		}
		if _, stillThere := room.Peers[p.peer.ID]; !stillThere {
			s.mu.Unlock()
			continue
		}
		delete(s.peers, p.peer.ID)
		delete(room.Peers, p.peer.ID)
		peerCount := len(room.Peers)
		s.mu.Unlock()

		metrics.GetInstance().PeerLeft()
		s.emitEvent(p.roomID, "peer_left", map[string]interface{}{
			"peerID":    p.peer.ID,
			"userID":    p.peer.UserID,
			"peerCount": peerCount,
		})
		logger.Info("P2P: pruned idle peer", "roomID", p.roomID, "peerID", p.peer.ID)
	}
}

// SetPersistence enables JSON-file persistence of room metadata. When set,
// rooms survive a server restart (clients must still rejoin; only the room
// definition and host are restored). Without it, rooms are in-memory only.
func (s *Service) SetPersistence(store *persistence.Store) {
	s.mu.Lock()
	s.persistence = store
	s.mu.Unlock()

	if data, err := store.LoadRooms(); err == nil {
		s.mu.Lock()
		for _, snap := range data.Rooms {
			s.rooms[snap.ID] = &Room{
				ID:         snap.ID,
				Name:       snap.Name,
				HostID:     snap.HostID,
				HostUserID: snap.HostUserID,
				Password:   snap.Password,
				Peers:      make(map[string]*Peer),
				CreatedAt:  time.Unix(snap.CreatedAt, 0),
			}
		}
		s.mu.Unlock()
		logger.Info("P2P: restored rooms from persistence", "count", len(data.Rooms))
	} else {
		logger.Warn("P2P: failed to load persisted rooms", "error", err)
	}
}

// scheduleSave persists room metadata on a debounce timer to avoid
// hammering disk on every room mutation.
func (s *Service) scheduleSave() {
	if s.persistence == nil {
		return
	}
	s.mu.Lock()
	s.saveDebouncer.Trigger()
	s.mu.Unlock()
}

// flushRooms writes the current room metadata to disk.
func (s *Service) flushRooms() {
	s.mu.RLock()
	data := &persistence.RoomsData{Rooms: make(map[string]*persistence.RoomSnapshot, len(s.rooms))}
	for id, room := range s.rooms {
		data.Rooms[id] = &persistence.RoomSnapshot{
			ID:         room.ID,
			Name:       room.Name,
			HostID:     room.HostID,
			HostUserID: room.HostUserID,
			Password:   room.Password,
			CreatedAt:  room.CreatedAt.Unix(),
		}
	}
	store := s.persistence
	s.mu.RUnlock()

	if store == nil {
		return
	}
	if err := store.SaveRooms(data); err != nil {
		logger.Warn("P2P: failed to persist rooms", "error", err)
	}
}

// getOrCreateSession returns or creates a session for the user.
func (s *Service) getOrCreateSession(userID string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, exists := s.sessions[userID]; exists {
		if session.GetPeer() == nil {
			peerID, err := utils.GenerateID(peerIDLength)
			if err != nil {
				logger.Error("P2P: failed to generate peer ID on reconnect", "error", err, "userID", userID)
			} else {
				session.SetPeer(&Peer{
					ID:            peerID,
					UserID:        userID,
					LastHeartbeat: time.Now(),
				})
			}
		}
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
		ID:         roomID,
		Name:       name,
		HostID:     session.GetPeer().ID,
		HostUserID: userID,
		Password:   passwordHash,
		Peers:      make(map[string]*Peer),
		CreatedAt:  time.Now(),
	}

	s.rooms[roomID] = room
	session.SetRoom(roomID)
	s.scheduleSave()

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

	room, exists := s.rooms[roomID]
	if !exists {
		s.mu.Unlock()
		logger.Warn("P2P: room not found", "roomID", roomID)
		return errors.NotFound("room", roomID)
	}

	if room.Password != "" {
		if err := bcrypt.CompareHashAndPassword([]byte(room.Password), []byte(password)); err != nil {
			s.mu.Unlock()
			logger.Warn("P2P: invalid room password", "roomID", roomID)
			return errors.Unauthorized("invalid password")
		}
	}

	session.SetRoom(roomID)

	peer := session.GetPeer()
	peer.LastHeartbeat = time.Now()

	room.Peers[peer.ID] = peer
	s.peers[peer.ID] = peer
	peerCount := len(room.Peers)

	s.mu.Unlock()

	metrics.GetInstance().PeerJoined()

	// Notify other participants that a peer joined (used by the client to
	// build a peer roster / "out of sync" indicators).
	// Emitted after releasing the lock: emitEvent re-acquires the lock and
	// would deadlock if called while the write lock is held.
	s.emitEvent(roomID, "peer_joined", map[string]interface{}{
		"peerID":    peer.ID,
		"userID":    userID,
		"isHost":    room.HostUserID == userID,
		"peerCount": peerCount,
	})

	logger.Info("P2P: joined room",
		"roomID", roomID,
		"peerID", peer.ID,
		"userID", userID,
	)

	return nil
}

// LeaveRoom leaves the current room for the specified user.
// Now takes userID as parameter to isolate session state.
func (s *Service) LeaveRoom(ctx context.Context, userID string) error {
	_ = ctx
	session := s.getSession(userID)

	s.mu.Lock()

	if session == nil || session.CurrentRoom() == "" {
		s.mu.Unlock()
		return errors.New(errors.ErrInvalidInput, "not connected to a room")
	}

	roomID := session.CurrentRoom()
	room, exists := s.rooms[roomID]
	if !exists {
		s.mu.Unlock()
		logger.Warn("P2P: room not found on leave", "roomID", roomID)
		return errors.NotFound("room", roomID)
	}

	peer := session.GetPeer()
	var leftPeerID string
	if peer != nil {
		leftPeerID = peer.ID
		delete(s.peers, peer.ID)
	}

	peerCount := len(room.Peers)
	delete(room.Peers, leftPeerID)

	roomDeleted := false
	if len(room.Peers) == 0 {
		delete(s.rooms, roomID)
		roomDeleted = true
	}

	s.mu.Unlock()

	if roomDeleted {
		metrics.GetInstance().RoomClosed()
		logger.Info("P2P: room deleted (empty)", "roomID", roomID)
	} else {
		// Notify remaining participants that a peer left.
		// Emitted after releasing the lock (emitEvent re-acquires it).
		s.emitEvent(roomID, "peer_left", map[string]interface{}{
			"peerID":    leftPeerID,
			"userID":    userID,
			"peerCount": peerCount - 1,
		})
	}

	metrics.GetInstance().PeerLeft()
	session.SetRoom("")

	logger.Info("P2P: left room", "roomID", roomID, "peerID", leftPeerID)

	return nil
}

// SendSignal relays a sync signal (an opaque JSON payload exchanged between
// clients for out-of-band negotiation) to the other participants of the
// sender's current room. Signals are delivered over the existing SSE event
// stream (each peer's per-session event channel), exactly like sync playback
// events are broadcast. The sender is included in the fan-out but the client
// ignores its own signals. The transport is fully server-brokered.
func (s *Service) SendSignal(ctx context.Context, userID string, signal []byte) error {
	_ = ctx
	if len(signal) > constants.MaxSignalSize {
		return errors.InvalidInput(fmt.Sprintf("signal exceeds maximum size: %d > %d", len(signal), constants.MaxSignalSize))
	}

	s.mu.RLock()
	session := s.getSessionUnlocked(userID)
	if session == nil || session.CurrentRoom() == "" {
		s.mu.RUnlock()
		return errors.InvalidInput("not connected to a room")
	}
	roomID := session.CurrentRoom()
	s.mu.RUnlock()

	// Relay the raw signal bytes to all other peers in the room via SSE.
	// emitEvent fans the event out to every peer (and the host) in the room.
	s.emitEvent(roomID, "signal", signal)

	logger.Debug("P2P: signal relayed", "roomID", roomID, "signalSize", len(signal), "fromUser", userID)
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

	return session.GetEventChan()
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
		if s.pruneTicker != nil {
			s.pruneTicker.Stop()
		}
		close(s.closeChan)
	})

	// Persist room metadata before dropping in-memory state.
	s.saveDebouncer.Stop()
	s.flushRooms()

	s.mu.Lock()
	s.closed.Store(true)

	// Close all session event channels
	for _, session := range s.sessions {
		if session.eventChan != nil {
			go func(ch chan models.P2PEvent) {
				defer func() {
					if r := recover(); r != nil {
						logger.Debug("P2P: event channel already closed")
					}
				}()
				close(ch)
			}(session.eventChan)
		}
	}

	roomCount := len(s.rooms)
	s.rooms = make(map[string]*Room)
	s.peers = make(map[string]*Peer)
	s.sessions = make(map[string]*Session)

	close(s.doneChan)
	s.mu.Unlock()

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

// sendToSession delivers an event to a single session's event channel.
// Returns immediately when the session has no live SSE connection or the
// channel is full (non-blocking send).
func (s *Service) sendToSession(session *Session, userID string, event models.P2PEvent, eventType string) {
	if session == nil || session.eventChan == nil {
		return
	}
	select {
	case session.eventChan <- event:
	default:
		metrics.GetInstance().EventChannelFull()
		logger.Warn("P2P: event channel full for user", "userID", userID, "eventType", eventType)
	}
}

// emitEvent sends an event to all participants in the specified room.
// Events are sent to per-session event channels for SSE streaming.
func (s *Service) emitEvent(roomID string, eventType string, data interface{}) {
	if s.closed.Load() {
		return
	}

	select {
	case <-s.doneChan:
		return
	default:
	}

	s.mu.RLock()
	room, exists := s.rooms[roomID]
	s.mu.RUnlock()

	if !exists {
		return
	}

	event := models.P2PEvent{Type: eventType, Data: data}

	// Send event to all peers in the room
	for _, peer := range room.Peers {
		s.mu.RLock()
		session := s.sessions[peer.UserID]
		s.mu.RUnlock()
		s.sendToSession(session, peer.UserID, event, eventType)
	}

	// Also send to host if different from peers
	if room.HostUserID != "" {
		s.mu.RLock()
		hostSession := s.sessions[room.HostUserID]
		s.mu.RUnlock()

		if hostSession == nil || hostSession.eventChan == nil {
			// Host has no live SSE session; nothing to deliver.
			// Skip silently — client will re-subscribe on (re)connect.
		} else if hostPeer := hostSession.GetPeer(); hostPeer != nil {
			_, isPeer := room.Peers[hostPeer.ID]
			if !isPeer {
				s.sendToSession(hostSession, room.HostUserID, event, eventType)
			}
		}
	}

	logger.Debug("P2P: event emitted to room", "roomID", roomID, "eventType", eventType, "peerCount", len(room.Peers))
}

// BroadcastSync sends a sync event (play/pause/seek) to all participants in a room.
// Used by SyncService to propagate playback commands across P2P network.
func (s *Service) BroadcastSync(roomID string, syncData interface{}) {
	s.emitEvent(roomID, "sync", syncData)
}
