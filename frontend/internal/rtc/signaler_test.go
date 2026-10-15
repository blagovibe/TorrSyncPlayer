// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	wrtcsgl "github.com/pojntfx/weron/pkg/wrtcsgl"
)

// testSignaler is a weron signaler running in this process.
//
// The public signaler is for manual runs. A test that needs someone else's
// service fails for someone else's reasons, so the mesh tests introduce their
// peers through one of these instead: same protocol, loopback address, no
// network past the machine.
type testSignaler struct {
	url      string
	signaler interface {
		Close() error
	}
	cancel context.CancelFunc
}

// URL is the signaler address peers connect to.
func (s *testSignaler) URL() string { return s.url }

// startTestSignaler runs a signaler on a loopback port and waits until it
// answers, so the first Join does not race the listen.
func startTestSignaler(t *testing.T) *testSignaler {
	t.Helper()

	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	ctx, cancel := context.WithCancel(context.Background())
	// The signaler needs a heartbeat: with a zero config it builds a zero
	// ticker and panics on the first connection. The CLI defaults to ten
	// seconds; the test uses less, because nothing here waits that long.
	signaler := wrtcsgl.NewSignaler(addr, "", "", &wrtcsgl.SignalerConfig{
		Heartbeat:            2 * time.Second,
		EphemeralCommunities: true,
	}, ctx)
	if err := signaler.Open(); err != nil {
		cancel()
		t.Fatalf("open test signaler: %v", err)
	}
	t.Cleanup(func() {
		_ = signaler.Close()
		cancel()
	})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return &testSignaler{url: "ws://" + addr, signaler: signaler, cancel: cancel}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the test signaler never started listening")
	return nil
}

func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}
