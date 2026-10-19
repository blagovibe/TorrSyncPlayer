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

// TestALegacyCodeIsRefusedWithSomethingToDo is the old shape: three parts, no
// address. A room met through the rendezvous is a room that assembles in
// minutes and sometimes not at all, so such a code is refused — and the person
// pasting it is told to ask for a new one, not left with a silent failure.
func TestALegacyCodeIsRefusedWithSomethingToDo(t *testing.T) {
	_, err := ParseInviteCode("community:password:key")
	require.Error(t, err, "a code without an address must not parse")
	require.Contains(t, err.Error(), "older version",
		"the refusal must say what the code is: %v", err)
	require.Contains(t, err.Error(), "new code",
		"the refusal must say what to do: %v", err)
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

// TestTheRoomIsAMeshAndNotAStar is the goal's own word for it: the network is
// symmetric, the control is not. Three participants, a rendezvous that cannot
// answer, and the two guests must reach each other — not only the host. The
// address book is what carries them there: without it the room is a star, and
// a star is not the mesh the goal promises (bug-0136).
func TestTheRoomIsAMeshAndNotAStar(t *testing.T) {
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

	code := Invite{
		Community: invite.Community, Password: invite.Password, Key: invite.Key,
		Addrs: host.Addrs(),
	}.Encode()
	parsed, err := ParseInviteCode(code)
	if err != nil {
		t.Fatalf("the host's own code does not parse: %v", err)
	}

	// The room is what processes the greetings, so the room is what the
	// participants need: the mesh is the transport under it.
	hostRoom := NewRoom(host, host.SelfID(), "host", true, &Callbacks{})
	go hostRoom.Run(ctx)

	guest := func(name string) *Room {
		m := NewMesh(MeshOptions{
			Community: parsed.Community, Password: parsed.Password, Key: parsed.Key,
			Addrs: parsed.Addrs, Bootstrap: bootstrap, UserID: name,
		})
		t.Cleanup(func() { _ = m.Close() })
		if err := m.Join(ctx, false); err != nil {
			t.Fatalf("the guest %s could not join: %v", name, err)
		}
		room := NewRoom(m, m.SelfID(), name, false, &Callbacks{})
		go room.Run(ctx)
		return room
	}

	first := guest("guest-one")
	waitForRoster(t, ctx, hostRoom, 1, "the host never saw the first guest")

	second := guest("guest-two")
	waitForRoster(t, ctx, hostRoom, 2, "the host never saw the second guest")

	// And the two guests reach each other: the room is a mesh, not a star
	// around the host.
	waitForRoster(t, ctx, first, 2, "the first guest never saw the second one")
	waitForRoster(t, ctx, second, 2, "the second guest never saw the first one")
}

// waitForRoster waits until the room's roster holds the wanted number of
// people. The roster is who said hello and has not said bye: the room's own
// account of who is watching.
func waitForRoster(t *testing.T, ctx context.Context, room *Room, want int, msg string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if len(room.Peers()) >= want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s: %d in the roster, want %d", msg, len(room.Peers()), want)
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatalf("%s: %d in the roster, want %d", msg, len(room.Peers()), want)
}

// TestTheRoomFindsThePeerWhenTheAddressDoesNotWork is the fallback the goal
// promises: the room assembles always, and an address that stopped working —
// a changed one, a stale code — is not the end of it. The code named the
// peer, and the DHT can find a node by its id: a routing lookup, where the
// provider record is a search that the two sides answer differently.
func TestTheRoomFindsThePeerWhenTheAddressDoesNotWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	invite := testInvite()
	bed := startTestbed(t, ctx, invite)
	bootstrap := addrsToStrings(t, bed.AddrInfo())

	host := NewMesh(MeshOptions{
		Community: invite.Community, Password: invite.Password, Key: invite.Key,
		Bootstrap: bootstrap, UserID: "host",
	})
	t.Cleanup(func() { _ = host.Close() })
	if err := host.Join(ctx, true); err != nil {
		t.Fatalf("the host could not join: %v", err)
	}

	// A code that names the right peer at an address that cannot answer: the
	// port is closed, so the direct dial has nowhere to go.
	code := Invite{
		Community: invite.Community, Password: invite.Password, Key: invite.Key,
		Addrs: []string{fmt.Sprintf("/ip4/127.0.0.1/tcp/1/p2p/%s", host.SelfID())},
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

	waitForPeers(t, ctx, guest, 1, "the guest never reached the host")
}

// TestANodeThatNeverAnnouncedIsStillFindableByItsID pins the property the
// fallback leans on. The provider record is a search the two sides answer
// differently — that is bug-0129 and bug-0133 — while the routing table is
// not: a node that never announced anything is in nobody's provider list, and
// is still found by its id.
func TestANodeThatNeverAnnouncedIsStillFindableByItsID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inv := testInvite()
	bed := startTestbed(t, ctx, inv)
	quiet := startNode(t, inv)
	if err := quiet.Bootstrap(ctx, []peer.AddrInfo{bed.AddrInfo()}); err != nil {
		t.Fatalf("the node could not reach the testbed: %v", err)
	}
	if err := quiet.WaitReady(ctx); err != nil {
		t.Fatalf("the node's routing table stayed empty: %v", err)
	}
	// The node never announced: it published no provider record anywhere.

	found, err := bed.FindPeer(ctx, quiet.ID())
	if err != nil {
		t.Fatalf("a node that never announced was not found by its id: %v", err)
	}
	if found.ID != quiet.ID() {
		t.Errorf("found %s, want %s", found.ID, quiet.ID())
	}
	if len(found.Addrs) == 0 {
		t.Error("the node was found with no address to reach it at")
	}
}
