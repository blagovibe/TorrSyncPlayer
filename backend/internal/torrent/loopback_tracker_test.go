// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE for full license text

package torrent

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/netip"
	"sync"

	trackerServer "github.com/anacrolix/torrent/tracker/server"
	"github.com/anacrolix/torrent/tracker/udp"

	httpTrackerServer "github.com/anacrolix/torrent/tracker/http/server"
)

// A tracker, so two clients on one machine can find each other.
//
// The anacrolix library ships the tracker *protocol* and a handler for serving
// it, but no tracker: `AnnounceTracker` has no implementation in the module. So
// there are two ways to put two clients in touch — write one of these, or give
// up and pretend the torrent metadata is the swarm.
//
// The node list in a .torrent, which looked like the cheap answer, is not: the
// library hands it to `AddDhtNodes`, and `NoDHT: true` means there is no DHT
// server to hand it to, so the address is dropped without a word. That is what
// the first version of this test did, and it produced exactly zero pieces in
// sixty seconds while looking like it was working.
//
// A tracker is also the more honest shape: this is what the project does in
// production, a client announces and learns peers from an announce URL.

// loopbackTracker is the smallest thing that satisfies the announce protocol:
// it remembers who announced which infohash and hands that list back.
type loopbackTracker struct {
	mu        sync.Mutex
	announces int
	peers     map[trackerServer.InfoHash]map[netip.AddrPort]struct{}
}

func (t *loopbackTracker) state() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	counts := make([]string, 0, len(t.peers))
	for hash, set := range t.peers {
		counts = append(counts, fmt.Sprintf("%s: %d", hash.HexString()[:8], len(set)))
	}
	return fmt.Sprintf("%d announces, %v", t.announces, counts)
}

func newLoopbackTracker() *loopbackTracker {
	return &loopbackTracker{peers: map[trackerServer.InfoHash]map[netip.AddrPort]struct{}{}}
}

func (t *loopbackTracker) TrackAnnounce(_ context.Context, req udp.AnnounceRequest, addr netip.AddrPort) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.announces++

	known, ok := t.peers[req.InfoHash]
	if !ok {
		known = map[netip.AddrPort]struct{}{}
		t.peers[req.InfoHash] = known
	}
	known[addr] = struct{}{}
	return nil
}

// Scrape answers nothing on purpose: nothing in this test asks, and a wrong
// answer would be worse than none.
func (t *loopbackTracker) Scrape(context.Context, []trackerServer.InfoHash) ([]udp.ScrapeInfohashResult, error) {
	return nil, nil
}

func (t *loopbackTracker) GetPeers(_ context.Context, infoHash trackerServer.InfoHash, _ trackerServer.GetPeersOpts, _ netip.AddrPort) trackerServer.ServerAnnounceResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	var result trackerServer.ServerAnnounceResult
	for addr := range t.peers[infoHash] {
		result.Peers = append(result.Peers, trackerServer.PeerInfo{AnnounceAddr: addr})
	}
	return result
}

// startLoopbackTracker runs the tracker over HTTP and returns its announce URL.
func startLoopbackTracker(t interface {
	Cleanup(func())
	Helper()
}) string {
	url, _ := startTracker(t)
	return url
}

func startTracker(t interface {
	Cleanup(func())
	Helper()
}) (string, *loopbackTracker) {
	tracker := newLoopbackTracker()
	handler := httpTrackerServer.Handler{
		Announce: &trackerServer.AnnounceHandler{AnnounceTracker: tracker},
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server.URL + "/announce", tracker
}
