// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"sync"
	"testing"
)

// A tracker, so two real backends on one machine can find each other.
//
// The library ships the tracker *protocol* and a handler for serving it, but no
// tracker: anacrolix's `AnnounceTracker` has no implementation in the module. So
// there are two ways to put two clients in touch — write one, or give up and
// pretend the torrent metadata is the swarm.
//
// This one is written by hand rather than by linking anacrolix into the client's
// tests: the announcement is two query parameters and the answer is a bencoded
// list of peers, and a dependency that exists only to hand one list over is not
// worth the coupling. The same reasoning as internal/buffer being ours.
//
// It answers in the *dictionary* model rather than the compact one because that
// is what the client parses: `Peers.List` is what gets read, compact is sent in
// the request and ignored in the answer.

// loopbackTracker remembers who announced which infohash and hands the list back.
type loopbackTracker struct {
	mu        sync.Mutex
	announces int
	peers     map[string][]netip.AddrPort
}

func (t *loopbackTracker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	infoHash := r.URL.Query().Get("info_hash")
	port, _ := strconv.Atoi(r.URL.Query().Get("port"))

	if infoHash != "" && port > 0 {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			if addr, err := netip.ParseAddr(host); err == nil {
				t.mu.Lock()
				if t.peers == nil {
					t.peers = map[string][]netip.AddrPort{}
				}
				t.announces++
				t.peers[infoHash] = appendAddr(t.peers[infoHash], netip.AddrPortFrom(addr, uint16(port)))
				t.mu.Unlock()
			}
		}
	}

	t.mu.Lock()
	known := t.peers[infoHash]
	t.mu.Unlock()

	// Compact, not the dictionary model: the client asks for compact with
	// `compact=1`, and it is the only form in which the IP really is binary. The
	// dict model takes `ip` as *text*, so four raw bytes there parse as nil and
	// the peer is offered with no address — a tracker that answers perfectly and
	// connects nobody, which is what the first version of this one did.
	var compact []byte
	for _, p := range known {
		ip := p.Addr().As4()
		var port [2]byte
		binary.BigEndian.PutUint16(port[:], p.Port())
		compact = append(compact, ip[:]...)
		compact = append(compact, port[:]...)
	}

	// A short interval on purpose: the leecher must learn about the seeder
	// quickly, and the test cannot wait for a real tracker's half hour.
	body := bencodeValue(map[string]interface{}{
		"interval": 1,
		"peers":    string(compact),
	})
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write(body)
}

// appendAddr adds an address unless it is already there: a peer that announces
// twice must not be offered to itself twice, and a client that sees itself in the
// answer may try to connect to itself.
func appendAddr(have []netip.AddrPort, addr netip.AddrPort) []netip.AddrPort {
	for _, a := range have {
		if a == addr {
			return have
		}
	}
	return append(have, addr)
}

// state is what the test asserts when the swarm does not come up.
func (t *loopbackTracker) state() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	counts := make([]string, 0, len(t.peers))
	for hash, set := range t.peers {
		counts = append(counts, fmt.Sprintf("%s: %d", hash[:8], len(set)))
	}
	return fmt.Sprintf("%d announces, %v", t.announces, counts)
}

// startLoopbackTracker runs the tracker over HTTP and returns its announce URL.
func startLoopbackTracker(t *testing.T) (string, *loopbackTracker) {
	t.Helper()

	tracker := &loopbackTracker{}
	server := httptest.NewServer(tracker)
	t.Cleanup(server.Close)
	return server.URL + "/announce", tracker
}
