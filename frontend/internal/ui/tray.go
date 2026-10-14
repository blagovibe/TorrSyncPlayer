// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/systray"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
)

// Tray is the system-tray presence.
//
// It is a convenience, not behaviour: an application whose window is closed and
// which has no tray icon is simply gone. The tray is also what keeps the
// download going while the window is shut — the backend is a child process, so
// closing the window must not end it.
//
// Every platform call is best effort. A desktop that forbids tray icons, and a
// headless session, both leave the application running with its window, which is
// what they did before the tray existed.
type Tray struct {
	language string

	// running is set once systray has actually presented an icon. systray.Run
	// reports nothing about failure, so the flag is how the caller learns that
	// there is no tray to talk to.
	running bool
}

// NewTray attaches a tray icon.
//
// It returns a Tray whose Running reports false when the platform has no usable
// tray. That is not an error worth refusing to start over.
func NewTray(language string, onOpen, onQuit func()) *Tray {
	t := &Tray{language: language}

	systray.RunWithExternalLoop(func() {
		t.running = true

		systray.SetTooltip(t.tr("app.title"))
		if icon := trayIcon(); icon != nil {
			systray.SetIcon(icon)
		}

		open := systray.AddMenuItem(t.tr("action.open"), "")
		quit := systray.AddMenuItem(t.tr("action.quit"), "")
		systray.AddSeparator()

		go func() {
			for {
				select {
				case <-open.ClickedCh:
					if onOpen != nil {
						onOpen()
					}
				case <-quit.ClickedCh:
					if onQuit != nil {
						onQuit()
					}
					systray.Quit()
					return
				}
			}
		}()
	}, func() {})

	return t
}

// Running reports whether an icon is actually on screen.
func (t *Tray) Running() bool { return t != nil && t.running }

// Shutdown removes the icon.
func (t *Tray) Shutdown() {
	if t == nil || !t.running {
		return
	}
	systray.Quit()
}

func (t *Tray) tr(key string) string {
	return catalog.Lookup(t.language, key)
}

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
func CloseToTray(window fyne.Window, tray *Tray) {
	window.SetCloseIntercept(func() {
		if hideOnClose(tray.Running()) {
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
//
// Encoded once: the icon never changes, and re-encoding it per call is waste.
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
