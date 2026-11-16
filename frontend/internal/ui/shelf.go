// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// The shelf is the library as key art — the browsable face of everything
// this player holds. Tap a poster and that film plays; the one bound to
// the player already wears the amber ring drawn into its art.
func (w *window) buildRail() fyne.CanvasObject {
	w.railBox = container.NewHBox()
	w.railScroll = container.NewHScroll(w.railBox)
	inner := container.NewVBox(
		zoneCaption(w.tr("zone.shelf")),
		gap(8),
		w.railScroll,
	)
	return panel(inner, 18, theme.Padding())
}
