// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// rediscoverInterval re-runs DHT discovery for late joiners: presence is
// announced once, but nobody is owed an introduction at a particular moment.
const rediscoverInterval = 15 * time.Second

// Mesh is a room without a server: peers find each other over the DHT
// rendezvous and exchange the protocol messages over libp2p streams.
//
// Streams are admitted by the invite password and key pair (see p2p.go);
// what this wrapper owns is the mesh on top:
// accepting peers, reading each one's stream, broadcasting sends, and
// keeping the roster that the server used to hand out.
type Mesh struct {
	opts MeshOptions

	mu       sync.RWMutex
	node     *Node
	selfID   string
	isHost   bool
	peers    map[string]*meshPeer
	dialing  map[string]struct{}
	incoming chan incomingMessage
	closed   chan struct{}

	closeOnce sync.Once
}

type incomingMessage struct {
	from string
	msg  any
}

type meshPeer struct {
	id   string
	conn io.ReadWriteCloser
	enc  *json.Encoder
	mu   sync.Mutex
}

// MeshOptions configures one peer's place in the mesh.
type MeshOptions struct {
	// Community is the room. Only peers naming it meet each other.
	Community string
	// Password admits peers to the room. A wrong one joins nothing.
	Password string
	// Key gates the swarm and encrypts the signaling.
	Key string
	// Bootstrap is DHT rendezvous as comma-separated multiaddrs with /p2p/.
	// Empty means the public defaults.
	Bootstrap string
	// Relays are the owner's fallback relay candidates, same shape as
	// Bootstrap. Empty means DHT discovery only.
	Relays string
	// UserID is who this peer is, as the room will show it. It travels in
	// the presence greeting, because that greeting is the only thing a peer
	// hears about anyone: without it the roster is a list of nameless rows
	// and a person cannot tell who they are watching with (bug-0119).
	UserID string
	// Username is the name to show, when there is one. Empty means the user
	// id is the label.
	Username string
}

// NewMesh builds an unjoined mesh peer.
func NewMesh(opts MeshOptions) *Mesh {
	return &Mesh{
		opts:     opts,
		peers:    make(map[string]*meshPeer),
		dialing:  make(map[string]struct{}),
		incoming: make(chan incomingMessage, 64),
		closed:   make(chan struct{}),
	}
}

// Join publishes this peer's presence and starts meeting the room: accepting
// inbound streams and dialing discovered ones. It returns once the presence
// is published; the peers knowing each other takes discovery on top, so a
// send before convergence lands nowhere — convergence is part of joining
// here, not an afterthought in each caller.
func (m *Mesh) Join(ctx context.Context, isHost bool) error {
	bootstrap, err := resolveBootstrap(m.opts.Bootstrap)
	if err != nil {
		return err
	}
	relays, err := resolveRelays(m.opts.Relays)
	if err != nil {
		return err
	}
	node, err := NewNode(NodeOptions{Invite: Invite{
		Community: m.opts.Community,
		Password:  m.opts.Password,
		Key:       m.opts.Key,
	}, EnableRelay: true, StaticRelays: relays})
	if err != nil {
		return err
	}
	if err := node.Bootstrap(ctx, bootstrap); err != nil {
		_ = node.Close()
		return err
	}
	if err := node.WaitReady(ctx); err != nil {
		_ = node.Close()
		return err
	}
	if err := node.Announce(ctx); err != nil {
		_ = node.Close()
		return err
	}

	m.mu.Lock()
	m.node = node
	m.selfID = node.ID().String()
	m.isHost = isHost
	m.mu.Unlock()

	go m.acceptLoop()
	go m.discoverLoop()
	return nil
}

// resolveBootstrap parses the bootstrap setting or falls back to the public
// defaults. Every address must carry its /p2p/ peer ID: an address without
// one cannot be dialed, and a setting that looks configured but is not is
// worse than an empty one.
func resolveBootstrap(setting string) ([]peer.AddrInfo, error) {
	if setting == "" {
		return defaultBootstrap(), nil
	}
	seen := map[peer.ID]struct{}{}
	var out []peer.AddrInfo
	for _, part := range splitAddrs(setting) {
		addr, err := multiaddr.NewMultiaddr(part)
		if err != nil {
			return nil, fmt.Errorf("rtc: bad bootstrap address %q: %w", part, err)
		}
		info, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return nil, fmt.Errorf("rtc: bootstrap address %q has no /p2p/ peer ID", part)
		}
		if _, dup := seen[info.ID]; dup {
			continue
		}
		seen[info.ID] = struct{}{}
		out = append(out, *info)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("rtc: no bootstrap addresses")
	}
	return out, nil
}

// resolveRelays parses the relays setting. Empty means DHT discovery only —
// unlike bootstrap there are no public defaults to fall back to, so an empty
// setting is valid and common. Anything listed must be dialable.
func resolveRelays(setting string) ([]peer.AddrInfo, error) {
	if setting == "" {
		return nil, nil
	}
	seen := map[peer.ID]struct{}{}
	var out []peer.AddrInfo
	for _, part := range splitAddrs(setting) {
		addr, err := multiaddr.NewMultiaddr(part)
		if err != nil {
			return nil, fmt.Errorf("rtc: bad relay address %q: %w", part, err)
		}
		info, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return nil, fmt.Errorf("rtc: relay address %q has no /p2p/ peer ID", part)
		}
		if _, dup := seen[info.ID]; dup {
			continue
		}
		seen[info.ID] = struct{}{}
		out = append(out, *info)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("rtc: no relay addresses")
	}
	return out, nil
}

func splitAddrs(setting string) []string {
	var out []string
	for _, part := range strings.Split(setting, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// SelfID is this peer's ID in the mesh.
func (m *Mesh) SelfID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.selfID
}

// IsHost reports whether this peer holds the host role.
func (m *Mesh) IsHost() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isHost
}

// Peers lists the connected peer IDs.
func (m *Mesh) Peers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.peers))
	for id := range m.peers {
		ids = append(ids, id)
	}
	return ids
}

// Send broadcasts one protocol message to every connected peer.
//
// The message is encoded once and written per peer under that peer's own lock:
// streams are byte-oriented, and two goroutines writing one JSON document
// each would interleave them into neither.
func (m *Mesh) Send(ctx context.Context, msg any) error {
	m.mu.RLock()
	peers := make([]*meshPeer, 0, len(m.peers))
	for _, p := range m.peers {
		peers = append(peers, p)
	}
	m.mu.RUnlock()

	for _, p := range peers {
		p.mu.Lock()
		err := p.enc.Encode(msg)
		p.mu.Unlock()
		if err != nil {
			return fmt.Errorf("rtc: send to %s: %w", p.id, err)
		}
	}
	return nil
}

// Next blocks until any message arrives, returning its sender with it.
// It is what a room driver pumps: one message at a time, in arrival order.
func (m *Mesh) Next(ctx context.Context) (string, any, bool) {
	select {
	case in := <-m.incoming:
		return in.from, in.msg, true
	case <-ctx.Done():
		return "", nil, false
	case <-m.closed:
		return "", nil, false
	}
}

// WaitFor blocks until a message of the given type arrives, or nil on timeout,
// closed mesh, or done context. Messages of other types stay queued for later
// WaitFor calls.
func (m *Mesh) WaitFor(ctx context.Context, msgType string, timeout time.Duration) any {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case in := <-m.incoming:
			if messageType(in.msg) == msgType {
				return in.msg
			}
		case <-ctx.Done():
			return nil
		case <-m.closed:
			return nil
		case <-time.After(time.Until(deadline)):
			return nil
		}
	}
}

// Close leaves the mesh.
//
// A bye goes first so the roster updates at once rather than when the silence
// is noticed — but it is best-effort and bounded: the write blocks until the
// far end reads, and by leave time nobody may be reading anymore. An unbounded
// bye turned Close into a hang whenever the room had stopped listening. After
// the grace the streams close, which drops us from every remote roster
// promptly either way, and unblocks the read loops.
func (m *Mesh) Close() error {
	var err error
	m.closeOnce.Do(func() {
		close(m.closed)
		m.mu.RLock()
		node := m.node
		selfID := m.selfID
		peers := make([]*meshPeer, 0, len(m.peers))
		for _, p := range m.peers {
			peers = append(peers, p)
		}
		m.mu.RUnlock()

		if selfID != "" {
			done := make(chan struct{})
			go func() {
				defer close(done)
				_ = m.broadcast(NewBye(selfID))
			}()
			select {
			case <-done:
			case <-time.After(byeGracePeriod):
			}
		}
		for _, p := range peers {
			_ = p.conn.Close()
		}
		if node != nil {
			err = node.Close()
		}
	})
	return err
}

// byeGracePeriod bounds the farewell: long enough for any working connection,
// short enough that leaving never freezes the room behind it.
const byeGracePeriod = time.Second

func (m *Mesh) broadcast(msg any) error {
	m.mu.RLock()
	peers := make([]*meshPeer, 0, len(m.peers))
	for _, p := range m.peers {
		peers = append(peers, p)
	}
	m.mu.RUnlock()

	for _, p := range peers {
		p.mu.Lock()
		err := p.enc.Encode(msg)
		p.mu.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

// acceptLoop admits inbound streams: the handshake already passed inside the
// node, so everyone arriving here proved the password.
func (m *Mesh) acceptLoop() {
	m.mu.RLock()
	node := m.node
	m.mu.RUnlock()
	if node == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-m.closed
		cancel()
	}()

	for {
		s, err := node.WaitAdmitted(ctx)
		if err != nil {
			return
		}
		remote := s.Conn().RemotePeer().String()
		m.mu.RLock()
		self := m.selfID
		m.mu.RUnlock()
		if remote == "" || remote == self {
			_ = s.Close()
			continue
		}
		m.addPeer(remote, s)
	}
}

// discoverLoop dials discovered peers so late joiners converge without any
// introducer: presence is announced once, discovery runs for the session.
// One flight per peer: a dial already in flight is not started twice, and a
// failed dial frees the peer for the next round.
func (m *Mesh) discoverLoop() {
	m.mu.RLock()
	node := m.node
	m.mu.RUnlock()
	if node == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-m.closed
		cancel()
	}()

	discover := func() {
		ch, err := node.Discover(ctx)
		if err != nil {
			return
		}
		for info := range ch {
			if info.ID == node.ID() {
				continue
			}
			id := info.ID.String()
			m.mu.Lock()
			_, known := m.peers[id]
			_, flying := m.dialing[id]
			if !known && !flying {
				m.dialing[id] = struct{}{}
			} else {
				m.mu.Unlock()
				continue
			}
			m.mu.Unlock()

			go func(info peer.AddrInfo) {
				defer func() {
					m.mu.Lock()
					delete(m.dialing, id)
					m.mu.Unlock()
				}()
				dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
				defer dialCancel()
				s, err := node.DialRoom(dialCtx, info)
				if err != nil {
					return
				}
				m.addPeer(id, s)
			}(info)
		}
	}

	discover()
	ticker := time.NewTicker(rediscoverInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.closed:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			discover()
		}
	}
}

// addPeer plugs a connected peer into the mesh. The first stream for an ID
// wins and later ones close: dials and admissions race each other, and two
// streams to one peer would only waste connections — sends are keyed by ID,
// so either survivor carries the same traffic.
func (m *Mesh) addPeer(id string, conn io.ReadWriteCloser) {
	m.mu.Lock()
	if _, exists := m.peers[id]; exists {
		m.mu.Unlock()
		_ = conn.Close()
		return
	}
	mp := &meshPeer{id: id, conn: conn, enc: json.NewEncoder(conn)}
	m.peers[id] = mp
	selfID := m.selfID
	isHost := m.isHost
	m.mu.Unlock()

	go m.readLoop(mp)

	// Announce presence at once: the roster is whoever said hello, and a peer
	// that waits to be asked is a roster entry that never appears.
	_ = m.sendTo(mp, NewHello(selfID, m.opts.UserID, m.opts.Username, isHost))
}

func (m *Mesh) sendTo(mp *meshPeer, msg any) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	return mp.enc.Encode(msg)
}

func (m *Mesh) readLoop(mp *meshPeer) {
	defer func() {
		m.mu.Lock()
		if m.peers[mp.id] == mp {
			delete(m.peers, mp.id)
		}
		m.mu.Unlock()

		// The room learns of departures only from a bye. A stream that ends
		// on its own — a crash, a kill, a cut cable — therefore left the peer
		// in every roster forever, and a host that vanished that way never
		// handed over (bug-0106). The farewell the departed side would have
		// sent is manufactured here, on the side still listening. It is
		// dropped rather than waited for: a room that has stopped reading is
		// gone, and blocking on it would turn a departed peer into a stuck
		// goroutine.
		select {
		case m.incoming <- incomingMessage{from: mp.id, msg: NewBye(mp.id)}:
		default:
		}
	}()

	// Line framing, one JSON document per line: streams are byte-oriented,
	// and a queued message is never split across reads. JSON escapes newlines
	// inside strings, so a document never contains a raw one. The buffer here
	// fits any message this protocol speaks; anything bigger is a violation,
	// not a peer.
	reader := bufio.NewReaderSize(mp.conn, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		msg, err := Parse(line)
		if err != nil {
			continue
		}
		select {
		case m.incoming <- incomingMessage{from: mp.id, msg: msg}:
		case <-m.closed:
			return
		}
	}
}
