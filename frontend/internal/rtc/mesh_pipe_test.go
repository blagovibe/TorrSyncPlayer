// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)

// These tests drive the mesh without any network: a net.Pipe pair stands in
// for the stream, and the peer is injected directly. Rendezvous — DHT,
// handshakes, relays — is a different test (mesh_test.go), because it fails
// for the weather: CPU saturation, closed ports, empty routing tables.
// Everything here fails only when the mesh logic is wrong.

// pipePeer injects a peer whose far end the test holds.
//
// The hello the injection sends is read concurrently: a pipe is synchronous,
// and the write blocks until someone reads — someone who only appears after
// the injection returns. A peer that is never announced is a roster entry that
// never appears, so the hello is asserted, not skipped.
func pipePeer(t *testing.T, m *Mesh, peerID string) net.Conn {
	t.Helper()

	local, remote := net.Pipe()
	t.Cleanup(func() {
		_ = local.Close()
		_ = remote.Close()
	})
	helloed := make(chan error, 1)
	go func() {
		_ = remote.SetReadDeadline(time.Now().Add(5 * time.Second))
		var hello Hello
		if err := json.NewDecoder(remote).Decode(&hello); err != nil {
			helloed <- err
			return
		}
		if hello.Type != TypeHello {
			helloed <- fmt.Errorf("first message is %q, not a hello", hello.Type)
			return
		}
		helloed <- nil
	}()
	// A pipe stands in for an admitted stream, so the remote peer opened it.
	m.addPeer(peerID, local, peerID, PathInbound)
	if err := <-helloed; err != nil {
		t.Fatalf("no hello from the mesh: %v", err)
	}
	return remote
}

// TestMeshDeliversWhatThePeerSends checks the whole receive path: lines off the
// transport, parsed, queued, and handed out by type.
func TestMeshDeliversWhatThePeerSends(t *testing.T) {
	m := NewMesh(MeshOptions{})
	m.selfID = "host"
	remote := pipePeer(t, m, "guest")
	defer func() { _ = m.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	send := func(v any) {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		_, _ = remote.Write(append(raw, '\n'))
	}
	send(NewPause(true))
	send(NewPosition(12.5))

	got := m.WaitFor(ctx, TypePause, 5*time.Second)
	pause, ok := got.(Pause)
	if !ok || !pause.Pause {
		t.Fatalf("waited for a pause, got %+v", got)
	}
	got = m.WaitFor(ctx, TypePosition, 5*time.Second)
	position, ok := got.(Position)
	if !ok || position.Position != 12.5 {
		t.Fatalf("waited for a position, got %+v", got)
	}
}

// TestMeshSkipsGarbageInsteadOfDying is the corrupted-line half of Parse: one
// bad line must not kill the stream behind it.
func TestMeshSkipsGarbageInsteadOfDying(t *testing.T) {
	m := NewMesh(MeshOptions{})
	m.selfID = "host"
	remote := pipePeer(t, m, "guest")
	defer func() { _ = m.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _ = remote.Write([]byte("this is not json\n"))
	_, _ = remote.Write([]byte(`{"type":"teleport","to":999}` + "\n"))
	raw, _ := json.Marshal(NewPause(false))
	_, _ = remote.Write(append(raw, '\n'))

	got := m.WaitFor(ctx, TypePause, 5*time.Second)
	if _, ok := got.(Pause); !ok {
		t.Fatalf("a pause behind two bad lines never arrived: %+v", got)
	}
}

// TestMeshSendReachesThePeer checks the send path: one encode per peer, under
// that peer's lock, so two concurrent sends cannot interleave into neither.
func TestMeshSendReachesThePeer(t *testing.T) {
	m := NewMesh(MeshOptions{})
	m.selfID = "host"
	remote := pipePeer(t, m, "guest")
	defer func() { _ = m.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- m.Send(ctx, NewMagnet(Magnet{MagnetURI: "magnet:?xt=urn:btih:abc", InfoHash: "abc"}))
	}()

	_ = remote.SetReadDeadline(time.Now().Add(5 * time.Second))
	var magnet Magnet
	if err := json.NewDecoder(remote).Decode(&magnet); err != nil {
		t.Fatalf("the peer never received the send: %v", err)
	}
	if magnet.MagnetURI != "magnet:?xt=urn:btih:abc" {
		t.Errorf("the magnet arrived changed: %+v", magnet)
	}
	if err := <-done; err != nil {
		t.Errorf("send reported an error for a delivered message: %v", err)
	}
}

// TestMeshSaysByeOnClose pins the graceful leave: the far end learns at once,
// instead of when the silence is noticed.
func TestMeshSaysByeOnClose(t *testing.T) {
	m := NewMesh(MeshOptions{})
	m.selfID = "host"
	remote := pipePeer(t, m, "guest")

	// The bye is sent during Close, so the reader has to already be waiting:
	// nobody reads after Close returns, because the connections are closed by
	// then and there is nothing left to read.
	byeArrived := make(chan Bye, 1)
	readErr := make(chan error, 1)
	go func() {
		_ = remote.SetReadDeadline(time.Now().Add(5 * time.Second))
		var bye Bye
		if err := json.NewDecoder(remote).Decode(&bye); err != nil {
			readErr <- err
			return
		}
		byeArrived <- bye
	}()

	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case bye := <-byeArrived:
		if bye.PeerID != "host" {
			t.Errorf("the bye names %q", bye.PeerID)
		}
	case err := <-readErr:
		t.Fatalf("no bye on close: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("no bye on close: nothing arrived")
	}
}
