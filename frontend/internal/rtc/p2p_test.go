// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"bufio"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Slice 1 of the libp2p transplant (idea-0086), as corrected by idea-0091:
// a node is one participant's libp2p host. Streams are admitted by the
// invite password and key pair (challenge-response before a single protocol
// byte flows), and peers find each other over the DHT (provider records
// under the community key). There is deliberately no swarm PSK gate — it
// would cut the node off from the public bootstrap nodes and relays.
// These tests run against an in-process DHT testbed: two loopback nodes
// bootstrapped to each other, no network past the machine.

func TestDeriveAuthKey(t *testing.T) {
	a, err := deriveAuthKey("test-password", "test-key-01234567")
	require.NoError(t, err)
	assert.Len(t, a, 32)

	again, err := deriveAuthKey("test-password", "test-key-01234567")
	require.NoError(t, err)
	assert.Equal(t, a, again, "derivation must be deterministic")

	otherPassword, err := deriveAuthKey("another-password", "test-key-01234567")
	require.NoError(t, err)
	assert.NotEqual(t, a, otherPassword, "the password must matter")

	otherKey, err := deriveAuthKey("test-password", "different-key-0")
	require.NoError(t, err)
	assert.NotEqual(t, a, otherKey, "the key must matter: admission proves the pair")

	legacy, err := deriveAuthKey("test-password", "")
	assert.Error(t, err, "a keyless invite must not admit")
	_ = legacy

	_, err = deriveAuthKey("", "test-key-01234567")
	assert.Error(t, err, "an empty password must not admit")
}

func TestRendezvousKey(t *testing.T) {
	a, err := rendezvousKey("test-room")
	require.NoError(t, err)

	again, err := rendezvousKey("test-room")
	require.NoError(t, err)
	assert.True(t, a.Equals(again), "same community must rendezvous at the same key")

	other, err := rendezvousKey("other-room")
	require.NoError(t, err)
	assert.False(t, a.Equals(other))

	_, err = rendezvousKey("")
	assert.Error(t, err)
}

// testInvite is one room both ends agree on. Password and key are short here
// the way GenerateInvite makes them; derivation stretches them, it does not
// assume length.
func testInvite() Invite {
	return Invite{Community: "test-room", Password: "test-password", Key: "test-key-01234567"}
}

func startNode(t *testing.T, inv Invite) *Node {
	t.Helper()
	n, err := NewNode(NodeOptions{Invite: inv})
	require.NoError(t, err, "a node must start with no arguments")
	t.Cleanup(func() { _ = n.Close() })
	return n
}

// startTestbed starts a stable DHT node the room nodes bootstrap through.
// It mirrors production, where the bootstrap nodes are long-lived DHT
// servers: whoever bootstraps meets a peer that already speaks kad, so the
// routing table can fill on first contact. It announces nothing and joins no
// room.
func startTestbed(t *testing.T, ctx context.Context, inv Invite) *Node {
	t.Helper()
	bed := startNode(t, inv)
	require.NoError(t, bed.Bootstrap(ctx, nil))
	return bed
}

// TestAGuestFindsTheHostByItsIDThroughTheDHT is the whole slice in one
// assertion: the guest knows the host's id and nothing else — no address, no
// provider record — the DHT's routing table finds it, the password handshake
// passes, and a magnet travels the admitted stream. This is the fallback the
// room has when the address in the code did not work.
func TestAGuestFindsTheHostByItsIDThroughTheDHT(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inv := testInvite()
	bed := startTestbed(t, ctx, inv)
	host := startNode(t, inv)
	guest := startNode(t, inv)

	require.NoError(t, host.Bootstrap(ctx, []peer.AddrInfo{bed.AddrInfo()}))
	require.NoError(t, guest.Bootstrap(ctx, []peer.AddrInfo{bed.AddrInfo()}))
	// Both DHTs must be up before either table can fill: whoever bootstrapped
	// first met a peer with no DHT yet.
	require.NoError(t, host.WaitReady(ctx))
	require.NoError(t, guest.WaitReady(ctx))

	hostInfo, err := guest.FindPeerWhenReady(ctx, host.ID())
	require.NoError(t, err, "the guest must find the host by its id")
	require.Equal(t, host.ID(), hostInfo.ID, "the guest must find the host, not somebody else")

	stream, err := guest.DialRoom(ctx, hostInfo)
	require.NoError(t, err, "the password handshake must pass")
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
	require.NoError(t, err, "the host must admit the stream its handshake passed")
	line, err := bufio.NewReader(admitted).ReadBytes('\n')
	require.NoError(t, err)
	got, err := Parse(line)
	require.NoError(t, err)
	assert.Equal(t, magnet, got)
}

// TestWrongPasswordIsRejected pins the admission rule at the new transport:
// the key is right but the password is wrong, so the pair proof fails and
// the handshake — not the mesh, not the UI — turns the peer away.
func TestWrongPasswordIsRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	host := startNode(t, testInvite())
	strangerInvite := testInvite()
	strangerInvite.Password = "wrong-password"
	stranger := startNode(t, strangerInvite)
	bed := startTestbed(t, ctx, testInvite())

	require.NoError(t, host.Bootstrap(ctx, []peer.AddrInfo{bed.AddrInfo()}))
	require.NoError(t, stranger.Bootstrap(ctx, []peer.AddrInfo{bed.AddrInfo()}))
	require.NoError(t, host.WaitReady(ctx))
	require.NoError(t, stranger.WaitReady(ctx))

	_, err := stranger.DialRoom(ctx, host.AddrInfo())
	assert.Error(t, err, "a stranger with the wrong password must not get a stream")
}

// TestDifferentKeyCannotJoin pins the admission rule after idea-0091: without
// the PSK swarm gate the transport connects, but the handshake proves the
// password and key pair — so a wrong key fails admission and no stream opens.
func TestDifferentKeyCannotConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	host := startNode(t, testInvite())
	outsiderInvite := testInvite()
	outsiderInvite.Key = "different-key-0"
	outsider := startNode(t, outsiderInvite)

	// Hand it the address directly — knowledge of the address must not help
	// without the key: the transport connects, the handshake refuses.
	dialCtx, dialCancel := context.WithTimeout(ctx, 20*time.Second)
	defer dialCancel()
	_, err := outsider.DialRoom(dialCtx, host.AddrInfo())
	assert.Error(t, err, "without the invite key the handshake must fail")

	admitCtx, admitCancel := context.WithTimeout(ctx, 5*time.Second)
	defer admitCancel()
	_, err = host.WaitAdmitted(admitCtx)
	assert.Error(t, err, "the host must admit no stream from a wrong-key peer")
}
