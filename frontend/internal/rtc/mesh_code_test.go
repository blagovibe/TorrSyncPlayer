// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

// The room meets two ways now. The code names the host, and the guest dials
// it — that is the fast path, and it is what these tests pin. The rendezvous
// stays behind it as a fallback, so a test that lets the rendezvous work
// proves nothing about the code.

// TestTheRoomMeetsByTheCodeAlone is the fast path as a check: a rendezvous
// that cannot answer, a code that names the host, and two peers that meet
// anyway. The room must assemble in seconds — not minutes, and not by the
// rendezvous, which is not there.
func TestTheRoomMeetsByTheCodeAlone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	invite := testInvite()
	bootstrap := unreachableBootstrap(t)

	host := NewMesh(MeshOptions{
		Community: invite.Community, Password: invite.Password, Key: invite.Key,
		Bootstrap: bootstrap, UserID: "host",
	})
	t.Cleanup(func() { _ = host.Close() })
	if err := host.Join(ctx, true); err != nil {
		t.Fatalf("the host could not join: %v", err)
	}

	// The code the host copies: community, password, key and the addresses
	// the guest dials.
	code := Invite{
		Community: invite.Community, Password: invite.Password, Key: invite.Key,
		Addrs: host.Addrs(),
	}.Encode()
	parsed, err := ParseInviteCode(code)
	if err != nil {
		t.Fatalf("the host's own code does not parse: %v", err)
	}

	guest := NewMesh(MeshOptions{
		Community: parsed.Community, Password: parsed.Password, Key: parsed.Key,
		Addrs: parsed.Addrs, Bootstrap: bootstrap, UserID: "guest",
	})
	t.Cleanup(func() { _ = guest.Close() })
	if err := guest.Join(ctx, false); err != nil {
		t.Fatalf("the guest could not join: %v", err)
	}

	waitForPeers(t, ctx, host, 1, "the host never saw the guest")
	waitForPeers(t, ctx, guest, 1, "the guest never saw the host")
}

// TestTheInviteCodeCarriesTheAddress pins what the code is for: the address
// the guest dials, with the peer ID inside it. The ID matters as much as the
// address — it is what the guest pins, so it cannot be left out.
func TestTheInviteCodeCarriesTheAddress(t *testing.T) {
	addr := p2pAddr(t, "/ip4/127.0.0.1/tcp/4001")
	invite := Invite{
		Community: "room-abc", Password: "secret", Key: "key-123",
		Addrs: []string{addr},
	}

	parsed, err := ParseInviteCode(invite.Encode())
	require.NoError(t, err, "a code with an address must parse")
	require.Len(t, parsed.Addrs, 1, "the address did not survive the code")
	require.Equal(t, addr, parsed.Addrs[0], "the address came back changed")

	info, err := addrInfoFromAddrs(parsed.Addrs)
	require.NoError(t, err, "the address must name a peer")
	require.NotEmpty(t, info.ID, "the peer ID did not survive the code")
	require.NotEmpty(t, info.Addrs, "the address did not survive the code")
}

// TestTheInviteCodeRefusesTwoPeers is the code that cannot be trusted: two
// addresses naming two different peers. Dialling half of it is worse than
// refusing it.
func TestTheInviteCodeRefusesTwoPeers(t *testing.T) {
	one := p2pAddr(t, "/ip4/127.0.0.1/tcp/4001")
	other := p2pAddr(t, "/ip4/127.0.0.2/tcp/4001")
	_, err := addrInfoFromAddrs([]string{one, other})
	require.Error(t, err, "a code naming two peers was accepted")
}

// TestALegacyCodeStillNamesARoom is the old shape: three parts, no address.
// Such a code can only be met through the rendezvous, and the room says so
// rather than pretending otherwise.
func TestALegacyCodeStillNamesARoom(t *testing.T) {
	parsed, err := ParseInviteCode("community:password:key")
	require.NoError(t, err, "a code without an address must still parse")
	require.Empty(t, parsed.Addrs, "a code without an address grew one")
}

// waitForPeers waits until the mesh sees the wanted number of peers. The
// meeting is seconds, not minutes, and a failure must say which side did not
// see the other.
func waitForPeers(t *testing.T, ctx context.Context, mesh *Mesh, want int, msg string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if len(mesh.Peers()) >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s: %d peers, want %d", msg, len(mesh.Peers()), want)
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("%s: %d peers, want %d", msg, len(mesh.Peers()), want)
}

// unreachableBootstrap is a rendezvous that cannot answer: a real peer ID on
// a closed port. A test that lets the rendezvous work proves nothing about
// the code.
func unreachableBootstrap(t *testing.T) string {
	t.Helper()
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	require.NoError(t, err)
	id, err := peer.IDFromPublicKey(pub)
	require.NoError(t, err)
	return fmt.Sprintf("/ip4/127.0.0.1/tcp/1/p2p/%s", id)
}

// p2pAddr builds a multiaddr with a real peer ID in it, so the address can
// be dialled and its ID read.
func p2pAddr(t *testing.T, base string) string {
	t.Helper()
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	require.NoError(t, err)
	id, err := peer.IDFromPublicKey(pub)
	require.NoError(t, err)
	return fmt.Sprintf("%s/p2p/%s", base, id)
}
