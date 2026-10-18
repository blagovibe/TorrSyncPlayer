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
	n = &Node{host: h, staticRelays: opts.StaticRelays, authKey: authKey, key: key, admitted: make(chan *roomStream, 16)}
	h.SetStreamHandler(syncProtocolID, n.serveHandshake)
	return n, nil
}

// ID is this node's peer ID.
func (n *Node) ID() peer.ID { return n.host.ID() }

// AddrInfo is this node's address for out-of-band exchange (tests).
func (n *Node) AddrInfo() peer.AddrInfo {
	return peer.AddrInfo{ID: n.host.ID(), Addrs: n.host.Addrs()}
}

// Close leaves the swarm.
func (n *Node) Close() error {
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

// defaultBootstrap is the public DHT rendezvous: libp2p's default bootstrap
// peers. Empty settings resolve here; an entry that lost its /p2p/ part is
// skipped rather than trusted.
func defaultBootstrap() []peer.AddrInfo {
	var out []peer.AddrInfo
	for _, addr := range dht.DefaultBootstrapPeers {
		info, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			continue
		}
		out = append(out, *info)
	}
	return out
}

// Bootstrap wires the DHT through the given peers. In tests the peers are
// each other; in production they are the public bootstrap nodes. It returns
// once the DHT runs; use Refresh to wait until the routing table fills —
// that needs the other side to have its DHT up, which simultaneous bring-up
// cannot guarantee yet.
func (n *Node) Bootstrap(ctx context.Context, peers []peer.AddrInfo) error {
	d, err := dht.New(n.host, dht.Mode(dht.ModeServer), dht.BootstrapPeers(peers...))
	if err != nil {
		return fmt.Errorf("rtc: create DHT: %w", err)
	}
	n.mu.Lock()
	n.dht = d
	n.mu.Unlock()
	for _, p := range peers {
		if err := n.host.Connect(ctx, p); err != nil {
			_ = d.Close()
			n.mu.Lock()
			n.dht = nil
			n.mu.Unlock()
			return fmt.Errorf("rtc: connect to bootstrap peer: %w", err)
		}
	}
	// No peers means no wider table to join — a bare DHT server, as the
	// testbed runs. Skipping Bootstrap also skips the fallback to the
	// public default bootstrappers: hermetic tests must not dial out.
	if len(peers) > 0 {
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

// Announce publishes this node's provider record under the community key:
// "this room is watched here".
func (n *Node) Announce(ctx context.Context) error {
	d := n.getDHT()
	if d == nil {
		return fmt.Errorf("rtc: DHT is not bootstrapped")
	}
	if err := d.Provide(ctx, n.key, true); err != nil {
		return fmt.Errorf("rtc: announce room: %w", err)
	}
	return nil
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
