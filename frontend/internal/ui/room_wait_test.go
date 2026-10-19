// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
	"github.com/stretchr/testify/assert"
)

// The connection line says what is happening, not how long it has been
// happening. The room assembles in seconds now — by the address in the code —
// so a counter that grows while a person waits for a guest to paste it is
// noise where the truth used to be, and a deadline that turns silence into
// "the room did not assemble" is a verdict the application has no right to
// (idea-0131, bug-0137).

// TestTheLineSaysWhatIsHappening pins the three states a room can be in:
// connecting, connected, and not connected with the cause spelled out.
func TestTheLineSaysWhatIsHappening(t *testing.T) {
	connecting := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true}
	assert.Equal(t, "Connecting…", connStatusText(trEN, connecting, false))

	connected := rtc.ConnStatus{
		SelfID:   "12D3KooWABCDEFGH",
		DHTReady: true,
		Peers: []rtc.ConnPeer{
			{ID: "a", Via: rtc.PathInvite},
			{ID: "b", Relayed: true, Via: rtc.PathInvite},
		},
	}
	assert.Equal(t, "Connected — 2 peers (direct 1, through a relay 1), you 12D3KooW",
		connStatusText(trEN, connected, false))

	failed := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", InviteFailed: true}
	assert.Contains(t, connStatusText(trEN, failed, false), "the address in the code did not work",
		"a failure must name the cause: %q", connStatusText(trEN, failed, false))
}

// TestTheLineNeverCounts pins what is gone: no counter, no deadline, no
// "the first meeting can take a couple of minutes". The room waits for a
// person, and a person does not need a stopwatch to know how long they have
// been standing there.
func TestTheLineNeverCounts(t *testing.T) {
	joined := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true}
	got := connStatusText(trEN, joined, true)
	assert.NotContains(t, got, "0:07")
	assert.NotContains(t, got, "Looking for people in the room")
	assert.NotContains(t, got, "a couple of minutes")
	assert.NotContains(t, got, "did not meet")
}

// TestTheHostWaitsForAPersonNotForTheNetwork pins the difference between the
// two sides of the same moment. A host with an empty room is waiting for
// someone to paste the code; a guest with an empty room is connecting. The
// line said "looking for people" to both, which was true of neither.
func TestTheHostWaitsForAPersonNotForTheNetwork(t *testing.T) {
	joined := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true}
	assert.Contains(t, connStatusText(trEN, joined, true), "waiting for someone to join")
	assert.Contains(t, connStatusText(trEN, joined, false), "Connecting")
}

// TestTheFallbackIsSaidWhenItIsDown pins the one fact that changes what a
// person should do next: the fallback rendezvous is how a peer without an
// address finds this one, and its absence is worth saying out loud. When it
// is up, it is silence.
func TestTheFallbackIsSaidWhenItIsDown(t *testing.T) {
	down := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: false}
	assert.Contains(t, connStatusText(trEN, down, true), "the fallback rendezvous is not up")

	up := rtc.ConnStatus{
		SelfID:   "12D3KooWABCDEFGH",
		DHTReady: true,
		Peers:    []rtc.ConnPeer{{ID: "a"}},
	}
	assert.NotContains(t, connStatusText(trEN, up, false), "rendezvous")
}

// The Russian line must carry the same facts. A key that exists only in one
// language renders as the key itself, which is what the catalog's own drift
// test is there to prevent — this pins the sentence, not the key.
func TestTheConnectionLineIsTranslated(t *testing.T) {
	tr := func(key string) string { return catalog.Lookup(catalog.LangRussian, key) }
	joined := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true}

	assert.Equal(t, "Подключаемся…", connStatusText(tr, joined, false))
	assert.Contains(t, connStatusText(tr, joined, true), "ждём, кто подключится")
}

// TestTheLineSaysHowEachPeerWasReached pins the paths the goal promises: how a
// peer is connected is a different fact from whether it is connected at all,
// and the one worth watching is the fallback — a peer that had to be looked up
// by its id is a peer the fast path did not reach.
//
// The three paths are the goal's own list: direct, through a relay, and
// through the DHT fallback. A peer that dialled us is a direct connection —
// how it found the room is its business, not ours.
func TestTheLineSaysHowEachPeerWasReached(t *testing.T) {
	room := rtc.ConnStatus{
		SelfID:   "12D3KooWABCDEFGH",
		DHTReady: true,
		Peers: []rtc.ConnPeer{
			{ID: "a", Via: rtc.PathInvite},
			{ID: "b", Via: rtc.PathRendezvous},
			{ID: "c", Via: rtc.PathInbound},
			{ID: "d", Relayed: true, Via: rtc.PathInvite},
		},
	}

	got := connStatusText(trEN, room, false)
	assert.Contains(t, got, "direct 2", "the directly connected peers must be counted: %q", got)
	assert.Contains(t, got, "via the fallback 1", "the peer found by the id must be counted: %q", got)
	assert.Contains(t, got, "through a relay 1", "the relayed peer must be counted: %q", got)
}
