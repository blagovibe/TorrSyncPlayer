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
	"net/url"
	"sync"
	"time"

	"github.com/pojntfx/weron/pkg/wrtcconn"
)

// channelSync is the data channel everything in a room travels on. One channel,
// not one per message kind: the envelope routes, and a second channel would be
// a second connection to watch die.
const channelSync = "torrsync/sync"

// Mesh is a room without a server: peers find each other through a weron
// signaler and exchange the protocol messages over WebRTC data channels.
//
// The signaler only introduces peers that share the community *and* the
// password, and it encrypts the introduction with the key. The channel itself
// runs under DTLS. What this wrapper owns is the mesh on top: accepting peers,
// reading each one's stream, broadcasting sends, and keeping the roster that
// the server used to hand out.
type Mesh struct {
	opts MeshOptions

	mu            sync.RWMutex
	adapter       *wrtcconn.Adapter
	adapterCancel context.CancelFunc
	selfID        string
	isHost        bool
	peers         map[string]*meshPeer

	incoming  chan incomingMessage
	closed    chan struct{}
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
	// Signaler is the weron signaler URL without credentials, e.g.
	// wss://weron.up.railway.app/ or ws://127.0.0.1:1337/ in tests.
	Signaler string
	// Community is the room. Only peers naming it meet each other.
	Community string
	// Password admits peers to the community. A wrong one joins nothing.
	Password string
	// Key encrypts the signaling. The data channel itself runs under DTLS.
	Key string
}

// NewMesh builds an unjoined mesh peer.
func NewMesh(opts MeshOptions) *Mesh {
	return &Mesh{
		opts:     opts,
		peers:    make(map[string]*meshPeer),
		incoming: make(chan incomingMessage, 64),
		closed:   make(chan struct{}),
	}
}

// Join introduces this peer to the community and starts accepting others.
//
// isHost records whether this peer holds the host role. It is announced in the
// hello every peer sends on connect, so the mesh agrees on who drives playback
// without asking anyone — there is no one left to ask.
func (m *Mesh) Join(ctx context.Context, isHost bool) error {
	u, err := url.Parse(m.opts.Signaler)
	if err != nil {
		return fmt.Errorf("rtc: parse signaler URL: %w", err)
	}
	q := u.Query()
	q.Set("community", m.opts.Community)
	q.Set("password", m.opts.Password)
	u.RawQuery = q.Encode()

	adapterCtx, cancel := context.WithCancel(context.Background())

	adapter := wrtcconn.NewAdapter(
		u.String(),
		m.opts.Key,
		[]string{},
		[]string{channelSync},
		&wrtcconn.AdapterConfig{Timeout: 10 * time.Second},
		adapterCtx,
	)

	ids, err := adapter.Open()
	if err != nil {
		cancel()
		return fmt.Errorf("rtc: open adapter: %w", err)
	}

	select {
	case id := <-ids:
		m.mu.Lock()
		m.adapter = adapter
		m.adapterCancel = cancel
		m.selfID = id
		m.isHost = isHost
		m.mu.Unlock()
	case <-ctx.Done():
		cancel()
		_ = adapter.Close()
		return fmt.Errorf("rtc: no ID from the signaler: %w", ctx.Err())
	case <-time.After(30 * time.Second):
		cancel()
		_ = adapter.Close()
		return fmt.Errorf("rtc: no ID from the signaler in time")
	}

	go m.acceptLoop()
	return nil
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
// data channels are full-duplex streams, and two goroutines writing one JSON
// document each would interleave them into neither.
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

func messageType(msg any) string {
	switch m := msg.(type) {
	case Magnet:
		return m.Type
	case Pause:
		return m.Type
	case Position:
		return m.Type
	case Buffering:
		return m.Type
	case Hello:
		return m.Type
	case Host:
		return m.Type
	case Bye:
		return m.Type
	default:
		return ""
	}
}

// Close leaves the mesh.
//
// A bye goes first so the roster updates at once rather than when the silence
// is noticed — but it is best-effort and bounded: the write blocks until the
// far end reads, and by leave time nobody may be reading anymore. An unbounded
// bye turned Close into a hang whenever the room had stopped listening. After
// the grace the peer connections close, which drops us from every remote roster
// promptly either way, and unblocks the read loops.
func (m *Mesh) Close() error {
	var err error
	m.closeOnce.Do(func() {
		close(m.closed)
		m.mu.RLock()
		adapter := m.adapter
		cancel := m.adapterCancel
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
		if adapter != nil {
			err = adapter.Close()
		}
		if cancel != nil {
			cancel()
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

func (m *Mesh) acceptLoop() {
	m.mu.RLock()
	adapter := m.adapter
	m.mu.RUnlock()
	if adapter == nil {
		return
	}

	for {
		select {
		case <-m.closed:
			return
		case peer, ok := <-adapter.Accept():
			if !ok {
				return
			}
			m.addPeer(peer)
		}
	}
}

func (m *Mesh) addPeer(peer *wrtcconn.Peer) {
	m.mu.Lock()
	if _, exists := m.peers[peer.PeerID]; exists {
		m.mu.Unlock()
		return
	}
	mp := &meshPeer{id: peer.PeerID, conn: peer.Conn, enc: json.NewEncoder(peer.Conn)}
	m.peers[peer.PeerID] = mp
	selfID := m.selfID
	isHost := m.isHost
	m.mu.Unlock()

	go m.readLoop(peer, mp)

	// Announce presence at once: the roster is whoever said hello, and a peer
	// that waits to be asked is a roster entry that never appears.
	_ = m.sendTo(mp, NewHello(selfID, "", "", isHost))
}

func (m *Mesh) sendTo(mp *meshPeer, msg any) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	return mp.enc.Encode(msg)
}

func (m *Mesh) readLoop(peer *wrtcconn.Peer, mp *meshPeer) {
	defer func() {
		m.mu.Lock()
		delete(m.peers, peer.PeerID)
		m.mu.Unlock()
	}()

	// Line framing, not a streaming decoder, for two transports at once. A data
	// channel is message-oriented — one read returns one sender's write — while
	// a pipe is a byte stream; what both honor is the newline every Encode ends
	// with (JSON escapes newlines inside strings, so a document never contains
	// a raw one). A streaming decoder over the channel is the alternative that
	// was tried: its first refill reads 512 bytes, and a queued message bigger
	// than that fails the whole read with io.ErrShortBuffer instead of blocking
	// for the rest — dropping the peer over nothing. The buffer here fits any
	// message this protocol speaks; anything bigger is a violation, not a peer.
	reader := bufio.NewReaderSize(peer.Conn, 64*1024)
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
		case m.incoming <- incomingMessage{from: peer.PeerID, msg: msg}:
		case <-m.closed:
			return
		}
	}
}
