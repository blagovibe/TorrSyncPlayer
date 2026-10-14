// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

// The tray is the one part of the application no unit test can see: there is no
// desktop in CI, and systray says nothing when a platform refuses it. What can
// be tested is the decision around it — and the icon, which is code like any
// other and turned out to be exactly the kind of thing that is never run.

// TestCloseToTrayDecision states the rule a person notices at once if it is
// wrong: with a tray, closing the window leaves the download running; without
// one, closing the window ends the application, because there would then be no
// way to bring it back.
//
// The decision is a function because the behaviour it governs cannot be
// exercised in a test: Fyne's test driver closes windows unconditionally and
// never consults the close intercept, so a test that called Close would pass
// whatever this returned.
func TestCloseToTrayDecision(t *testing.T) {
	if !hideOnClose(true) {
		t.Error("a window with a tray must survive being closed — that is the tray's purpose")
	}
	if hideOnClose(false) {
		t.Error("a window with no tray has nothing to bring it back, so closing it must end the application")
	}
}

// TestTrayRunningIsFalseForAMissingTray is the same statement about the
// predicate the decision reads, including the nil tray that --no-tray and a
// desktop refusing tray icons both produce.
func TestTrayRunningIsFalseForAMissingTray(t *testing.T) {
	var missing *Tray
	if missing.Running() {
		t.Error("a tray that was never created reported itself as running")
	}
	if (&Tray{}).Running() {
		t.Error("a tray whose platform gave it no icon reported itself as running")
	}
	if !(&Tray{running: true}).Running() {
		t.Error("a tray with an icon on screen reported itself as absent")
	}
}

// TestTrayShutdownIsAlwaysSafe covers the cleanup path. It is deferred, so a
// panic there would replace whatever actually went wrong with a confusing
// crash — and it runs on the exit that follows a failure, which is the worst
// moment to add one.
func TestTrayShutdownIsAlwaysSafe(t *testing.T) {
	var missing *Tray
	missing.Shutdown()
	(&Tray{}).Shutdown()
}

// TestTrayIconIsDrawable checks the icon is a real PNG with a disc in the middle
// and nothing in the corner.
//
// Worth checking because a tray with no icon is refused outright by some
// desktops: a broken encoder here would not look like a missing picture, it
// would look like the application failing to start.
func TestTrayIconIsDrawable(t *testing.T) {
	raw := trayIcon()
	if len(raw) == 0 {
		t.Fatal("the tray icon is empty, and some desktops refuse a tray without one")
	}

	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("the tray icon is not a readable PNG: %v", err)
	}
	if got := img.Bounds().Dx(); got != 32 || img.Bounds().Dy() != 32 {
		t.Fatalf("the icon is %dx%d, and 32 is what the drawing assumes",
			img.Bounds().Dx(), img.Bounds().Dy())
	}

	if alphaAt(t, img, 16, 16) == 0 {
		t.Fatal("the middle is transparent: the disc was never drawn")
	}
	if corner := alphaAt(t, img, 0, 0); corner != 0 {
		t.Fatalf("the corner is %d%% opaque: the disc has no edge, so the tray would show a square",
			corner*100/255)
	}
}

// TestTrayIconIsCached guards the once-value. An icon that re-encoded on every
// call would still be correct and still be invisible in every other test, which
// is how waste survives.
func TestTrayIconIsCached(t *testing.T) {
	first, second := trayIcon(), trayIcon()
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("no icon")
	}
	if &first[0] != &second[0] {
		t.Error("the icon was encoded twice instead of once")
	}
}

// TestInsideTriangleIsNotAFunctionOfEverything guards the one piece of real
// geometry in the icon. It is easy to write a point-in-triangle test that
// returns true for everything, which would turn the play triangle into a white
// square — and the icon tests above would all still pass.
func TestInsideTriangleIsNotAFunctionOfEverything(t *testing.T) {
	// A unit triangle with vertices (0,0), (10,0), (0,10).
	const ax, ay, bx, by, cx, cy = 0.0, 0.0, 10.0, 0.0, 0.0, 10.0

	if !insideTriangle(1, 1, ax, ay, bx, by, cx, cy) {
		t.Error("(1,1) is inside the triangle")
	}
	if insideTriangle(9, 9, ax, ay, bx, by, cx, cy) {
		t.Error("(9,9) is outside the triangle — the test accepts anything")
	}
	if insideTriangle(-1, 5, ax, ay, bx, by, cx, cy) {
		t.Error("(-1,5) is left of the triangle")
	}
	if !insideTriangle(5, 5, ax, ay, bx, by, cx, cy) {
		t.Error("(5,5) is on the hypotenuse, and belongs to the triangle")
	}
}

func alphaAt(t *testing.T, img image.Image, x, y int) uint32 {
	t.Helper()
	_, _, _, a := img.At(x, y).RGBA()
	return a >> 8
}
