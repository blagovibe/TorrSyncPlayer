// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package p2p provides the room and real-time event service for synchronized
// playback. Rooms and peer-to-peer synchronization are brokered by the server:
// all events (peer roster, room signals, playback sync commands) are
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
	"sort"
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
	// JoinedAt tracks when this peer joined the room. Used for deterministic
	// host transfer: the peer that joined earliest becomes the new host.
	JoinedAt time.Time
	// Buffering is what this peer last reported through POST /sync/buffering.
	// It lives on the peer rather than only on the room's SyncStatus because the
	// roster has to say WHO is buffering, and the room keeps a single shared
	// status that cannot answer that (idea-0017).
	Buffering bool
}

// Room represents a room for synchronized playback.
type Room struct {
	// RosterSeq rises on every change to Peers. Clients compare it to decide
	// which of two roster snapshots is newer — necessary because /rooms/info and
	// the peer_list event race by construction (idea-0017).
	RosterSeq int64

	// SyncSeq is the sequence number for sync events in this room.
	// Incremented on every sync event emission. Used for replay/ack.
	SyncSeq int64

	// EventHistory stores recent events for replay (bug-0040).
	// Keyed by sequence number. Max size limited by EventHistoryMaxSize.
	EventHistory map[int64]*models.P2PEvent

	ID         string
	Name       string
	TorrentID  string // infohash of the torrent bound to this room (idea-0046)
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
	mu          sync.RWMutex

	// emitMu serialises sends on eventChan against closing it. A send and a
	// close of the same channel must never overlap: the panic that results is
	// recoverable, but the race underneath it is not, and a recover() that
	// hides it makes the behaviour undefined rather than safe (bug-0058).
	emitMu    sync.RWMutex
	emitDone  bool
	eventChan chan models.P2PEvent // per-session event channel for SSE
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

// sendEvent delivers one event without blocking. It reports false when the
// event was dropped, either because the session is shutting down or because the
// consumer is not keeping up.
//
// Holding emitMu across the send is what makes this safe: closeEvents takes the
// same lock for writing, so the two can never overlap on the channel.
func (s *Session) sendEvent(event models.P2PEvent) bool {
	if s.eventChan == nil {
		return false
	}

	s.emitMu.RLock()
	defer s.emitMu.RUnlock()

	if s.emitDone {
		return false
	}

	select {
	case s.eventChan <- event:
		return true
	default:
		return false
	}
}

// closeEvents closes the event channel exactly once. Idempotent, because the
// channel is closed both when a session leaves its last room and again if the
// service shuts down in the same instant.
func (s *Session) closeEvents() {
	if s.eventChan == nil {
		return
	}

	s.emitMu.Lock()
	defer s.emitMu.Unlock()

	if s.emitDone {
		return
	}
	s.emitDone = true
	close(s.eventChan)
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
// their roster. Also cleans up expired sessions (bug-0041).
// Safe to call concurrently; acquires the lock internally.
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
		// Still check for expired sessions even if no idle peers
		s.pruneExpiredSessions()
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
		room.RosterSeq++
		peerCount := len(room.Peers)
		s.mu.Unlock()

		metrics.GetInstance().PeerLeft()
		s.emitEvent(p.roomID, models.EventTypePeerLeft, models.PeerLeftEvent{
			PeerID:    p.peer.ID,
			UserID:    p.peer.UserID,
			PeerCount: peerCount,
		})
		logger.Info("P2P: pruned idle peer", "roomID", p.roomID, "peerID", p.peer.ID)
		// The roster changed, so everyone else is holding a stale copy.
		s.BroadcastPeerList(p.roomID)
	}

	// Also clean up expired sessions
	s.pruneExpiredSessions()
}

// pruneExpiredSessions removes sessions that have been idle for too long
// (no room, no heartbeat). This prevents unbounded memory growth from
// abandoned sessions (bug-0041).
func (s *Service) pruneExpiredSessions() {
	if s.closed.Load() {
		return
	}

	// Sessions without a room and with old heartbeat are candidates for removal
	cutoff := time.Now().Add(-constants.SessionIdleTimeout)

	s.mu.Lock()
	defer s.mu.Unlock()

	for userID, session := range s.sessions {
		if session.CurrentRoom() != "" {
			continue // still in a room
		}
		peer := session.GetPeer()
		if peer == nil || peer.LastHeartbeat.Before(cutoff) {
			session.closeEvents()
			delete(s.sessions, userID)
			logger.Info("P2P: pruned expired session", "userID", userID)
		}
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
// scheduleSave asks the debouncer to persist room state shortly after the
// current burst of mutations settles.
//
// It deliberately does NOT take s.mu. Callers invoke it from inside critical
// sections (CreateRoom calls it while holding s.mu), and re-acquiring that
// non-reentrant mutex here deadlocked the whole service: the goroutine waited
// on a lock it already owned, and every other goroutine queued behind it. Room
// creation then hung forever on any server with persistence enabled, which is
// how cmd/server runs — only the tests leave persistence nil, so nothing
// exercised this path.
//
// No extra synchronisation is needed here. Debouncer has its own mutex and
// Trigger is safe to call concurrently, and it runs the flush callback
// out-of-line in its own goroutine; flushRooms takes s.mu.RLock() itself, so it
// simply waits for an in-flight writer to release the lock.
func (s *Service) scheduleSave() {
	if s.persistence == nil {
		return
	}
	s.saveDebouncer.Trigger()
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

	// Bug-0041: enforce max sessions limit to prevent unbounded memory growth
	// from arbitrary X-Client-IDs.
	if len(s.sessions) >= constants.MaxSessions {
		// Find and remove the oldest idle session to make room
		s.removeOldestIdleSession()
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

// removeOldestIdleSession removes the session with the oldest LastHeartbeat.
// Caller must hold s.mu.
func (s *Service) removeOldestIdleSession() {
	var oldestUserID string
	var oldestTime time.Time
	for userID, session := range s.sessions {
		peer := session.GetPeer()
		if peer == nil {
			// Sessions without peers are candidates for removal
			oldestUserID = userID
			break
		}
		if oldestUserID == "" || peer.LastHeartbeat.Before(oldestTime) {
			oldestUserID = userID
			oldestTime = peer.LastHeartbeat
		}
	}
	if oldestUserID != "" {
		if session, exists := s.sessions[oldestUserID]; exists {
			session.closeEvents()
			delete(s.sessions, oldestUserID)
			logger.Warn("P2P: removed oldest idle session to enforce max sessions limit", "userID", oldestUserID)
		}
	}
}

// CreateRoom creates a new room for the specified user.
// Now takes userID as parameter to isolate session state.
// torrentID is required (idea-0046: each room is bound to a torrent).
func (s *Service) CreateRoom(ctx context.Context, userID, name, password, torrentID string) (*models.RoomInfo, error) {
	_ = ctx
	session := s.getOrCreateSession(userID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.rooms) >= maxRooms {
		logger.Warn("P2P: room limit exceeded", "max", maxRooms)
		return nil, errors.New(errors.ErrUnavailable, "maximum number of rooms exceeded")
	}

	if torrentID == "" {
		logger.Warn("P2P: room creation without torrentID rejected", "userID", userID)
		return nil, errors.New(errors.ErrInvalidInput, "torrentID is required")
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

	// ensureSession leaves Peer nil if the ID could not be generated. HostID has
	// always dereferenced it, so a nil peer was a panic waiting to happen; now
	// that the peer is also stored in the roster, handle it explicitly.
	peer := session.GetPeer()
	if peer == nil {
		s.mu.Unlock()
		logger.Error("P2P: cannot create room without a peer", "roomID", roomID, "userID", userID)
		return nil, fmt.Errorf("cannot create room without a peer")
	}

	room := &Room{
		ID:         roomID,
		Name:       name,
		TorrentID:  torrentID,
		HostID:     peer.ID,
		HostUserID: userID,
		Password:   passwordHash,
		// The host is a participant and belongs in the roster (bug-0021). It
		// already was one everywhere else: it passes the membership check, it
		// drives playback, and emitEvent adds it to the recipient list by hand.
		// Leaving it out of Peers made every count one short and, worse, made a
		// room look empty the moment the last guest left — which deleted the
		// room out from under a host who was still connected and watching.
		Peers:     map[string]*Peer{peer.ID: peer},
		CreatedAt: time.Now(),
		RosterSeq: 1,
	}
	s.peers[peer.ID] = peer

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
		ID:         room.ID,
		Name:       room.Name,
		TorrentID:  room.TorrentID,
		HostID:     room.HostID,
		HostUserID: room.HostUserID,
		PeerCount:  len(room.Peers),
		RosterSeq:  room.RosterSeq,
	}, nil
}

// JoinRoom joins a room for the specified user.
// Now takes userID as parameter to isolate session state.
// torrentID is required and must match the room's torrent (idea-0046).
func (s *Service) JoinRoom(ctx context.Context, userID, roomID, password, torrentID string) error {
	_ = ctx
	session := s.getOrCreateSession(userID)

	s.mu.Lock()

	room, exists := s.rooms[roomID]
	if !exists {
		s.mu.Unlock()
		logger.Warn("P2P: room not found", "roomID", roomID)
		return errors.NotFound("room", roomID)
	}

	// Validate torrentID matches (idea-0046)
	if room.TorrentID != "" && torrentID != room.TorrentID {
		s.mu.Unlock()
		logger.Warn("P2P: torrentID mismatch on join", "roomID", roomID, "expected", room.TorrentID, "got", torrentID)
		return errors.New(errors.ErrInvalidInput, "torrentID does not match room")
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
	now := time.Now()
	peer.LastHeartbeat = now
	peer.JoinedAt = now

	room.Peers[peer.ID] = peer
	s.peers[peer.ID] = peer
	room.RosterSeq++
	peerCount := len(room.Peers)

	s.mu.Unlock()

	metrics.GetInstance().PeerJoined()

	// Notify other participants that a peer joined (used by the client to
	// build a peer roster / "out of sync" indicators).
	// Emitted after releasing the lock: emitEvent re-acquires the lock and
	// would deadlock if called while the write lock is held.
	s.emitEvent(roomID, models.EventTypePeerJoined, models.PeerJoinedEvent{
		PeerID:    peer.ID,
		UserID:    userID,
		IsHost:    room.HostUserID == userID,
		PeerCount: peerCount,
	})

	// A client that joined late never saw the peer_joined events of whoever was
	// already here, so the incremental events alone leave it permanently out of
	// date. The snapshot closes that (idea-0017).
	s.BroadcastPeerList(roomID)

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
	room.RosterSeq++

	// Host transfer (bug-0050): if the leaving user was the host, promote
	// the first remaining peer to host (earliest JoinedAt for determinism).
	var newHostUserID string
	if room.HostUserID == userID && len(room.Peers) > 0 {
		var earliestJoined time.Time
		for _, p := range room.Peers {
			if p != nil && (earliestJoined.IsZero() || p.JoinedAt.Before(earliestJoined)) {
				earliestJoined = p.JoinedAt
				newHostUserID = p.UserID
			}
		}
		room.HostUserID = newHostUserID
		logger.Info("P2P: host transferred", "roomID", roomID, "from", userID, "to", newHostUserID)
	}

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
		s.emitEvent(roomID, models.EventTypePeerLeft, models.PeerLeftEvent{
			PeerID:    leftPeerID,
			UserID:    userID,
			PeerCount: peerCount - 1,
		})

		// If host changed, broadcast updated peer list so clients learn new host.
		if newHostUserID != "" {
			s.BroadcastPeerList(roomID)
		}
	}

	metrics.GetInstance().PeerLeft()
	session.SetRoom("")

	// Bug-0041: clean up session if user has no other rooms.
	// Currently each user only has one room, so we can remove the session entirely.
	// This prevents unbounded memory growth from abandoned sessions.
	s.mu.Lock()
	if sess, exists := s.sessions[userID]; exists {
		if sess.CurrentRoom() == "" {
			sess.closeEvents()
			delete(s.sessions, userID)
		}
	}
	s.mu.Unlock()

	// Only worth broadcasting if the room still exists: when the last peer left
	// it was deleted, and there is nobody to tell.
	if !roomDeleted {
		s.BroadcastPeerList(roomID)
	}

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
		ID:         room.ID,
		Name:       room.Name,
		TorrentID:  room.TorrentID,
		HostID:     room.HostID,
		HostUserID: room.HostUserID,
		PeerCount:  len(room.Peers),
		Peers:      buildRoster(room),
		RosterSeq:  room.RosterSeq,
	}, nil
}

// buildRoster renders a room's peers for the wire.
//
// The caller must hold at least the read lock: room.Peers is mutated by Join,
// Leave and idle pruning, and iterating it unlocked risks a fatal concurrent
// map access.
//
// Order is by peer ID. Map iteration order is deliberately not used: it is
// randomised per run, so an unordered roster would make every peer_list event
// look different from the last even when nothing changed, and the client would
// repaint a list that had not moved.
func buildRoster(room *Room) []models.PeerInfo {
	peers := make([]models.PeerInfo, 0, len(room.Peers))
	for _, peer := range room.Peers {
		peers = append(peers, models.PeerInfo{
			PeerID:    peer.ID,
			UserID:    peer.UserID,
			Username:  peer.Username,
			IsHost:    peer.UserID == room.HostUserID,
			Buffering: peer.Buffering,
		})
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].PeerID < peers[j].PeerID })
	return peers
}

// BroadcastPeerList sends the current roster to everyone in the room.
//
// It is the second half of idea-0017's delivery: GET /rooms/info carries the
// roster for a client that just connected, and this carries it for everyone
// after any change. Both are full snapshots, so a client may receive either one
// and be correct — and may receive the same information twice without harm.
func (s *Service) BroadcastPeerList(roomID string) {
	if s.closed.Load() {
		return
	}

	s.mu.RLock()
	room, exists := s.rooms[roomID]
	if !exists {
		s.mu.RUnlock()
		return
	}
	peers := buildRoster(room)
	seq := room.RosterSeq
	s.mu.RUnlock()

	s.emitEvent(roomID, models.EventTypePeerList, models.PeerListEvent{Peers: peers, RosterSeq: seq})
}

// SetPeerBuffering records that a peer reported itself as buffering or caught
// up, so the roster can name it.
//
// A user with no room is a no-op: reporting must not create membership.
func (s *Service) SetPeerBuffering(_ context.Context, userID string, buffering bool) {
	session := s.getSession(userID)
	if session == nil {
		return
	}
	roomID := session.CurrentRoom()
	if roomID == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	room, exists := s.rooms[roomID]
	if !exists {
		return
	}

	// Bug-0042: guard against nil peer (e.g., GenerateID failure).
	peerObj := session.GetPeer()
	if peerObj == nil {
		return
	}
	if peer, present := room.Peers[peerObj.ID]; present {
		if peer.Buffering != buffering {
			peer.Buffering = buffering
			room.RosterSeq++
		}
	}
}

// IsHost checks if the given user is the host of the specified room.
func (s *Service) IsHost(ctx context.Context, userID, roomID string) (bool, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, exists := s.rooms[roomID]
	if !exists {
		return false, fmt.Errorf("room not found")
	}

	return room.HostUserID == userID, nil
}

// roomHasUser reports whether userID is in the room's roster.
//
// room.Peers is keyed by peer.ID, which is per-connection: a peer that
// reconnects gets a new one while keeping its userID. Matching the map key
// directly would therefore miss live members.
func roomHasUser(room *Room, userID string) bool {
	if room.HostUserID == userID {
		return true
	}
	for _, peer := range room.Peers {
		if peer != nil && peer.UserID == userID {
			return true
		}
	}
	return false
}

// TorrentsVisibleTo reports which torrents a peer may read: the torrent bound
// to the room it currently participates in (bug-0049, layer 2).
//
// The access token is per-process, so "presents the token" cannot distinguish a
// room guest from the machine's owner. Without this, GET /torrents handed a
// guest the host's entire library — names, sizes, progress.
//
// Only the CURRENT room counts. A peer belongs to at most one room at a time,
// so leaving revokes the visibility together with the membership — no separate
// expiry to get wrong. An unknown peer gets an empty set: absence of a session
// must never read as permission.
//
// The host is included by construction: CreateRoom puts it in room.Peers, so it
// keeps access to its own room's torrent.
func (s *Service) TorrentsVisibleTo(ctx context.Context, userID string) map[string]bool {
	_ = ctx

	if userID == "" {
		return map[string]bool{}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	visible := make(map[string]bool)

	session := s.sessions[userID]
	if session == nil {
		return visible
	}

	room := s.rooms[session.CurrentRoom()]
	if room == nil {
		return visible
	}

	// Membership is checked against the roster, not merely the session: a
	// session can outlive its room (disconnect, reaped peer), and a stale
	// session must not keep the door open.
	//
	// room.Peers is keyed by peer.ID, not userID — a peer reconnects with a
	// fresh local id — so membership is matched on Peer.UserID.
	if !roomHasUser(room, userID) {
		return visible
	}

	// A room with no bound torrent would otherwise contribute an empty key,
	// which would then match a caller passing an empty torrentID.
	if room.TorrentID == "" {
		return visible
	}

	visible[room.TorrentID] = true
	return visible
}

// TouchPeer records that the user is still connected, by refreshing the
// heartbeat the idle pruner reads.
//
// LastHeartbeat used to be written only by JoinRoom, which made PeerIdleTimeout
// a measure of "has not joined lately" rather than "is not connected". Every
// peer — host included — was reaped five minutes in, while its SSE stream was
// still open for thirty. The host never calls join at all, so putting it in the
// roster (bug-0021) would have handed it a guaranteed reap.
//
// The SSE stream is the honest liveness signal: a peer that is holding a stream
// open is by definition present. RoomEvents therefore touches on subscribe and
// on every ping, which keeps the timeout meaningful — a peer whose stream died
// stops being touched and is reaped as before.
//
// A user with no room is a no-op: touching must never confer membership.
func (s *Service) TouchPeer(ctx context.Context, userID string) error {
	_ = ctx
	session := s.getSession(userID)
	if session == nil {
		return nil
	}

	roomID := session.CurrentRoom()
	if roomID == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rooms[roomID]; !exists {
		return nil
	}
	if peer := session.GetPeer(); peer != nil {
		peer.LastHeartbeat = time.Now()
	}
	return nil
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

// Close closes the P2P service. It is safe to call more than once and from
// multiple goroutines: every destructive step — including close(doneChan) —
// runs inside closeOnce. Previously only the ticker and closeChan were
// guarded, so a second call reached close(s.doneChan) and panicked with
// "close of closed channel".
func (s *Service) Close() error {
	var roomCount int

	s.closeOnce.Do(func() {
		if s.pruneTicker != nil {
			s.pruneTicker.Stop()
		}
		close(s.closeChan)

		// Persist room metadata before dropping in-memory state.
		s.saveDebouncer.Stop()
		s.flushRooms()

		s.mu.Lock()
		s.closed.Store(true)

		// Close all session event channels. Close every one while holding the
		// write lock: doing it without the lock let a concurrent JoinRoom
		// append a session we then missed, leaving its SSE connection open.
		for _, session := range s.sessions {
			session.closeEvents()
		}

		roomCount = len(s.rooms)
		s.rooms = make(map[string]*Room)
		s.peers = make(map[string]*Peer)
		s.sessions = make(map[string]*Session)

		close(s.doneChan)
		s.mu.Unlock()
	})

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
// Returns immediately when the session has no live SSE connection, the
// service is closing, or the channel is full (non-blocking send).
func (s *Service) sendToSession(session *Session, userID string, event models.P2PEvent, eventType string) {
	if session == nil {
		return
	}
	if s.closed.Load() {
		return
	}

	// sendEvent takes the session's emit lock, so a concurrent closeEvents
	// cannot overlap the send. The previous version guarded this with a
	// recover() instead, which suppressed the panic but left the race in place
	// (bug-0058).
	if !session.sendEvent(event) {
		if !s.closed.Load() {
			metrics.GetInstance().EventChannelFull()
			logger.Warn("P2P: event channel full for user", "userID", userID, "eventType", eventType)
		}
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

	// Assign sequence number and store in history for sync events (bug-0040).
	var seq int64
	if eventType == models.EventTypeSync {
		s.mu.Lock()
		room, exists := s.rooms[roomID]
		if !exists {
			s.mu.Unlock()
			return
		}
		room.SyncSeq++
		seq = room.SyncSeq

		// Initialize event history if needed
		if room.EventHistory == nil {
			room.EventHistory = make(map[int64]*models.P2PEvent)
		}
		s.mu.Unlock()
	}

	event := models.P2PEvent{Type: eventType, Data: data, Seq: seq}

	// Store in history for replay (bug-0040)
	if seq > 0 {
		s.mu.Lock()
		room, exists := s.rooms[roomID]
		if exists {
			room.EventHistory[seq] = &event
			// Trim history if too large
			if len(room.EventHistory) > constants.EventHistoryMaxSize {
				// Remove oldest entries
				var oldestSeq int64
				for s := range room.EventHistory {
					if oldestSeq == 0 || s < oldestSeq {
						oldestSeq = s
					}
				}
				delete(room.EventHistory, oldestSeq)
			}
		}
		s.mu.Unlock()
	}

	// Snapshot the recipients while holding the read lock, then send outside it.
	//
	// room.Peers is mutated by Join, Leave and peer pruning. Ranging over it
	// without the lock made a concurrent write during iteration possible, and
	// Go's runtime raises "concurrent map iteration and map write" as a fatal
	// error that recover() cannot catch — it takes the whole process down.
	//
	// Copying first keeps the critical section short and avoids holding the
	// lock across delivery, so no lock-ordering question can arise.
	s.mu.RLock()
	room, exists := s.rooms[roomID]
	if !exists {
		s.mu.RUnlock()
		return
	}
	recipients := make([]string, 0, len(room.Peers)+1)
	seen := make(map[string]struct{}, len(room.Peers)+1)
	for _, peer := range room.Peers {
		recipients = append(recipients, peer.UserID)
		seen[peer.UserID] = struct{}{}
	}
	// The host is a participant too. Deduplicating by user id means the host
	// receives the event exactly once, whether or not it is also in room.Peers.
	hostUserID := room.HostUserID
	if hostUserID != "" {
		if _, dup := seen[hostUserID]; !dup {
			recipients = append(recipients, hostUserID)
		}
	}
	peerCount := len(room.Peers)
	s.mu.RUnlock()

	for _, userID := range recipients {
		s.mu.RLock()
		session := s.sessions[userID]
		s.mu.RUnlock()
		s.sendToSession(session, userID, event, eventType)
	}

	logger.Debug("P2P: event emitted to room", "roomID", roomID, "eventType", eventType, "peerCount", peerCount, "seq", seq)
}

// BroadcastSync sends a sync event (play/pause/seek/buffering) to all
// participants in a room.
//
// The parameter is a models.SyncEvent rather than interface{}: the four callers
// in the sync handlers used to build map[string]interface{} literals, and nothing
// stopped one of them from omitting the "initiator" key or misspelling an action.
// The broadcast is a relay, so the payload is carried through untouched.
func (s *Service) BroadcastSync(roomID string, event models.SyncEvent) {
	s.emitEvent(roomID, models.EventTypeSync, event)
}

// ReplayEvents returns events from the room's history starting from afterSeq.
// Used by clients that reconnected or detected a gap in sequence numbers.
// Returns empty slice if room doesn't exist or afterSeq is beyond history.
// (bug-0040)
func (s *Service) ReplayEvents(roomID string, afterSeq int64) []*models.P2PEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, exists := s.rooms[roomID]
	if !exists || room.EventHistory == nil {
		return nil
	}

	var result []*models.P2PEvent
	for seq, event := range room.EventHistory {
		if seq > afterSeq {
			result = append(result, event)
		}
	}
	// Sort by sequence number (map iteration is random)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Seq > result[j].Seq {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// GetSyncSeq returns the current sync sequence number for a room.
// (bug-0040)
func (s *Service) GetSyncSeq(roomID string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	room, exists := s.rooms[roomID]
	if !exists {
		return 0, false
	}
	return room.SyncSeq, true
}
