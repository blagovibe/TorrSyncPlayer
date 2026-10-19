// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
	"github.com/stretchr/testify/assert"
)

// A room that keeps counting forever is the last way this window can leave a
// person guessing. The wait has a measured length: against the public DHT the
// longest measured meeting took 4m35s, and two runs did not meet inside five
// and a half. So six minutes is where "slow" stops being slow and starts being
// something the person can act on — and the line has to say so in words that
// name what to do, because a number going up is not advice (bug-0128).

// TestAFailedMeetingSaysItFailed pins the end of the count.
func TestAFailedMeetingSaysItFailed(t *testing.T) {
	joined := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true, Published: true}

	got := connStatusText(trEN, joined, meetingFailedAfter+time.Second)
	assert.Contains(t, got, "did not meet", "a room that has waited this long must say so: %q", got)
	assert.NotContains(t, got, "Looking for people",
		"counting on after the room has failed is the guessing the goal forbids")

	// And it says what a person can do about it, in both directions: their
	// network, or a rendezvous of their own.
	assert.Contains(t, got, "rendezvous", "the advice must name the thing they can change: %q", got)
}

// TestTheRoomStillTellsYouItIsWaitingBeforeItGivesUp pins the other side of the
// same boundary: the failure line must not arrive early and turn ordinary
// waiting into an alarm.
func TestTheRoomStillTellsYouItIsWaitingBeforeItGivesUp(t *testing.T) {
	joined := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true, Published: true}

	justBefore := connStatusText(trEN, joined, meetingFailedAfter-time.Second)
	assert.Contains(t, justBefore, "Looking for people in the room",
		"a second before giving up is still waiting: %q", justBefore)
}

// TestAGuestWhoArrivedIsNotToldTheMeetingFailed pins the third case: the clock
// keeps running while the room is full, and a line about a failed meeting on a
// screen where three people are watching would be worse than no line.
func TestAGuestWhoArrivedIsNotToldTheMeetingFailed(t *testing.T) {
	met := rtc.ConnStatus{
		SelfID:    "12D3KooWABCDEFGH",
		DHTReady:  true,
		Published: true,
		Peers:     []rtc.ConnPeer{{ID: "b"}, {ID: "c"}},
	}

	got := connStatusText(trEN, met, 2*time.Hour)
	assert.Contains(t, got, "DHT ready — 2 peers")
	assert.NotContains(t, got, "did not meet")
}

// TestTheFailedMeetingLineIsTranslated pins the Russian sentence carries the
// same facts, including the advice.
func TestTheFailedMeetingLineIsTranslated(t *testing.T) {
	tr := func(key string) string { return catalog.Lookup(catalog.LangRussian, key) }
	joined := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true, Published: true}

	got := connStatusText(tr, joined, meetingFailedAfter+time.Second)
	assert.Contains(t, got, "не собралась", "%q", got)
	assert.Contains(t, got, "рандеву", "%q", got)
}
