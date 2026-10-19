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
//
// Five seconds, measured: on a loopback testbed two participants meet in
// 0.2s when discovery finds them on the first query, and in one whole interval
// when it does not — which is most of the time, because both announce in the
// same second and neither is in the other's provider store yet. At fifteen
// seconds a room spent most of its life waiting for a discovery tick, and the
// gap is exactly what a person waiting for a guest experiences as silence.
// The cost is one provider lookup per tick per peer, which is what the lookup
// is for.
const rediscoverInterval = 5 * time.Second

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
	// wake asks the discovery loop to look again right now. The tick is a
	// floor, not a schedule: a room that becomes findable in the second
	// between two ticks must not wait out the interval.
	wake chan struct{}
	// inviteFailed says the address in the invite code did not work.
	inviteFailed bool

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
	// via is how this node reached the peer: by the address in the invite
	// code, by the fallback's lookup of its id, or by the peer dialling us.
	via PeerPath
	// initiator is the peer whose dial opened this stream: this node for a
	// dial we made, the remote peer for one we admitted. Two participants
	// discover each other in the same second and therefore dial each other in
	// the same second, so a mesh regularly ends up with two streams to one
	// peer — and the rule for which one survives has to be one both ends can
	// compute without talking.
	initiator string
	mu        sync.Mutex
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
	// Addrs are the host's addresses, from the invite code. With them the
	// room meets without a rendezvous, in seconds; without them the room can
	// only be met through the DHT.
	Addrs []string
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
		wake:     make(chan struct{}, 1),
	}
}

// Join publishes this peer's presence and starts meeting the room: accepting
// inbound streams and dialing discovered ones. It returns once the node is on
// the rendezvous; the peers knowing each other takes discovery on top, so a
// send before convergence lands nowhere — convergence is part of joining here,
// not an afterthought in each caller.
//
// Two ways to meet, and the fast one does not wait for anything. When the
// invite carries the host's addresses, the peer dials them directly and the
// rendezvous comes up behind it: the DHT is the fallback for an address that
// did not work, and the only way the node learns its own public address. When
// the code names nobody, the rendezvous is the only way to meet, and the room
// waits for it the way it always has — by looking again every few seconds.
//
// A failed rendezvous does not fail the join. The room is open either way, and
// the node remembers whether it announced itself for the window to show.
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

	invited, inviteErr := addrInfoFromAddrs(m.opts.Addrs)

	m.mu.Lock()
	m.node = node
	m.selfID = node.ID().String()
	m.isHost = isHost
	m.mu.Unlock()

	go m.acceptLoop()
	go m.discoverLoop()

	// The node is up, and the room is open. The rendezvous is the fallback —
	// the way a peer that has no address finds this one — and it comes up
	// behind the join: a room whose code named its peer does not wait for a
	// network to be ready, and one whose code named nobody waits for it the
	// same way, by looking again every few seconds. A rendezvous that does
	// not answer no longer refuses the entry: the room is open either way,
	// and the window says what state the network is in (bug-0121, bug-0137).
	go m.startRendezvous(node, bootstrap)

	if inviteErr == nil {
		// The code named the peer: the room meets by a direct dial, and
		// without waiting for anything.
		go m.dialInvited(node, invited)
	}
	return nil
}

// startRendezvous brings the fallback path up: the DHT, then the announce that
// puts the room where a peer without an address can find it. It runs behind
// the join, on the node's own lifetime — the caller's context dies with the
// join, and a rendezvous that dies with it is no rendezvous at all.
func (m *Mesh) startRendezvous(node *Node, bootstrap []peer.AddrInfo) {
	ctx := node.Lifetime()
	if err := node.Bootstrap(ctx, bootstrap); err != nil {
		return
	}
	if err := node.WaitReady(ctx); err != nil {
		return
	}
	// The routing table is up: the peer the code named can be found by its id
	// now, and the room's own network can grow.
	m.wakeDiscovery()
}

// dialInvited dials the peer the invite code named. This is the primary
// path: no rendezvous, no discovery, seconds.
//
// A failed dial is remembered: the room is then on the lookup by id alone,
// and the window says so rather than waiting in silence.
func (m *Mesh) dialInvited(node *Node, info peer.AddrInfo) {
	ctx, cancel := context.WithTimeout(node.Lifetime(), 30*time.Second)
	defer cancel()
	s, err := node.DialRoom(ctx, info)
	if err != nil {
		m.markInviteFailed()
		return
	}
	m.addPeer(info.ID.String(), s, m.selfID, PathInvite)
}

// markInviteFailed remembers that the address in the code did not work. The
// room is then on the fallback path alone, and the window says so rather than
// waiting in silence.
func (m *Mesh) markInviteFailed() {
	m.mu.Lock()
	m.inviteFailed = true
	m.mu.Unlock()
}

// DialHint reaches the peers the room's address book named. It is the mesh
// growing into the mesh the goal promises: a newcomer is told where the
// others are and dials them itself, so the room is not a star around the
// host (bug-0136).
func (m *Mesh) DialHint(peers []PeerAddr) {
	m.mu.RLock()
	node := m.node
	m.mu.RUnlock()
	if node == nil {
		return
	}
	for _, p := range peers {
		info, err := addrInfoFromAddrs(p.Addrs)
		if err != nil {
			continue
		}
		m.mu.RLock()
		_, known := m.peers[p.PeerID]
		_, flying := m.dialing[p.PeerID]
		m.mu.RUnlock()
		if known || flying {
			continue
		}
		m.mu.Lock()
		m.dialing[p.PeerID] = struct{}{}
		m.mu.Unlock()
		go func(id string, info peer.AddrInfo) {
			defer func() {
				m.mu.Lock()
				delete(m.dialing, id)
				m.mu.Unlock()
			}()
			ctx, cancel := context.WithTimeout(node.Lifetime(), 30*time.Second)
			defer cancel()
			s, err := node.DialRoom(ctx, info)
			if err != nil {
				return
			}
			m.addPeer(id, s, m.selfID, PathRendezvous)
		}(p.PeerID, info)
	}
}

// addrInfoFromAddrs reads the peer out of the addresses the invite carried.
// Every address names the same peer, and a code that names two is refused
// rather than half-dialed.
func addrInfoFromAddrs(addrs []string) (peer.AddrInfo, error) {
	var info peer.AddrInfo
	for _, raw := range addrs {
		addr, err := multiaddr.NewMultiaddr(raw)
		if err != nil {
			return peer.AddrInfo{}, fmt.Errorf("rtc: invite address %q: %w", raw, err)
		}
		ai, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return peer.AddrInfo{}, fmt.Errorf("rtc: invite address %q carries no peer id: %w", raw, err)
		}
		if info.ID == "" {
			info.ID = ai.ID
		}
		if info.ID != ai.ID {
			return peer.AddrInfo{}, fmt.Errorf("rtc: the invite names two different peers")
		}
		info.Addrs = append(info.Addrs, ai.Addrs...)
	}
	if info.ID == "" {
		return peer.AddrInfo{}, fmt.Errorf("rtc: the invite carries no address")
	}
	return info, nil
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

// peerPath is how this node reached the peer.
func (m *Mesh) peerPath(id string) PeerPath {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.peers[id]; ok {
		return p.via
	}
	return ""
}

// Addrs are this peer's dialable addresses, each carrying its peer ID. They
// go into the invite code: a guest that has them dials this peer directly,
// and the room meets without a rendezvous.
func (m *Mesh) Addrs() []string {
	m.mu.RLock()
	node := m.node
	m.mu.RUnlock()
	if node == nil {
		return nil
	}
	return node.Addrs()
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
		m.addPeer(remote, s, remote, PathInbound)
	}
}

// discoverLoop dials discovered peers so late joiners converge without any
// introducer: presence is announced once, discovery runs for the session.
// One flight per peer: a dial already in flight is not started twice, and a
// failed dial frees the peer for the next round.
//
// One lookup at a time, never two: a provider lookup that finds nothing walks
// the network until it gives up, which can take far longer than the interval,
// and a walk still running when the next tick comes would mean two of them at
// once. One flight per peer already keeps a second dial of the same peer from
// being started, and that is the rule that matters here.
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

	// The peer the code named. The room's way in when the address does not
	// work: it is looked up by its id, and a routing lookup is a different and
	// much steadier thing than a provider record, which is where the
	// "closest peers" of the two sides stop matching (bug-0129, bug-0133).
	invited, _ := addrInfoFromAddrs(m.opts.Addrs)
	if invited.ID == "" {
		// A host waits to be called: it has nobody to look up.
		return
	}

	// The lookup runs in its own goroutine, so a slow one cannot swallow the
	// interval: a lookup that finds nothing walks the network until it gives
	// up, and a lookup still running when the next tick came meant the next
	// one waited behind it.
	walking := make(chan struct{}, 1)
	discover := func() {
		select {
		case walking <- struct{}{}:
		default:
			// A lookup is still going. A second would only compete with it
			// for the same DHT and make both slower.
			return
		}

		go func() {
			defer func() { <-walking }()

			id := invited.ID.String()
			m.mu.Lock()
			_, known := m.peers[id]
			_, flying := m.dialing[id]
			if !known && !flying {
				m.dialing[id] = struct{}{}
			} else {
				m.mu.Unlock()
				return
			}
			m.mu.Unlock()

			defer func() {
				m.mu.Lock()
				delete(m.dialing, id)
				m.mu.Unlock()
			}()

			found, err := node.FindPeerWhenReady(ctx, invited.ID)
			if err != nil {
				return
			}
			dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
			defer dialCancel()
			s, err := node.DialRoom(dialCtx, found)
			if err != nil {
				return
			}
			m.addPeer(id, s, m.selfID, PathRendezvous)
		}()
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
		case <-m.wake:
			discover()
		case <-ticker.C:
			discover()
		}
	}
}

// wakeDiscovery asks the discovery loop to look again now. The room became
// findable in the middle of an interval, and waiting out the interval is a
// room that is ready and still says it is looking.
func (m *Mesh) wakeDiscovery() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// addPeer plugs a connected peer into the mesh, keyed by the peer that opened
// the stream.
//
// Two streams to one peer is the normal case, not a rare one: discovery is
// symmetric, so both sides dial in the same second. "First one wins" is not a
// rule both ends can compute — it is decided by who got there first, and each
// side gets a different answer. The loser is then closed, which closes a
// stream the *other* side may have been the one reading, and that side's read
// loop ends and drops the peer from its roster. The room keeps meeting and
// goes silent, which is exactly what the public-DHT scenario caught.
//
// So the surviving stream is the one the peer with the smaller id opened: the
// same rule the room already uses to decide who hosts after a host leaves. Both
// ends compute it, so both keep the same stream and neither tears down the
// connection the other is using.
func (m *Mesh) addPeer(id string, conn io.ReadWriteCloser, initiator string, via PeerPath) {
	m.mu.Lock()
	existing, exists := m.peers[id]
	if exists && existing.initiator <= initiator {
		m.mu.Unlock()
		_ = conn.Close()
		return
	}
	mp := &meshPeer{id: id, conn: conn, enc: json.NewEncoder(conn), initiator: initiator, via: via}
	m.peers[id] = mp
	selfID := m.selfID
	isHost := m.isHost
	node := m.node
	m.mu.Unlock()

	if exists {
		// Replaced, not added: the old stream is the loser of the rule above.
		_ = existing.conn.Close()
	}

	go m.readLoop(mp)

	// Announce presence at once: the roster is whoever said hello, and a peer
	// that waits to be asked is a roster entry that never appears. The
	// greeting carries this node's addresses, so the room can hand them on
	// and the mesh can grow without a rendezvous (bug-0136).
	var addrs []string
	if node != nil {
		addrs = node.Addrs()
	}
	_ = m.sendTo(mp, NewHello(selfID, m.opts.UserID, m.opts.Username, isHost, addrs))
}

func (m *Mesh) sendTo(mp *meshPeer, msg any) error {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	return mp.enc.Encode(msg)
}

func (m *Mesh) readLoop(mp *meshPeer) {
	defer func() {
		m.mu.Lock()
		current := m.peers[mp.id] == mp
		if current {
			delete(m.peers, mp.id)
		}
		m.mu.Unlock()

		if !current {
			// This stream lost the race to another stream to the same peer and
			// the entry has already been replaced. Nobody left: announcing a
			// departure here would drop a member who is still in the room, and
			// the roster would be wrong about people who are watching.
			return
		}

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
