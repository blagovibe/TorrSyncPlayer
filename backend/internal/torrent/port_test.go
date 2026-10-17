// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"fmt"
	"net"
	"testing"
	"time"
)

// The torrent client listens on TCP and on UDP with the same number, and the
// number used to be left to the client: it bound TCP on a random one and then
// bound UDP on the same number. Windows refuses a UDP bind inside a reserved
// range, the client retries only a "port taken" refusal, and the backend died
// with "access permissions" in the log — at random, about one run in three on a
// machine with WSL behind it (bug-0118).
//
// So the number is chosen here, and it has to be one both protocols accept.
// The test cannot fail on a machine without reserved ranges — it is a guard on
// the promise, not a reproduction — and the reproduction lives in the Windows
// job, which is where this was found.
func TestPickedListenPortIsFreeForBothProtocols(t *testing.T) {
	port, err := pickListenPort()
	if err != nil {
		t.Fatalf("pick a listen port: %v", err)
	}

	tcp, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("port %d was picked and will not bind for TCP: %v", port, err)
	}
	defer func() { _ = tcp.Close() }()

	udp, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("port %d was picked and will not bind for UDP: %v", port, err)
	}
	_ = udp.Close()
}

// TestTheServiceGetsAPortItCanHold says that the client is not left to pick
// one: a service that starts with a number of its own is a service that can
// fail to start, and the failure reads like the machine's fault.
//
// The number is checked by connecting to it, not by binding it: the client
// holds it while the service runs, and holding it is the point.
func TestTheServiceGetsAPortItCanHold(t *testing.T) {
	svc, err := NewServiceWithOptions(nil, ServiceOptions{MemoryStorageCapacity: 1 << 20})
	if err != nil {
		t.Fatalf("start the service: %v", err)
	}
	defer func() { _ = svc.Close() }()

	port := svc.ListenPort()
	if port == 0 {
		t.Fatal("the service has no port of its own, so the client chose one")
	}

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 5*time.Second)
	if err != nil {
		t.Fatalf("the client does not listen on the port %d it was given: %v", port, err)
	}
	_ = conn.Close()
}
