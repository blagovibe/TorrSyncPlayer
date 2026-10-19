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

// A room that cannot form is a fact, and the line says it in words that name
// what a person can change. What the window no longer does is decide on its
// own that a room has failed: the deadline that turned a long wait into "the
// room did not assemble" is gone, because the room assembles in seconds now
// and a verdict the application has no right to is worse than no verdict
// (idea-0131, bug-0137, bug-0128).
//
// The one failure it does name is the one it can see: the address in the
// invite code did not work, and the fallback that could have found the peer
// anyway is not there either.

// TestAFailureNamesWhatCanBeDoneAboutIt pins the failure sentence: the address
// that did not work, and the way in that is not there.
func TestAFailureNamesWhatCanBeDoneAboutIt(t *testing.T) {
	failed := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", InviteFailed: true}

	got := connStatusText(trEN, failed, false)
	assert.Contains(t, got, "the address in the code did not work",
		"a failure must name the address that failed: %q", got)
	assert.Contains(t, got, "fallback rendezvous",
		"a failure must name the way that is left: %q", got)
}

// TestAFailureIsNotClaimedWhileTheFallbackCanStillWork pins the other side:
// a failed dial is not a failed room while the fallback is up and looking.
func TestAFailureIsNotClaimedWhileTheFallbackCanStillWork(t *testing.T) {
	stillLooking := rtc.ConnStatus{
		SelfID:       "12D3KooWABCDEFGH",
		DHTReady:     true,
		Published:    true,
		InviteFailed: true,
	}

	got := connStatusText(trEN, stillLooking, false)
	assert.NotContains(t, got, "Not connected",
		"the fallback is still looking, and the room is not failed: %q", got)
	assert.Contains(t, got, "Connecting", "%q", got)
}

// TestAFailureIsNotClaimedWhenTheRoomIsFull pins the third case: a line about
// a failed connection on a screen where people are watching would be worse
// than no line.
func TestAFailureIsNotClaimedWhenTheRoomIsFull(t *testing.T) {
	met := rtc.ConnStatus{
		SelfID:       "12D3KooWABCDEFGH",
		DHTReady:     true,
		Published:    true,
		InviteFailed: true,
		Peers:        []rtc.ConnPeer{{ID: "b"}, {ID: "c"}},
	}

	got := connStatusText(trEN, met, false)
	assert.Contains(t, got, "Connected — 2 peers")
	assert.NotContains(t, got, "Not connected")
}

// The Russian sentence carries the same facts. A key that exists only in one
// language renders as the key itself, which is what the catalog's own drift
// test is there to prevent — this pins the sentence, not the key.
func TestTheFailureLineIsTranslated(t *testing.T) {
	tr := func(key string) string { return catalog.Lookup(catalog.LangRussian, key) }
	failed := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", InviteFailed: true}

	got := connStatusText(tr, failed, false)
	assert.Contains(t, got, "Не соединилось", "%q", got)
	assert.Contains(t, got, "адрес из кода не сработал", "%q", got)
}
