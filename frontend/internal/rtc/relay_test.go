// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Slice 3 of the libp2p transplant (idea-0087): autorelay with a peer source.
// Bare EnableAutoRelay panics without candidates, so the node carries its own:
// the configured static relays first, then random DHT walks. These tests run
// against in-process nodes; no network past the machine.

// startRelay runs a circuit relay the room nodes can reserve. It joins no
// room and speaks no protocol: it only forwards bytes. The raw relayv2
// service serves hop unconditionally; the host-level relay manager would wait
// for public reachability, which a loopback test never reports.
func startRelay(t *testing.T) host.Host {
	t.Helper()
	h, err := libp2p.New(
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
	)
	require.NoError(t, err, "a relay must start on loopback")
	_, err = relayv2.New(h)
	require.NoError(t, err, "the relay service must attach")
	t.Cleanup(func() { _ = h.Close() })
	return h
}

// circuitAddr reaches peerID only through relay: the caller who dials this
// address cannot fall back to a direct connection, so a byte that arrives
// travelled the relay.
func circuitAddr(t *testing.T, relay host.Host, peerID peer.ID) peer.AddrInfo {
	t.Helper()
	require.NotEmpty(t, relay.Addrs(), "the relay must advertise an address")
	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p/%s/p2p-circuit/p2p/%s",
		relay.Addrs()[0].String(), relay.ID().String(), peerID.String()))
	require.NoError(t, err)
	return peer.AddrInfo{ID: peerID, Addrs: []multiaddr.Multiaddr{addr}}
}

func startNodeWithRelays(t *testing.T, inv Invite, relays []peer.AddrInfo) *Node {
	t.Helper()
	n, err := NewNode(NodeOptions{Invite: inv, EnableRelay: true, StaticRelays: relays})
	require.NoError(t, err, "a node with relays must start")
	t.Cleanup(func() { _ = n.Close() })
	return n
}

// TestRelayedStreamCarriesMagnet is the promised relay gain in one assertion:
// two room nodes that know each other only by circuit address exchange a
// magnet over the admitted stream.
func TestRelayedStreamCarriesMagnet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inv := testInvite()
	relay := startRelay(t)
	relayInfo := peer.AddrInfo{ID: relay.ID(), Addrs: relay.Addrs()}

	host := startNodeWithRelays(t, inv, []peer.AddrInfo{relayInfo})
	guest := startNodeWithRelays(t, inv, []peer.AddrInfo{relayInfo})

	// Circuit v2 delivers to a peer only while it holds a reservation. In
	// production autorelay keeps it; here the test holds it explicitly, so
	// the stream below proves the relayed path rather than autorelay's
	// background timing.
	for _, n := range []*Node{host, guest} {
		_, err := client.Reserve(ctx, n.host, relayInfo)
		require.NoError(t, err, "a room node must be able to reserve the relay")
	}

	stream, err := guest.DialRoom(ctx, circuitAddr(t, relay, host.ID()))
	require.NoError(t, err, "the handshake must pass over the relayed stream")
	defer func() { _ = stream.Close() }()

	magnet := Magnet{
		Envelope:     Envelope{Type: TypeMagnet},
		MagnetURI:    "magnet:?xt=urn:btih:abc",
		InfoHash:     "abc",
		SelectedFile: 0,
		Name:         "Movie.2024.mkv",
	}
	raw, err := json.Marshal(magnet)
	require.NoError(t, err)
	_, err = stream.Write(append(raw, '\n'))
	require.NoError(t, err)

	admitted, err := host.WaitAdmitted(ctx)
	require.NoError(t, err, "the host must admit the relayed stream")
	line, err := bufio.NewReader(admitted).ReadBytes('\n')
	require.NoError(t, err)
	got, err := Parse(line)
	require.NoError(t, err)
	assert.Equal(t, magnet, got)
}

// TestDHTPeerSourceYieldsCandidates pins the primary relay source: random
// DHT walks must return dialable candidates, so autorelay never sits with an
// empty candidate set while the DHT knows peers.
func TestDHTPeerSourceYieldsCandidates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inv := testInvite()
	bed := startTestbed(t, ctx, inv)
	n := startNode(t, inv)
	require.NoError(t, n.Bootstrap(ctx, []peer.AddrInfo{bed.AddrInfo()}))
	require.NoError(t, n.WaitReady(ctx))

	found := n.relayPeerSource(ctx, 3)
	select {
	case info, ok := <-found:
		require.True(t, ok, "the source must yield before it closes")
		assert.NotEqual(t, n.ID(), info.ID, "a node must not relay through itself")
		assert.NotEmpty(t, info.Addrs, "a candidate without addresses is no candidate")
	case <-ctx.Done():
		t.Fatal("the DHT peer source yielded nothing while the DHT knows the testbed")
	}
}

// TestResolveRelays pins the setting's shape: empty means DHT discovery only
// (no error, no addresses), anything else must be dialable multiaddrs.
func TestResolveRelays(t *testing.T) {
	got, err := resolveRelays("")
	require.NoError(t, err, "empty relays must mean DHT discovery, not a misconfiguration")
	assert.Empty(t, got)

	_, err = resolveRelays("not-a-multiaddr")
	assert.Error(t, err, "a setting that cannot be dialed must fail loudly")

	_, err = resolveRelays("/ip4/127.0.0.1/tcp/4001")
	assert.Error(t, err, "a relay address without /p2p/ cannot be dialed")

	dup := "/ip4/127.0.0.1/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN"
	got, err = resolveRelays(dup + ", " + dup)
	require.NoError(t, err)
	assert.Len(t, got, 1, "a relay listed twice is still one relay")
}
