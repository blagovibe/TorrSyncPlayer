// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The room window's connection block reads the node, not the protocol:
// DHTReady, and per peer direct or relayed. These tests pin that the flags
// follow the actual connections, because a diagnostics line that cannot go
// red is decoration.

// TestDHTReadyTracksBootstrap pins the rendezvous flag both ways: nothing
// bootstrapped reads false, a node on a live DHT reads true.
func TestDHTReadyTracksBootstrap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inv := testInvite()
	n := startNode(t, inv)
	assert.False(t, n.DHTReady(), "an unbootstrapped node has no rendezvous")

	bed := startTestbed(t, ctx, inv)
	require.NoError(t, n.Bootstrap(ctx, []peer.AddrInfo{bed.AddrInfo()}))
	require.NoError(t, n.WaitReady(ctx))
	assert.True(t, n.DHTReady(), "a node on a live DHT has a rendezvous")
}

// TestDirectPeerIsNotRelayed pins the common case: two nodes that dialled
// each other directly must not read as relayed.
func TestDirectPeerIsNotRelayed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inv := testInvite()
	bed := startTestbed(t, ctx, inv)
	host := startNode(t, inv)
	guest := startNode(t, inv)
	require.NoError(t, host.Bootstrap(ctx, []peer.AddrInfo{bed.AddrInfo()}))
	require.NoError(t, guest.Bootstrap(ctx, []peer.AddrInfo{bed.AddrInfo()}))
	require.NoError(t, host.WaitReady(ctx))
	require.NoError(t, guest.WaitReady(ctx))

	stream, err := guest.DialRoom(ctx, peer.AddrInfo{ID: host.ID(), Addrs: host.AddrInfo().Addrs})
	require.NoError(t, err, "the direct dial must succeed")
	defer func() { _ = stream.Close() }()

	assert.False(t, host.PeerRelayed(guest.ID()), "a directly dialled peer is not relayed")
}

// TestRelayedPeerReadsRelayed pins the other half: a peer reachable only by
// circuit address must read as relayed. Same rigging as the relayed-magnet
// test — the byte that arrives travelled the relay, so must the flag.
func TestRelayedPeerReadsRelayed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inv := testInvite()
	relay := startRelay(t)
	relayInfo := peer.AddrInfo{ID: relay.ID(), Addrs: relay.Addrs()}

	host := startNodeWithRelays(t, inv, []peer.AddrInfo{relayInfo})
	guest := startNodeWithRelays(t, inv, []peer.AddrInfo{relayInfo})

	for _, n := range []*Node{host, guest} {
		_, err := client.Reserve(ctx, n.host, relayInfo)
		require.NoError(t, err, "a room node must be able to reserve the relay")
	}

	stream, err := guest.DialRoom(ctx, circuitAddr(t, relay, host.ID()))
	require.NoError(t, err, "the relayed dial must succeed")
	defer func() { _ = stream.Close() }()

	assert.True(t, host.PeerRelayed(guest.ID()), "a circuit-only peer reads as relayed")
}

// TestPeerRelayedWithoutConnectionIsDirect pins the absence rule: a peer the
// node holds no connection to must not read as relayed. The roster owns
// absence; the flag must not invent a relay.
func TestPeerRelayedWithoutConnectionIsDirect(t *testing.T) {
	inv := testInvite()
	a := startNode(t, inv)
	b := startNode(t, inv)
	assert.False(t, a.PeerRelayed(b.ID()), "an unconnected peer is not relayed")
}

// TestMeshConnStatusBeforeJoinIsEmpty pins the pre-join state: no node, no
// knowledge — an empty status, not an error and not a guess.
func TestMeshConnStatusBeforeJoinIsEmpty(t *testing.T) {
	inv := testInvite()
	m := NewMesh(MeshOptions{Community: inv.Community, Password: inv.Password, Key: inv.Key})
	st := m.ConnStatus()
	assert.Empty(t, st.SelfID)
	assert.False(t, st.DHTReady)
	assert.Empty(t, st.Peers)
}

// TestMeshConnStatusSaysWhetherTheRoomAnnouncedItself pins the one piece of
// connection state the window needs to keep a person from waiting: a room
// that never announced itself will never be found by a guest who is not here
// yet, and "rendezvous ready" on its own would not say so (bug-0121).
//
// The announcement follows the rendezvous, and the rendezvous comes up behind
// the join: a room whose code named its peer does not wait for a network to
// be ready. So the state is not there the instant the join returns — it is
// there once the room has announced itself, and the window is what tells the
// two apart.
func TestMeshConnStatusSaysWhetherTheRoomAnnouncedItself(t *testing.T) {
	inv := testInvite()
	m := NewMesh(MeshOptions{Community: inv.Community, Password: inv.Password, Key: inv.Key})
	assert.False(t, m.ConnStatus().Published, "a mesh that never joined has published nothing")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	host, _ := joinMesh(t, ctx, 0)
	deadline := time.Now().Add(30 * time.Second)
	for !host.ConnStatus().Published && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	assert.True(t, host.ConnStatus().Published, "a joined room has announced itself")
}

// TestMeshConnStatusListsRoomPeers pins the window's input end to end: a
// joined mesh reports itself, a ready DHT, and its room peer as direct.
func TestMeshConnStatusListsRoomPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	host, gs := joinMesh(t, ctx, 1)

	st := host.ConnStatus()
	assert.Equal(t, host.SelfID(), st.SelfID, "the status names this node")
	assert.True(t, st.DHTReady, "a joined mesh stands on a live DHT")
	require.Len(t, st.Peers, 1, "one guest joined")
	assert.Equal(t, gs[0].SelfID(), st.Peers[0].ID)
	assert.False(t, st.Peers[0].Relayed, "testbed peers connect directly")
}
