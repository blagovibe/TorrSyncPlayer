// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"sync"

	"fyne.io/fyne/v2"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
)

// The tray is the application's way back into itself when the window is closed.
//
// Closing the window hides it rather than ending the application — the backend
// is a child process, so a download should not stop because someone tidied their
// taskbar — which makes the tray the only way to bring the window back. That is
// also why settings belong in the tray menu and not only in the window: the
// window may be the thing that is closed.
//
// The toolkit owns the tray, not this package. It was ours, and the tray never
// appeared: systray.RunWithExternalLoop hands back a start hook, to be called
// once the toolkit's event loop is up, and the hook was dropped on the floor —
// proven by calling it and watching the difference (bug-0116). Fyne starts and
// stops the tray itself, at the right moment on each platform, so the menu is
// handed over and the answer to "is there a tray" comes from the driver.

// trayDriver is the part of Fyne's desktop driver that owns the tray.
//
// Fyne keeps this interface unexported, so a type assertion is the only way in.
// It is also the whole reason this is safe: the driver refuses to start a tray
// before its own loop is running, which is the mistake bug-0116 was.
type trayDriver interface {
	SetSystemTrayMenu(*fyne.Menu)
}

// InstallTray hands the system-tray menu to the toolkit.
//
// It reports whether the driver took it. False means there is no tray to talk
// to — --no-tray, the test driver, or a driver without tray support — and the
// caller must not pretend otherwise: closing the window with no tray ends the
// application, because there would be no way back.
func InstallTray(language string, onOpen, onSettings, onQuit func()) bool {
	app := fyne.CurrentApp()
	if app == nil {
		return false
	}
	d, ok := app.Driver().(trayDriver)
	if !ok {
		// Said out loud because the alternative is invisible: the window would
		// quit on close as it always did, and the tray the owner was promised
		// by the flag's very existence would simply never appear (bug-0116).
		fyne.LogError("no tray on this driver: closing the window will end the application", nil)
		return false
	}
	d.SetSystemTrayMenu(trayMenu(language, onOpen, onSettings, onQuit))
	return true
}

// trayMenu is the menu itself, without a driver to put it on, so that what it
// contains can be looked at without a desktop (tray_test.go).
func trayMenu(language string, onOpen, onSettings, onQuit func()) *fyne.Menu {
	tr := func(key string) string { return catalog.Lookup(language, key) }
	quit := fyne.NewMenuItem(tr("action.quit"), onQuit)
	// IsQuit is what tells the toolkit this is the way out. Without it Fyne
	// looks at the last item's label, compares it with its own translation of
	// "Quit" and, not finding a match in Russian, appends a second exit of its
	// own — read off a running tray over DBus, which is where «Выход» and
	// «Выйти» sat one under the other.
	quit.IsQuit = true
	return fyne.NewMenu("",
		fyne.NewMenuItem(tr("action.open"), onOpen),
		fyne.NewMenuItem(tr("action.settings"), onSettings),
		fyne.NewMenuItemSeparator(),
		quit,
	)
}

// TrayIcon is the application's mark as PNG bytes, for the toolkit to show in
// the tray and on the window.
//
// Encoded once: it never changes, and re-encoding it per call is waste.
func TrayIcon() []byte { return trayIcon() }

// CloseToTray hides the window instead of ending the application.
//
// This is a close *intercept* rather than an after-close hook: Fyne's
// SetOnClosed runs once the window is already gone, so hiding it there is
// undoing a completed close, and calling Close again from inside it re-enters
// the very code that is running. SetCloseIntercept exists for exactly this
// case — the callback runs instead of the close, so the decision to stay alive
// is made before anything is destroyed.
//
// The window is hidden rather than destroyed so it comes back instantly, and
// the application keeps running: the backend is a child process and downloads
// continue whether or not anyone is looking.
func CloseToTray(window fyne.Window, tray bool) {
	window.SetCloseIntercept(func() {
		if hideOnClose(tray) {
			window.Hide()
			return
		}
		window.Close()
	})
}

// hideOnClose decides whether a close request is absorbed.
//
// With a tray, yes: the backend is a child process and a download should not
// stop because someone tidied their taskbar. Without one, no — the window is
// the only way back to the application, so honouring the close is the only
// honest reading of it.
//
// Separate from CloseToTray because the behaviour it decides cannot be tested
// through the window at all: Fyne's test driver closes unconditionally and
// never consults the intercept, so a test that called Close would pass whatever
// this answered.
func hideOnClose(trayRunning bool) bool {
	return trayRunning
}

// trayIcon is the tray's picture, drawn rather than shipped.
//
// A tray with no icon is refused outright by some desktops, which would turn a
// cosmetic detail into a failed start. Drawing it in code keeps the binary free
// of an asset that has to be regenerated for every density a platform asks for,
// and there is only one icon to draw: the application's own mark — a disc with
// a play triangle on it. See icon.go, which draws the same mark at any size
// because packaging needs it as a file too.
var trayIcon = sync.OnceValue(func() []byte {
	icon, err := IconPNG(trayIconSize)
	if err != nil {
		return nil
	}
	return icon
})

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// insideTriangle reports whether a point lies inside the triangle (ax, ay),
// (bx, by), (cx, cy).
//
// By sign agreement of the three cross products: an interior point is on the
// same side of all three edges. Simpler than solving three half-planes and
// exact enough for a 32-pixel icon.
func insideTriangle(px, py, ax, ay, bx, by, cx, cy float64) bool {
	d1 := cross(px, py, ax, ay, bx, by)
	d2 := cross(px, py, bx, by, cx, cy)
	d3 := cross(px, py, cx, cy, ax, ay)

	hasNegative := d1 < 0 || d2 < 0 || d3 < 0
	hasPositive := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNegative && hasPositive)
}

// cross is the z component of the cross product of (b−a) and (p−a).
func cross(px, py, ax, ay, bx, by float64) float64 {
	return (bx-ax)*(py-ay) - (by-ay)*(px-ax)
}
