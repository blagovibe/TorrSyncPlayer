// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
)

// Transport is what a Room talks through: the mesh in production, channels in
// tests. Rendezvous fails for the weather — CPU saturation, empty routing
// tables, ghost provider records — and room logic must not.
type Transport interface {
	Send(ctx context.Context, msg any) error
	Next(ctx context.Context) (from string, msg any, ok bool)
	Peers() []string
	SelfID() string
	Close() error
}

// Peer is one participant as the room sees it. There is no server roster
// anymore; the roster is whoever said hello and has not said bye, or gone
// quiet.
type Peer struct {
	PeerID    string
	UserID    string
	Username  string
	IsHost    bool
	Buffering bool
}

// Callbacks are the room's effects on the world. The driver decides; the
// caller acts — players, widgets and backends stay out of this package.
type Callbacks struct {
	// OnRoster fires on every roster change with the whole roster.
	OnRoster func(peers []Peer)
	// OnMagnet fires when the host names what the room is watching.
	OnMagnet func(m Magnet)
	// OnPause fires on the host's play state. Pause=false is play.
	OnPause func(paused bool)
	// OnSeek fires on the host's raw seek target. Smoothing is the caller's:
	// only the follower knows its own lag (see SmoothSeek).
	OnSeek func(seconds float64)
	// OnBuffering fires when any peer reports its own stall or recovery.
	OnBuffering func(peerID string, buffering bool)
	// OnHostChanged fires when the host role moves, with the new host's peer ID.
	OnHostChanged func(hostPeerID string)
}

// Room drives one room on top of a Transport.
//
// It owns the roster, the host role, and the magnet the room is watching. Play,
// pause and seek arrive as callbacks; sending goes through Play/Pause/Seek/
// ReportBuffering/AnnounceMagnet/TransferHost. Nothing here touches a player,
// a widget, or a backend.
type Room struct {
	transport Transport
	selfID    string
	userID    string
	username  string
	isHost    bool
	cb        Callbacks

	mu       sync.RWMutex
	peers    map[string]*Peer
	hostPeer string
	magnet   *Magnet
}

// NewRoom builds a room driver. isHost seeds the role the local peer claims in
// its hellos: true for the peer that created the room, false for joiners.
func NewRoom(transport Transport, selfID, userID string, isHost bool, cb *Callbacks) *Room {
	var callbacks Callbacks
	if cb != nil {
		callbacks = *cb
	}
	r := &Room{
		transport: transport,
		selfID:    selfID,
		userID:    userID,
		cb:        callbacks,
		peers:     make(map[string]*Peer),
		isHost:    isHost,
	}
	if isHost {
		r.hostPeer = selfID
	}
	return r
}

// IsHost reports whether this peer currently holds the host role.
func (r *Room) IsHost() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isHost
}

// HostPeer is the peer ID currently believed to hold the host role, or empty
// when nobody does yet.
func (r *Room) HostPeer() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hostPeer
}

// Peers returns the roster as the room currently sees it.
func (r *Room) Peers() []Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	peers := make([]Peer, 0, len(r.peers))
	for _, p := range r.peers {
		peers = append(peers, *p)
	}
	return peers
}

// Run pumps transport messages into the driver until ctx ends. One message at
// a time, in arrival order: the mesh delivers each peer's stream ordered, and
// interleaving two peers' state updates would invent orders that never
// happened.
func (r *Room) Run(ctx context.Context) {
	for {
		from, msg, ok := r.transport.Next(ctx)
		if !ok {
			return
		}
		r.handle(from, msg)
	}
}

// Play broadcasts a start. Only the host's commands change anything downstream,
// but every participant sends through the same call so that "I pressed play"
// and "the room is playing" do not become different beliefs.
func (r *Room) Play(ctx context.Context) error {
	return r.transport.Send(ctx, NewPause(false))
}

// Pause broadcasts a stop.
func (r *Room) Pause(ctx context.Context) error {
	return r.transport.Send(ctx, NewPause(true))
}

// Seek broadcasts a raw seek target in seconds. Raw, not smoothed: smoothing
// needs the follower's lag and the host does not have it.
func (r *Room) Seek(ctx context.Context, seconds float64) error {
	return r.transport.Send(ctx, NewPosition(seconds))
}

// ReportBuffering tells the room this peer stalled, or caught up. Any peer
// may, not only the host: otherwise a participant whose stream recovered could
// not say so, and the room would stay paused for everyone.
func (r *Room) ReportBuffering(ctx context.Context, buffering bool) error {
	return r.transport.Send(ctx, NewBuffering(buffering))
}

// AnnounceMagnet names what the room is watching, remembers it for late
// joiners, and broadcasts it. Late joiners learn the film from the next hello
// they trigger, not from waiting: a magnet that only travels once strands
// everyone who joins afterwards.
func (r *Room) AnnounceMagnet(ctx context.Context, m Magnet) error {
	r.mu.Lock()
	cp := m
	r.magnet = &cp
	r.mu.Unlock()
	return r.transport.Send(ctx, NewMagnet(m))
}

func (r *Room) handle(from string, msg any) {
	switch m := msg.(type) {
	case Hello:
		r.onHello(from, m)
	case Magnet:
		r.onMagnet(from, m)
	case Pause:
		if r.shouldFollow(from) {
			if r.cb.OnPause != nil {
				r.cb.OnPause(m.Pause)
			}
		}
	case Position:
		if r.shouldFollow(from) {
			if r.cb.OnSeek != nil {
				r.cb.OnSeek(m.Position)
			}
		}
	case Buffering:
		r.mu.Lock()
		if p, ok := r.peers[from]; ok {
			p.Buffering = m.Buffering
		}
		r.mu.Unlock()
		if r.cb.OnBuffering != nil {
			r.cb.OnBuffering(from, m.Buffering)
		}
	case Host:
		r.onHost(from, m)
	case Bye:
		r.onBye(from, m)
	}
}

func (r *Room) onHello(from string, m Hello) {
	r.mu.Lock()
	r.peers[from] = &Peer{PeerID: from, UserID: m.UserID, Username: m.Username, IsHost: m.IsHost}
	if m.IsHost {
		r.hostPeer = from
	}
	magnet := r.magnet
	amHost := r.isHost
	r.mu.Unlock()

	r.emitRoster()
	if m.IsHost {
		r.emitHostChanged(from)
	}

	// A host answers presence with state: the newcomer missed every broadcast
	// so far, and waiting for the next one strands it until someone acts.
	if amHost {
		ctx := context.Background()
		if magnet != nil {
			_ = r.transport.Send(ctx, NewMagnet(*magnet))
		}
		_ = r.transport.Send(ctx, NewHost(r.selfID))
	}
}

func (r *Room) onMagnet(from string, m Magnet) {
	r.mu.Lock()
	if r.hostPeer == "" {
		// No host known yet: the first magnet binds the sender as host. The
		// alternative is ignoring the film until someone claims the role, and
		// a room that names a film but nobody to follow is a room that cannot
		// start.
		r.hostPeer = from
	}
	follow := from == r.hostPeer
	cp := m
	r.magnet = &cp
	r.mu.Unlock()

	if follow && r.cb.OnMagnet != nil {
		r.cb.OnMagnet(m)
	}
}

func (r *Room) onHost(from string, m Host) {
	r.mu.RLock()
	current := r.hostPeer
	r.mu.RUnlock()

	believable := false
	if m.PeerID == from {
		// Self-announcement: believable only from the takeover winner. Anyone
		// else naming themselves is noise.
		believable = from == r.takeoverWinner()
	} else if from == current {
		// Transfer: believable for being sent by the current host.
		believable = true
	}
	if !believable {
		return
	}

	r.mu.Lock()
	r.hostPeer = m.PeerID
	if p, ok := r.peers[m.PeerID]; ok {
		p.IsHost = true
	}
	for id, p := range r.peers {
		if id != m.PeerID {
			p.IsHost = false
		}
	}
	r.isHost = m.PeerID == r.selfID
	r.mu.Unlock()

	r.emitRoster()
	r.emitHostChanged(m.PeerID)
}

func (r *Room) onBye(from string, _ Bye) {
	r.mu.Lock()
	_, known := r.peers[from]
	delete(r.peers, from)
	wasHost := from == r.hostPeer
	if wasHost {
		r.hostPeer = ""
	}
	r.mu.Unlock()

	if known {
		r.emitRoster()
	}
	if !wasHost {
		return
	}

	// The host is gone without a transfer. The smallest living ID takes over —
	// deterministic, no election, no chatter — and announces itself, so every
	// remaining peer converges on the same new host from the same peer set.
	winner := r.takeoverWinner()
	r.mu.Lock()
	r.hostPeer = winner
	if winner == r.selfID {
		r.isHost = true
	}
	r.mu.Unlock()

	if winner == r.selfID {
		_ = r.transport.Send(context.Background(), NewHost(winner))
	}
	r.emitRoster()
	r.emitHostChanged(winner)
}

// takeoverWinner runs the host-leave rule over the currently known peers plus
// this peer itself: the smallest ID among the living.
func (r *Room) takeoverWinner() string {
	r.mu.RLock()
	ids := make([]string, 0, len(r.peers)+1)
	ids = append(ids, r.selfID)
	for id := range r.peers {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	return TakeoverHost(ids)
}

func (r *Room) shouldFollow(from string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return ShouldFollow(from, r.hostPeer)
}

func (r *Room) emitRoster() {
	if r.cb.OnRoster == nil {
		return
	}
	r.cb.OnRoster(r.Peers())
}

func (r *Room) emitHostChanged(hostPeerID string) {
	if r.cb.OnHostChanged != nil {
		r.cb.OnHostChanged(hostPeerID)
	}
}

// MaxSeekDrift is the biggest position change between two polls that still
// counts as drift rather than a seek. The host's broadcast loop polls twice a
// second, so drift is half a second; anything past this is a seek someone made
// and travels the room. It matches the smoothing boundary on purpose: a jump
// the follower would take whole is not worth announcing either.
const MaxSeekDrift = 2.0

// SmoothSeek is where a follower actually goes after a seek.
//
// The host sends the raw target; the follower moves the whole way for small
// jumps and partway for big ones, so a lagging follower does not teleport past
// everyone else. The numbers are the server's old ones (2 seconds, 30%),
// kept because the perceived behavior is already tuned to them, not because
// the server that computed them still exists.
func SmoothSeek(current, requested float64) float64 {
	diff := requested - current
	if diff < 0 {
		diff = -diff
	}
	if diff <= MaxSeekDrift {
		return requested
	}
	return current + (requested-current)*smoothRatio
}

// smoothRatio is how far a smoothed jump moves: partway, not teleporting.
const smoothRatio = 0.3

// Invite is a room's address: the community to join and the password and key
// pair that admits. The host generates all three and copies them as one
// string; the guest pastes it back.
type Invite struct {
	Community string
	Password  string
	Key       string
}

// Encode renders the invite as community:password:key.
func (in Invite) Encode() string {
	return in.Community + ":" + in.Password + ":" + in.Key
}

// ParseInviteCode parses community:password:key. Colons separate the parts, so
// none of the parts may contain one — and a magnet is not an invite.
func ParseInviteCode(code string) (Invite, error) {
	parts := strings.Split(code, ":")
	if len(parts) != 3 {
		return Invite{}, fmt.Errorf("rtc: an invite is community:password:key, got %d parts", len(parts))
	}
	for _, part := range parts {
		if part == "" {
			return Invite{}, fmt.Errorf("rtc: an invite part is empty")
		}
	}
	return Invite{Community: parts[0], Password: parts[1], Key: parts[2]}, nil
}

// GenerateInvite makes a fresh room address. Short random strings from an
// alphabet without colons, so every generated code parses back.
func GenerateInvite() Invite {
	return Invite{
		Community: randomPart(9),
		Password:  randomPart(9),
		Key:       randomPart(16),
	}
}

const inviteAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomPart(n int) string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("rtc: no randomness for invite generation: " + err.Error())
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = inviteAlphabet[int(buf[i])%len(inviteAlphabet)]
	}
	return string(out)
}
