// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multihash"
)

// syncProtocolID names the room streams. Every admitted stream speaks the
// protocol.go messages, newline-delimited: one write is one message, and a
// queued message is never split.
const syncProtocolID = protocol.ID("/torrsync/sync/1.0.0")

// handshakeTimeout bounds the whole admission: challenge, proof, verdict.
const handshakeTimeout = 15 * time.Second

// maxRelayWalks caps one peer-source call's random DHT walks: on a small DHT
// the walks keep returning the same peers, and walking forever would burn
// queries for no new candidates.
const maxRelayWalks = 8

// deriveAuthKey turns the invite password and key pair into the handshake
// HMAC key. Neither travels: the dialer proves knowledge of the pair.
// SHA-256 with a domain separator stretches the short invite parts to the 32
// bytes HMAC wants. There is deliberately no swarm PSK gate: PrivateNetwork
// would wrap every connection — including the public bootstrap nodes and
// public relays the room needs — so admission lives in the handshake, not in
// the transport (idea-0091).
func deriveAuthKey(password, key string) ([]byte, error) {
	if password == "" {
		return nil, fmt.Errorf("rtc: invite password is empty")
	}
	if key == "" {
		return nil, fmt.Errorf("rtc: invite key is empty")
	}
	sum := sha256.Sum256([]byte("torrsync-auth-v1\x00" + password + "\x00" + key))
	return sum[:], nil
}

// rendezvousKey maps a community to the DHT provider key peers announce and
// look up. Raw codec over the SHA-256 of a namespaced community: finding the
// key needs the community, and nothing else about the room leaks into it.
func rendezvousKey(community string) (cid.Cid, error) {
	if community == "" {
		return cid.Undef, fmt.Errorf("rtc: community is empty")
	}
	mh, err := multihash.Sum([]byte("torrsync-room-v1\x00"+community), multihash.SHA2_256, -1)
	if err != nil {
		return cid.Undef, fmt.Errorf("rtc: hash community: %w", err)
	}
	return cid.NewCidV1(cid.Raw, mh), nil
}

// NodeOptions configures one participant's libp2p host.
type NodeOptions struct {
	// Invite is the room's address. Community names the rendezvous key, the
	// password and key pair admits streams in the handshake.
	Invite Invite
	// EnableRelay turns on circuit relay and autorelay for harsh NAT.
	// Off in unit tests (loopback needs none); on everywhere else.
	EnableRelay bool
	// StaticRelays are the owner's fallback relay candidates: autorelay
	// tries them first, then discovers more over the DHT. Empty means DHT
	// discovery only.
	StaticRelays []peer.AddrInfo
}

// Node is one participant's libp2p host: streams admitted by the invite
// password and key pair, peers found over the DHT.
type Node struct {
	mu           sync.Mutex
	host         host.Host
	dht          *dht.IpfsDHT
	staticRelays []peer.AddrInfo
	authKey      []byte
	key          cid.Cid
	published    bool
	// ctx lives as long as the node: work that must outlive the join — the
	// wide publish below — cannot hang off the join's own context, which is
	// cancelled the moment Join returns.
	ctx    context.Context
	cancel context.CancelFunc

	admitted chan *roomStream
}

// NewNode starts a node on all interfaces: a room peer must be dialable
// directly, and loopback-only listen (bug-0092) made every inter-machine
// connection impossible. It joins nothing yet: Bootstrap wires the DHT,
// Announce publishes the room, Discover looks it up.
func NewNode(opts NodeOptions) (*Node, error) {
	if opts.Invite.Community == "" || opts.Invite.Password == "" || opts.Invite.Key == "" {
		return nil, fmt.Errorf("rtc: invite is incomplete")
	}
	authKey, err := deriveAuthKey(opts.Invite.Password, opts.Invite.Key)
	if err != nil {
		return nil, err
	}
	key, err := rendezvousKey(opts.Invite.Community)
	if err != nil {
		return nil, err
	}

	// The peer source closes over the node before it exists: autorelay
	// calls it asynchronously, and a call winning the race with the
	// assignment below must see an empty candidate set, not a nil pointer.
	var n *Node
	peerSource := func(ctx context.Context, numPeers int) <-chan peer.AddrInfo {
		if n == nil {
			empty := make(chan peer.AddrInfo)
			close(empty)
			return empty
		}
		return n.relayPeerSource(ctx, numPeers)
	}

	libp2pOpts := []libp2p.Option{
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0", "/ip4/0.0.0.0/udp/0/quic-v1"),
		libp2p.EnableHolePunching(),
	}
	if opts.EnableRelay {
		libp2pOpts = append(libp2pOpts,
			libp2p.EnableRelay(),
			libp2p.EnableAutoRelayWithPeerSource(autorelay.PeerSource(peerSource)),
		)
	} else {
		libp2pOpts = append(libp2pOpts, libp2p.DisableRelay())
	}

	h, err := libp2p.New(libp2pOpts...)
	if err != nil {
		return nil, fmt.Errorf("rtc: start node: %w", err)
	}
	nodeCtx, nodeCancel := context.WithCancel(context.Background())
	n = &Node{
		host: h, staticRelays: opts.StaticRelays, authKey: authKey, key: key,
		admitted: make(chan *roomStream, 16),
		ctx:      nodeCtx, cancel: nodeCancel,
	}
	h.SetStreamHandler(syncProtocolID, n.serveHandshake)
	return n, nil
}

// ID is this node's peer ID.
func (n *Node) ID() peer.ID { return n.host.ID() }

// Published reports whether this room announced itself to the rendezvous. The
// window polls it from its own goroutine while an announce writes it, so it is
// read under the same lock that writes it.
func (n *Node) Published() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.published
}

// AddrInfo is this node's address for out-of-band exchange (tests).
func (n *Node) AddrInfo() peer.AddrInfo {
	return peer.AddrInfo{ID: n.host.ID(), Addrs: n.host.Addrs()}
}

// Close leaves the swarm.
func (n *Node) Close() error {
	n.cancel()
	n.host.RemoveStreamHandler(syncProtocolID)
	if d := n.getDHT(); d != nil {
		_ = d.Close()
	}
	return n.host.Close()
}

// getDHT reads the DHT under the mutex: autorelay's peer source may walk it
// while the room joins.
func (n *Node) getDHT() *dht.IpfsDHT {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.dht
}

// relayPeerSource is autorelay's candidate supply: the configured static
// relays first, then random DHT walks. The walks hash fresh random keys, so
// successive calls sweep different corners of the keyspace instead of asking
// for the same neighbours twice.
func (n *Node) relayPeerSource(ctx context.Context, numPeers int) <-chan peer.AddrInfo {
	out := make(chan peer.AddrInfo)
	go func() {
		defer close(out)
		seen := map[peer.ID]struct{}{n.ID(): {}}
		emit := func(info peer.AddrInfo) bool {
			if _, dup := seen[info.ID]; dup || len(info.Addrs) == 0 {
				return true
			}
			seen[info.ID] = struct{}{}
			select {
			case out <- info:
				return true
			case <-ctx.Done():
				return false
			}
		}

		n.mu.Lock()
		statics := append([]peer.AddrInfo(nil), n.staticRelays...)
		n.mu.Unlock()
		sent := 0
		for _, info := range statics {
			if sent >= numPeers {
				return
			}
			if !emit(info) {
				return
			}
			sent++
		}

		d := n.getDHT()
		if d == nil {
			return
		}
		for walk := 0; walk < maxRelayWalks && sent < numPeers; walk++ {
			var raw [32]byte
			if _, err := rand.Read(raw[:]); err != nil {
				return
			}
			mh, err := multihash.Sum(raw[:], multihash.SHA2_256, -1)
			if err != nil {
				return
			}
			closest, err := d.GetClosestPeers(ctx, cid.NewCidV1(cid.Raw, mh).String())
			if err != nil {
				return
			}
			for _, id := range closest {
				if sent >= numPeers {
					return
				}
				if !emit(n.host.Peerstore().PeerInfo(id)) {
					return
				}
				sent++
			}
		}
	}()
	return out
}

// seedDialTimeout bounds one rendezvous dial. A seed that does not answer
// must cost that seed, not the join: without a bound, one dead address can
// spend the whole join budget on its own (bug-0121).
const seedDialTimeout = 10 * time.Second

// ownSeedAddrs is the project's own rendezvous list: public DHT servers, named
// by a DNS name that carries A records and reached over TCP, which is the
// transport that is open most often. These are the same long-lived nodes
// libp2p's default list points at — but libp2p reaches them through
// /dnsaddr/bootstrap.libp2p.io, and that name publishes no SRV records, so
// four of its five entries cannot be dialled at all. Owning the list is what
// makes the difference: an address we can dial cannot rot under a name we do
// not control.
//
// Measured from this project against the public DHT (bug-0121): these nodes
// answer, and they fill a routing table on first contact, which is what a
// room needs to find other rooms' participants.
var ownSeedAddrs = []string{
	"/dns4/sing-1.bootstrap.ipni.io/tcp/4001/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
	"/dns4/chic-1.bootstrap.ipni.io/tcp/4001/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
	"/dns4/mars.i.ipfs.io/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
}

// ownSeeds is the project's own rendezvous list, parsed. An entry that does
// not parse is skipped rather than fatal: a typo in a list of addresses must
// not take every room down with it, and the tests read the raw list so the
// typo cannot hide.
func ownSeeds() []peer.AddrInfo {
	addrs := make([]multiaddr.Multiaddr, 0, len(ownSeedAddrs))
	for _, raw := range ownSeedAddrs {
		addr, err := multiaddr.NewMultiaddr(raw)
		if err != nil {
			continue
		}
		addrs = append(addrs, addr)
	}
	return resolveAddrInfos(addrs)
}

// resolveAddrInfos turns multiaddrs into dialable peers, skipping any entry
// that lost its /p2p/ part rather than trusting it.
func resolveAddrInfos(addrs []multiaddr.Multiaddr) []peer.AddrInfo {
	var out []peer.AddrInfo
	for _, addr := range addrs {
		info, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			continue
		}
		out = append(out, *info)
	}
	return out
}

// defaultBootstrap is the rendezvous list for an empty setting: the project's
// own seeds first, then whatever libp2p's default list still offers, one entry
// per peer and ours winning. The library list is kept as the tail on purpose —
// it costs nothing when it does not answer, and it is the way a node the
// project has not heard of yet still gets reached.
func defaultBootstrap() []peer.AddrInfo {
	own := ownSeeds()
	seen := make(map[peer.ID]struct{}, len(own))
	out := make([]peer.AddrInfo, 0, len(own))
	for _, info := range own {
		seen[info.ID] = struct{}{}
		out = append(out, info)
	}
	for _, info := range resolveAddrInfos(dht.DefaultBootstrapPeers) {
		if _, dup := seen[info.ID]; dup {
			continue
		}
		seen[info.ID] = struct{}{}
		out = append(out, info)
	}
	return out
}

// Bootstrap wires the DHT through the given peers. In tests the peers are
// each other; in production they are the project's rendezvous list. It returns
// once the DHT runs; use Refresh to wait until the routing table fills —
// that needs the other side to have its DHT up, which simultaneous bring-up
// cannot guarantee yet.
//
// A seed that does not answer is skipped, never fatal: the room loses that
// seed, not the join. Only when not one of them answers does the join stop,
// and then it says in those words that the rendezvous is unreachable — the
// goal is that nobody has to guess whether the room or their network is at
// fault.
func (n *Node) Bootstrap(ctx context.Context, peers []peer.AddrInfo) error {
	d, err := dht.New(n.host, dht.Mode(dht.ModeServer), dht.BootstrapPeers(peers...))
	if err != nil {
		return fmt.Errorf("rtc: create DHT: %w", err)
	}
	n.mu.Lock()
	n.dht = d
	n.mu.Unlock()

	// No peers means no wider table to join — a bare DHT server, as the
	// testbed runs. Skipping the dials also skips the fallback to the
	// public default bootstrappers: hermetic tests must not dial out.
	if len(peers) > 0 {
		if n.dialSeeds(ctx, peers) == 0 {
			_ = d.Close()
			n.mu.Lock()
			n.dht = nil
			n.mu.Unlock()
			return fmt.Errorf("rtc: rendezvous unreachable: none of the %d rendezvous nodes answered", len(peers))
		}
		if err := d.Bootstrap(ctx); err != nil {
			_ = d.Close()
			n.mu.Lock()
			n.dht = nil
			n.mu.Unlock()
			return fmt.Errorf("rtc: bootstrap DHT: %w", err)
		}
	}
	return nil
}

// dialSeeds connects to as many rendezvous nodes as answer, each under its own
// bound, and reports how many did. Dialling in parallel matters: sequential
// dials made the join wait out every dead address in turn, which is how one
// unroutable seed once turned a working room into a minute of silence.
func (n *Node) dialSeeds(ctx context.Context, peers []peer.AddrInfo) int {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		answered int
	)
	for _, p := range peers {
		wg.Add(1)
		go func(p peer.AddrInfo) {
			defer wg.Done()
			dialCtx, cancel := context.WithTimeout(ctx, seedDialTimeout)
			defer cancel()
			if err := n.host.Connect(dialCtx, p); err != nil {
				return
			}
			mu.Lock()
			answered++
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return answered
}

// WaitReady blocks until the routing table is non-empty: only then can the
// node announce and discover. The table fills on its own once identify
// reports the bootstrap peers as kad speakers — a forced crawl with no seeds
// fails instead of helping, so this waits rather than pushes.
func (n *Node) WaitReady(ctx context.Context) error {
	d := n.getDHT()
	if d == nil {
		return fmt.Errorf("rtc: DHT is not bootstrapped")
	}
	deadline := time.Now().Add(30 * time.Second)
	for d.RoutingTable().Size() == 0 {
		if time.Now().After(deadline) {
			return fmt.Errorf("rtc: DHT routing table stayed empty")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("rtc: DHT ready: %w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil
}

// announceOnJoinBroadcast is false: the announce that runs during the join
// writes the room's provider record to the closest peers only.
//
// announceWideningBroadcast is true: the follow-up publish reaches every peer
// the DHT considers closest to the room key.
//
// Both are measured, not preferred. The wide form of this exact key took 2m16s
// against the public DHT — past the 60s join budget, which is how a room
// became impossible to enter (bug-0121). The narrow form is instant but
// insufficient on its own: with the record on the single closest peer, a guest
// that looked in the same period did not find the room, because "the closest
// peers" is not one fixed set — a lookup asks the peers closest to *its own*
// view. So the join publishes quickly and the wide publish follows off the
// join path, where minutes cost nobody.
const (
	announceOnJoinBroadcast     = false
	announceWideningBroadcast   = true
	announceWideningRepublishes = 30 * time.Minute
)

// announceTimeout bounds the announce the join waits for, whatever budget the
// caller handed in. A rendezvous that needs longer than this is not slow, it
// is not there, and the join must learn that in seconds rather than in a
// minute of silence.
const announceTimeout = 15 * time.Second

// Announce publishes this node's provider record under the community key:
// "this room is watched here". A failed announce is reported, never fatal to
// the join: the room is already open, peers already inside are found by
// discovery all the same, and the window shows the room as not published, so
// nobody sits waiting for a guest who was never told.
func (n *Node) Announce(ctx context.Context) error {
	d := n.getDHT()
	if d == nil {
		return fmt.Errorf("rtc: DHT is not bootstrapped")
	}
	announceCtx, cancel := context.WithTimeout(ctx, announceTimeout)
	defer cancel()

	if err := d.Provide(announceCtx, n.key, announceOnJoinBroadcast); err != nil {
		n.setPublished(false)
		return fmt.Errorf("rtc: announce room: %w", err)
	}
	n.setPublished(true)

	go n.widenAnnouncement()
	return nil
}

// widenAnnouncement publishes the room to the whole closest set, off the join
// path, and keeps doing so for as long as the node lives: a room that has
// widened once still expires as the DHT's provider records expire, and a room
// whose record went stale is a room a guest cannot find.
//
// The first widening is immediate, not deferred to the next tick. Measured
// against the public DHT: with the room left on the closest peer alone, a
// guest's provider lookup returned the guest's own record and the host's
// nothing — not once in two minutes of lookups. "Closest" is not one fixed
// set, so a room that means to be found publishes wide.
func (n *Node) widenAnnouncement() {
	publish := func(broadcast bool, budget time.Duration) {
		d := n.getDHT()
		if d == nil {
			return
		}
		// The wide publish is allowed to be slow — that is what it is — but it
		// still gets a bound, so a wedged DHT cannot leak a goroutine.
		ctx, cancel := context.WithTimeout(n.ctx, budget)
		_ = d.Provide(ctx, n.key, broadcast)
		cancel()
	}

	publish(announceWideningBroadcast, announceWideningRepublishes)
	ticker := time.NewTicker(announceWideningRepublishes)
	defer ticker.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			publish(announceWideningBroadcast, announceWideningRepublishes)
		}
	}
}

func (n *Node) setPublished(published bool) {
	n.mu.Lock()
	n.published = published
	n.mu.Unlock()
}

// Discover looks the community key up in the DHT. The channel closes when the
// lookup ends or ctx is done; the caller picks its peers out of it.
func (n *Node) Discover(ctx context.Context) (<-chan peer.AddrInfo, error) {
	d := n.getDHT()
	if d == nil {
		return nil, fmt.Errorf("rtc: DHT is not bootstrapped")
	}
	return d.FindProvidersAsync(ctx, n.key, 10), nil
}

// DialRoom opens an admitted room stream to a discovered peer: connect,
// prove the password, and only then speak protocol. Room streams allow
// limited connections: a relayed connection is constrained by construction,
// and refusing it would make every path autorelay reserves unusable.
func (n *Node) DialRoom(ctx context.Context, info peer.AddrInfo) (*roomStream, error) {
	if err := n.host.Connect(ctx, info); err != nil {
		return nil, fmt.Errorf("rtc: connect to room peer: %w", err)
	}
	ctx = network.WithAllowLimitedConn(ctx, "torrsync/sync")
	s, err := n.host.NewStream(ctx, info.ID, syncProtocolID)
	if err != nil {
		return nil, fmt.Errorf("rtc: open room stream: %w", err)
	}
	rw, err := provePassword(s, n.authKey)
	if err != nil {
		_ = s.Reset()
		return nil, err
	}
	return &roomStream{Stream: s, r: rw}, nil
}

// WaitAdmitted blocks until a peer passes the handshake on an inbound stream.
func (n *Node) WaitAdmitted(ctx context.Context) (*roomStream, error) {
	select {
	case s := <-n.admitted:
		return s, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("rtc: no admitted peer: %w", ctx.Err())
	}
}

// serveHandshake is the responder side: challenge, verify the proof, admit or
// reset. A wrong password ends at the reset: no protocol byte is read, none
// is answered.
func (n *Node) serveHandshake(s network.Stream) {
	if err := s.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		_ = s.Reset()
		return
	}
	var challenge [32]byte
	if _, err := rand.Read(challenge[:]); err != nil {
		_ = s.Reset()
		return
	}
	rw := bufio.NewReader(s)
	if _, err := fmt.Fprintf(s, "challenge %s\n", hex.EncodeToString(challenge[:])); err != nil {
		_ = s.Reset()
		return
	}
	line, err := rw.ReadBytes('\n')
	if err != nil {
		_ = s.Reset()
		return
	}
	proof, ok := strings.CutPrefix(strings.TrimSpace(string(line)), "proof ")
	if !ok {
		_ = s.Reset()
		return
	}
	mac := hmac.New(sha256.New, n.authKey)
	mac.Write(challenge[:])
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(proof)) != 1 {
		_ = s.Reset()
		return
	}
	if _, err := fmt.Fprintln(s, "ok"); err != nil {
		_ = s.Reset()
		return
	}
	if err := s.SetDeadline(time.Time{}); err != nil {
		_ = s.Reset()
		return
	}
	select {
	case n.admitted <- &roomStream{Stream: s, r: rw}:
	default:
		_ = s.Reset()
	}
}

// roomStream is an admitted room stream together with whatever the handshake
// already read into its buffer.
//
// The handshake reads through a bufio.Reader, and a bufio.Reader reads ahead:
// the peer's first protocol message very often arrives in the same segment as
// the handshake's verdict, because the other side writes them back to back. A
// reader that is thrown away after the handshake takes that message with it —
// and the message is the presence greeting. The peer then never learns that
// anyone is there: no roster, no host, and a follower who follows nobody
// (bug-0119).
//
// So the reader travels with the stream: whatever it already holds is read
// first, and nothing is lost between the handshake and the first protocol
// line.
type roomStream struct {
	network.Stream
	r *bufio.Reader
}

// Read serves the handshake's leftovers before the socket itself.
func (s *roomStream) Read(p []byte) (int, error) { return s.r.Read(p) }

// provePassword is the dialer side: answer the challenge, await the verdict.
//
// It hands back the reader it used: the verdict and the peer's first message
// can arrive together, and the message must not be lost with the reader.
func provePassword(s network.Stream, authKey []byte) (*bufio.Reader, error) {
	if err := s.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return nil, fmt.Errorf("rtc: handshake deadline: %w", err)
	}
	rw := bufio.NewReader(s)
	line, err := rw.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("rtc: read handshake challenge: %w", err)
	}
	raw, ok := strings.CutPrefix(strings.TrimSpace(string(line)), "challenge ")
	if !ok {
		return nil, fmt.Errorf("rtc: bad handshake challenge")
	}
	challenge, err := hex.DecodeString(raw)
	if err != nil || len(challenge) != 32 {
		return nil, fmt.Errorf("rtc: bad handshake challenge")
	}
	mac := hmac.New(sha256.New, authKey)
	mac.Write(challenge)
	if _, err := fmt.Fprintf(s, "proof %s\n", hex.EncodeToString(mac.Sum(nil))); err != nil {
		return nil, fmt.Errorf("rtc: send handshake proof: %w", err)
	}
	verdict, err := rw.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("rtc: admission rejected: %w", err)
	}
	if strings.TrimSpace(string(verdict)) != "ok" {
		return nil, fmt.Errorf("rtc: admission rejected")
	}
	if err := s.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("rtc: clear handshake deadline: %w", err)
	}
	return rw, nil
}
