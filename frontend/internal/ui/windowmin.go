// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

// Fyne has no minimum window size: `SetFixedSize` pins a size and refuses
// to change it, which is worse than a minimum — the window could not be
// maximised or restored. The audit's finding was that nothing stopped the
// window from being dragged small enough that the library collapsed to a
// strip and the zones above it stopped fitting.
//
// So the floor is enforced from the poll the window already runs: when the
// window is smaller than the floor, it is put back. Two seconds is the
// worst delay between the drag and the correction, and the correction only
// happens on a size the person cannot use anyway.
const (
	minWindowWidth  = 800
	minWindowHeight = 560
)

// ensureMinSize puts the window back above the floor if it was dragged
// below it. No-op without a parent window, which is what tests build.
func (w *window) ensureMinSize() {
	if w.parent == nil {
		return
	}
	size := w.parent.Canvas().Size()
	if size.Width >= minWindowWidth && size.Height >= minWindowHeight {
		return
	}
	if size.Width < minWindowWidth {
		size.Width = minWindowWidth
	}
	if size.Height < minWindowHeight {
		size.Height = minWindowHeight
	}
	onMain(func() { w.parent.Resize(size) })
}

// windowIsTooSmall is the decision, separated from the window itself so it
// can be tested without one.
func windowIsTooSmall(w, h float32) bool {
	return w < minWindowWidth || h < minWindowHeight
}
