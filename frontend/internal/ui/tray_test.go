// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
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

// TestTheTrayMenuCarriesOpenSettingsAndQuit pins what the menu says. The tray
// is the only way back into a closed window, so the settings the window carries
// have to be reachable from it too (owner's decision, idea-0112): a menu with
// only Open and Quit hides half the application exactly when the window is gone.
func TestTheTrayMenuCarriesOpenSettingsAndQuit(t *testing.T) {
	menu := trayMenu("en", func() {}, func() {}, func() {})

	var labels []string
	for _, item := range menu.Items {
		if item.IsSeparator {
			labels = append(labels, "---")
			continue
		}
		labels = append(labels, item.Label)
	}
	want := []string{"Open", "Settings", "---", "Quit"}
	if strings.Join(labels, "|") != strings.Join(want, "|") {
		t.Fatalf("the tray menu is %v, want %v", labels, want)
	}

	// The toolkit appends an exit of its own unless the last item says it is
	// the exit, which it decides by comparing the label with its own
	// translation — a comparison «Выход» cannot pass in Russian. Proven off a
	// running tray, where two exits sat one under the other.
	last := menu.Items[len(menu.Items)-1]
	if !last.IsQuit {
		t.Error("the last tray item is not marked as the exit, so a second one will appear")
	}
}

// TestTheTrayMenuOpensTheSettingsRatherThanTheWindow says the two top items do
// different things. One that opened the window as well would make Settings a
// slower way to reach the same place, and a person who closed the window on
// purpose would have to open it again to change anything.
func TestTheTrayMenuOpensTheSettingsRatherThanTheWindow(t *testing.T) {
	var opened, settings int
	menu := trayMenu("en",
		func() { opened++ },
		func() { settings++ },
		func() {})

	for _, item := range menu.Items {
		if item.Label == "Settings" {
			item.Action()
		}
	}
	if settings != 1 {
		t.Errorf("choosing Settings from the tray called it %d times", settings)
	}
	if opened != 0 {
		t.Errorf("choosing Settings from the tray also opened the window %d times", opened)
	}
}

// TestInstallTrayReportsNoTrayWhereThereIsNone covers the answer the close
// decision is built on. Fyne's test driver has no tray at all, and neither has
// --no-tray; a tray that claimed otherwise would leave a hidden window with no
// way back, which is the one outcome worse than quitting (bug-0116).
func TestInstallTrayReportsNoTrayWhereThereIsNone(t *testing.T) {
	test.NewTempApp(t)
	if InstallTray("en", func() {}, func() {}, func() {}) {
		t.Error("a driver with no tray accepted one")
	}
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
