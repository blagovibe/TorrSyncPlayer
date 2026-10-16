// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

// The mesh is the room without a server: peers find each other over the DHT
// rendezvous and talk over libp2p streams. These tests run against an
// in-process DHT testbed — the public bootstrap is for manual runs, and a
// test that needs someone else's service is a test that fails for someone
// else's reasons (idea-0086).
//
// Rendezvous is retried with a fresh testbed per attempt. A reused testbed
// accumulates ghosts — closed peers whose provider records outlive their
// connections — and newcomers spend their dials on peers that will never
// answer. That is a property of rapid open/close cycling in tests, not of a
// room: the product opens one mesh per session and keeps it.

// TestTwoPeersExchangeAMagnetThroughTheMesh is the whole transport in one
// assertion: a host and a guest, a local testbed, and the magnet arriving over
// a stream. If this fails, nothing above it — rooms, sync, UI — can work.
func TestTwoPeersExchangeAMagnetThroughTheMesh(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	host, guests := joinMesh(t, ctx, 1)

	magnet := NewMagnet(Magnet{
		MagnetURI:    "magnet:?xt=urn:btih:abc",
		InfoHash:     "abc",
		SelectedFile: 0,
		Name:         "Movie.2024.mkv",
	})
	if err := host.Send(ctx, magnet); err != nil {
		t.Fatalf("the host could not send the magnet: %v", err)
	}

	got := guests[0].WaitFor(ctx, TypeMagnet, 30*time.Second)
	if got == nil {
		t.Fatal("the guest never received the magnet")
	}
	received, ok := got.(Magnet)
	if !ok {
		t.Fatalf("the guest received a %T, not a magnet", got)
	}
	want := magnet
	want.Type = TypeMagnet
	if received != want {
		t.Errorf("the magnet arrived changed: %+v", received)
	}
}

// TestAStrangerWithTheWrongPasswordJoinsNothing pins the admission rule: the
// room admits only peers that prove the password. A guest with the wrong
// password publishes presence like everyone else — presence is not authority —
// but no handshake passes either way, so it never sees a peer and never hears
// traffic, and the room never sees it.
func TestAStrangerWithTheWrongPasswordJoinsNothing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inv := testInvite()
	bed := startTestbed(t, ctx, inv)
	bootstrap := addrsToStrings(t, bed.AddrInfo())

	host, guests := joinMeshOn(t, ctx, bootstrap, 1)

	strangerOpts := MeshOptions{
		Community: inv.Community,
		Password:  "wrong-password",
		Key:       inv.Key,
		Bootstrap: bootstrap,
	}
	stranger := NewMesh(strangerOpts)
	defer func() { _ = stranger.Close() }()
	if err := stranger.Join(ctx, false); err != nil {
		t.Fatalf("presence publishes even for a stranger; the handshake refuses, not the join: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if len(stranger.Peers()) > 0 {
			t.Fatalf("a stranger with the wrong password sees peers: %v", stranger.Peers())
		}
		for _, id := range host.Peers() {
			if id == stranger.SelfID() {
				t.Fatalf("the room sees a stranger with the wrong password: %v", host.Peers())
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}

	if got := stranger.WaitFor(ctx, TypeMagnet, 5*time.Second); got != nil {
		t.Fatalf("a stranger with the wrong password heard traffic: %+v", got)
	}

	// The room itself keeps working with the stranger hanging around: the
	// host's magnet still reaches the guest, so a refused handshake is not a
	// broken room.
	magnet := NewMagnet(Magnet{MagnetURI: "magnet:?xt=urn:btih:abc", InfoHash: "abc"})
	if err := host.Send(ctx, magnet); err != nil {
		t.Fatalf("the host could not send with a stranger present: %v", err)
	}
	if got := guests[0].WaitFor(ctx, TypeMagnet, 30*time.Second); got == nil {
		t.Fatal("the guest never received the magnet with a stranger present")
	}
}

// TestAHostCommandReachesEveryGuest checks the fan-out the server used to do:
// one send from the host, received by everyone connected. The mesh has no
// broker, so a send that reaches one peer and not the other is a room that
// watches two different films.
func TestAHostCommandReachesEveryGuest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	host, guests := joinMesh(t, ctx, 2)

	if err := host.Send(ctx, NewPause(true)); err != nil {
		t.Fatalf("the host could not send pause: %v", err)
	}

	for i, guest := range guests {
		got := guest.WaitFor(ctx, TypePause, 30*time.Second)
		if got == nil {
			t.Errorf("guest %d never received the pause", i)
			continue
		}
		if pause, ok := got.(Pause); !ok || !pause.Pause {
			t.Errorf("guest %d received %+v, not a pause", i, got)
		}
	}
}

// joinMesh builds one host and the given number of guests on a fresh testbed,
// and returns once the host sees them all. Joining returns once presence is
// published; the peers knowing each other takes discovery on top, so a send
// before convergence lands nowhere — convergence is part of joining here, not
// an afterthought in each test.
//
// Rendezvous is retried with a fresh testbed per attempt, up to three. A
// reused testbed keeps ghosts of closed peers, and a newcomer that dials
// ghosts never hears back. Fresh state per attempt keeps one attempt's weather
// out of the next one's.
func joinMesh(t *testing.T, ctx context.Context, guests int) (*Mesh, []*Mesh) {
	t.Helper()

	for attempt := 0; attempt < 3; attempt++ {
		bed := startTestbed(t, ctx, testInvite())
		host, gs, err := tryJoinMesh(t, ctx, addrsToStrings(t, bed.AddrInfo()), guests)
		if err == nil {
			t.Cleanup(func() {
				_ = host.Close()
				for _, g := range gs {
					_ = g.Close()
				}
			})
			return host, gs
		}
		t.Logf("rendezvous attempt %d failed: %v", attempt+1, err)
	}
	t.Fatal("rendezvous never succeeded in 3 attempts")
	return nil, nil
}

// joinMeshOn is joinMesh on someone else's testbed: for the stranger, whose
// presence must publish into the room's rendezvous to prove it hears nothing.
func joinMeshOn(t *testing.T, ctx context.Context, bootstrap string, guests int) (*Mesh, []*Mesh) {
	t.Helper()

	for attempt := 0; attempt < 3; attempt++ {
		host, gs, err := tryJoinMesh(t, ctx, bootstrap, guests)
		if err == nil {
			t.Cleanup(func() {
				_ = host.Close()
				for _, g := range gs {
					_ = g.Close()
				}
			})
			return host, gs
		}
		t.Logf("rendezvous attempt %d failed: %v", attempt+1, err)
	}
	t.Fatal("rendezvous never succeeded in 3 attempts")
	return nil, nil
}

func tryJoinMesh(t *testing.T, ctx context.Context, bootstrap string, guests int) (*Mesh, []*Mesh, error) {
	t.Helper()
	inv := testInvite()

	host := NewMesh(MeshOptions{
		Community: inv.Community,
		Password:  inv.Password,
		Key:       inv.Key,
		Bootstrap: bootstrap,
	})
	if err := host.Join(ctx, true); err != nil {
		_ = host.Close()
		return nil, nil, err
	}

	gs := make([]*Mesh, guests)
	for i := range gs {
		gs[i] = NewMesh(MeshOptions{
			Community: inv.Community,
			Password:  inv.Password,
			Key:       inv.Key,
			Bootstrap: bootstrap,
		})
		if err := gs[i].Join(ctx, false); err != nil {
			_ = host.Close()
			for _, g := range gs[:i] {
				_ = g.Close()
			}
			return nil, nil, err
		}
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		seen := 0
		for _, g := range gs {
			if len(g.Peers()) > 0 {
				seen++
			}
		}
		if len(host.Peers()) >= guests && seen == guests {
			return host, gs, nil
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	_ = host.Close()
	for _, g := range gs {
		_ = g.Close()
	}
	return nil, nil, errNoConvergence
}

// addrsToStrings renders a testbed's address for MeshOptions.Bootstrap.
func addrsToStrings(t *testing.T, info peer.AddrInfo) string {
	t.Helper()
	addrs, err := peer.AddrInfoToP2pAddrs(&info)
	require.NoError(t, err)
	out := ""
	for i, a := range addrs {
		if i > 0 {
			out += ","
		}
		out += a.String()
	}
	return out
}

var errNoConvergence = errorString("the mesh never converged")

type errorString string

func (e errorString) Error() string { return string(e) }
