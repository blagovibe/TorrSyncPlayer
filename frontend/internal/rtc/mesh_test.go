// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"testing"
	"time"
)

// The mesh is the room without a server: peers find each other through a weron
// signaler and talk over WebRTC data channels. These tests run against a
// signaler in this same process — the public one is for manual runs, and a test
// that needs someone else's service is a test that fails for someone else's
// reasons (idea-0064).
//
// Rendezvous is retried with a fresh signaler per attempt. A reused signaler
// accumulates ghosts — closed peers whose membership outlives their connection
// — and newcomers spend their offers on peers that will never answer. That is a
// property of rapid open/close cycling in tests, not of a room: the product
// opens one adapter per session and keeps it.

// TestTwoPeersExchangeAMagnetThroughTheMesh is the whole transport in one
// assertion: a host and a guest, a local signaler, and the magnet arriving over
// a data channel. If this fails, nothing above it — rooms, sync, UI — can work.
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
// signaler introduces only peers that share the community *and* the password. A
// guest with the wrong password must never see the room's traffic.
func TestAStrangerWithTheWrongPasswordJoinsNothing(t *testing.T) {
	signaler := startTestSignaler(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	host := NewMesh(MeshOptions{
		Signaler:  signaler.URL(),
		Community: "test-room",
		Password:  "test-password",
		Key:       "test-key",
	})
	defer func() { _ = host.Close() }()
	if err := host.Join(ctx, true); err != nil {
		t.Fatalf("the host could not join the mesh: %v", err)
	}

	stranger := NewMesh(MeshOptions{
		Signaler:  signaler.URL(),
		Community: "test-room",
		Password:  "wrong-password",
		Key:       "test-key",
	})
	defer func() { _ = stranger.Close() }()
	// Joining with the wrong password must fail rather than leave a peer that
	// hears nothing: a silent peer is a roster entry that never speaks.
	if err := stranger.Join(ctx, false); err == nil {
		t.Fatal("a stranger with the wrong password joined the mesh")
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

// joinMesh builds one host and the given number of guests on a fresh signaler,
// and returns once the host sees them all. Joining returns once the signaler
// knows a peer; the peers knowing each other takes ICE on top, so a send before
// convergence lands nowhere — convergence is part of joining here, not an
// afterthought in each test.
//
// Rendezvous is retried with a fresh signaler per attempt, up to three. A reused
// signaler keeps ghosts of closed peers, and a newcomer that offers to ghosts
// never hears back. Fresh state per attempt keeps one attempt's weather out of
// the next one's.
func joinMesh(t *testing.T, ctx context.Context, guests int) (*Mesh, []*Mesh) {
	t.Helper()

	for attempt := 0; attempt < 3; attempt++ {
		host, gs, err := tryJoinMesh(t, ctx, guests)
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

func tryJoinMesh(t *testing.T, ctx context.Context, guests int) (*Mesh, []*Mesh, error) {
	t.Helper()
	signaler := startTestSignaler(t)

	host := NewMesh(MeshOptions{
		Signaler:  signaler.URL(),
		Community: "test-room",
		Password:  "test-password",
		Key:       "test-key",
	})
	if err := host.Join(ctx, true); err != nil {
		_ = host.Close()
		return nil, nil, err
	}

	gs := make([]*Mesh, guests)
	for i := range gs {
		gs[i] = NewMesh(MeshOptions{
			Signaler:  signaler.URL(),
			Community: "test-room",
			Password:  "test-password",
			Key:       "test-key",
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

var errNoConvergence = errorString("the mesh never converged")

type errorString string

func (e errorString) Error() string { return string(e) }
