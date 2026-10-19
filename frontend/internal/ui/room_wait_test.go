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

// A room that has announced itself and found nobody yet is not a room that is
// finished: on the public DHT the meeting takes minutes, and the window used to
// say nothing at all while it happened. A person waiting for a guest was
// looking at a line that meant "everything is fine" for four of those minutes,
// which is the guessing the goal forbids — only slower and quieter (bug-0128).

func TestAQuietRoomSaysHowLongItHasBeenWaiting(t *testing.T) {
	joined := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true, Published: true}

	got := connStatusText(trEN, joined, 7*time.Second)
	assert.Contains(t, got, "Looking for people in the room",
		"a room with nobody in it must say it is looking: %q", got)
	assert.Contains(t, got, "0:07", "the wait must be counted, not merely admitted: %q", got)

	// And once it is long enough to be worth explaining, the line says why it
	// is long instead of only how long.
	long := connStatusText(trEN, joined, 3*time.Minute)
	assert.Contains(t, long, "a couple of minutes", "a long wait needs an explanation: %q", long)
}

// TestTheWaitIsOnlyShownWhileThereIsNobody pins the other side: once a peer is
// in the room, the line is the roster's job again. A timer that stays on screen
// after the guest arrived is noise where the truth used to be.
func TestTheWaitIsOnlyShownWhileThereIsNobody(t *testing.T) {
	met := rtc.ConnStatus{
		SelfID:    "12D3KooWABCDEFGH",
		DHTReady:  true,
		Published: true,
		Peers:     []rtc.ConnPeer{{ID: "b"}},
	}

	got := connStatusText(trEN, met, 3*time.Minute)
	assert.Contains(t, got, "DHT ready — 1 peers (1 direct, 0 relayed)")
	assert.NotContains(t, got, "Looking for people")
}

// TestTheWaitIsNotShownBeforeTheRoomIsEvenUp pins the order of the three
// problems the line can report. "Looking for people" is the healthiest of them
// and must never stand in for a rendezvous that is not ready or a room that
// never announced itself: those two need a different sentence entirely.
func TestTheWaitIsNotShownBeforeTheRoomIsEvenUp(t *testing.T) {
	notReady := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: false}
	assert.Equal(t, "Finding the room — DHT not ready yet", connStatusText(trEN, notReady, time.Minute))

	unannounced := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true}
	assert.Contains(t, connStatusText(trEN, unannounced, time.Minute), "did not reach the network")
}

// TestTheWaitIsCountedInMinutesAndSeconds pins the shape of the counter: a
// clock a person can read at a glance, and one that does not roll into hours
// while they are still waiting.
func TestTheWaitIsCountedInMinutesAndSeconds(t *testing.T) {
	assert.Equal(t, "0:00", fmtWaited(0))
	assert.Equal(t, "0:09", fmtWaited(9*time.Second))
	assert.Equal(t, "1:05", fmtWaited(65*time.Second))
	assert.Equal(t, "59:59", fmtWaited(59*time.Minute+59*time.Second))
	assert.Equal(t, "60:00", fmtWaited(time.Hour))
}

// The Russian line must carry the same facts. A key that exists only in one
// language renders as the key itself, which is what the catalog's own drift
// test is there to prevent — this pins the sentence, not the key.
func TestTheWaitingLineIsTranslated(t *testing.T) {
	tr := func(key string) string { return catalog.Lookup(catalog.LangRussian, key) }
	joined := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true, Published: true}

	got := connStatusText(tr, joined, 2*time.Minute)
	assert.Contains(t, got, "Ищем участников комнаты", "%q", got)
	assert.Contains(t, got, "2:00", "%q", got)
}
