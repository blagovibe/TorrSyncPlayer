// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
)

// The seeker speaks clock: minutes under an hour, hours above it. Garbage —
// negative, NaN, infinity — reads as zero: a missing duration is ordinary
// while the file header is still arriving, and it must not crash the tick.
func TestTheSeekerSpeaksClock(t *testing.T) {
	for seconds, want := range map[float64]string{
		0:    "0:00",
		5:    "0:05",
		65:   "1:05",
		3599: "59:59",
		3600: "1:00:00",
		7542: "2:05:42",
		-3:   "0:00",
		1.9:  "0:01",
		59.9: "0:59",
	} {
		if got := formatClock(seconds); got != want {
			t.Errorf("formatClock(%v) = %q, want %q", seconds, got, want)
		}
	}
}

// A drag is a landing, not a spray: nothing commits without a target, a
// fresh touch is still aiming, and only the quiet delay turns a touch into
// a seek the room will hear.
func TestADragLandsOnce(t *testing.T) {
	now := time.Now()
	if seekReady(false, now.Add(-time.Hour), now) {
		t.Error("no target must never commit, however old the touch")
	}
	if seekReady(true, now, now) {
		t.Error("a fresh touch is still a drag, not a landing")
	}
	if !seekReady(true, now.Add(-2*seekDelay), now) {
		t.Error("a touch older than the quiet delay must commit")
	}
}

// The media line reports only what is known: no name, no size or no length
// reads as no line, not as a half line filling in piece by piece.
func TestTheMediaLineWaitsForAllThree(t *testing.T) {
	tr := func(key string) string { return catalog.Lookup(catalog.English, key) }

	if got := mediaInfoText(tr, "", 100, 60); got != "" {
		t.Errorf("no name must read as no line, got %q", got)
	}
	if got := mediaInfoText(tr, "Film.mkv", 0, 60); got != "" {
		t.Errorf("no size must read as no line, got %q", got)
	}
	if got := mediaInfoText(tr, "Film.mkv", 100, 0); got != "" {
		t.Errorf("no length must read as no line, got %q", got)
	}
	if got, want := mediaInfoText(tr, "Film.mkv", 1500000000, 7542),
		"Film.mkv · 1.4 GB · 2:05:42"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Redrawing the slider from the poll must not fire the handler: a redraw
// that seeks does it on the poll's behalf, twice a second, forever.
func TestRedrawingTheSliderSeeksNothing(t *testing.T) {
	_ = test.NewApp()

	fired := -1.0
	s := widget.NewSlider(0, 100)
	s.OnChanged = func(v float64) { fired = v }

	setSliderValue(s, s.OnChanged, 42)
	if fired != -1 {
		t.Errorf("a programmatic move fired the handler with %v", fired)
	}
	if s.Value != 42 {
		t.Errorf("the slider shows %v, want 42", s.Value)
	}
	if s.OnChanged == nil {
		t.Error("the handler must be reattached after the move")
	}
}
