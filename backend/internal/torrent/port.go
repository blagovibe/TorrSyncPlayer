// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"fmt"
	"net"
)

// pickListenPort asks the kernel for a number the torrent client can hold on
// both protocols.
//
// The client listens on TCP and on UDP — uTP — with the same number. Left to
// itself it takes a random one from the ephemeral range, binds TCP on it, and
// then binds UDP on the same number. Windows reserves large parts of the UDP
// range for itself (WSL, Hyper-V, Docker Desktop) and refuses a bind inside a
// reserved range with WSAEACCES — "an attempt was made to access a socket in a
// way forbidden by its access permissions", which says nothing about the
// cause. The client retries that refusal only when the port is *taken*, so a
// number Windows will not give to UDP kills the whole backend, and it does so
// at random: roughly one run in three on a machine with WSL behind it.
//
// So the number is chosen here instead, and chosen to be bindable on both
// protocols — the same question asked of the kernel as one question, with both
// answers held at once (bug-0118).
func pickListenPort() (int, error) {
	for attempt := 0; attempt < 12; attempt++ {
		// #nosec G102 -- every interface, on purpose: the torrent client
		// listens on every interface, because that is how peers reach it. A
		// probe on loopback would say nothing about the bind the client is
		// about to make, and the whole point of the probe is to ask the
		// kernel the question the client will ask.
		tcp, err := net.Listen("tcp", ":0")
		if err != nil {
			return 0, fmt.Errorf("pick a listen port: %w", err)
		}
		port := tcp.Addr().(*net.TCPAddr).Port

		// The shape the client will bind — every interface — and not a
		// narrower one: a UDP port free on loopback can still be reserved
		// everywhere. Both are held while the other is checked, so nothing can
		// take the number in between.
		udp, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
		_ = tcp.Close()
		if err != nil {
			continue
		}
		_ = udp.Close()
		return port, nil
	}

	return 0, fmt.Errorf("no port was free for both TCP and UDP")
}
