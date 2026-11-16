// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The top bar is the house's lintel: the wordmark on the left, the one
// field the app exists for glowing softly in the middle, and the doorway
// actions — add a film — on the right. One floating frosted capsule, the
// same physics as everything else in the hall.
func (w *window) buildTopBar() fyne.CanvasObject {
	markBytes, err := IconPNG(96)
	var brand fyne.CanvasObject
	if err == nil {
		res := fyne.NewStaticResource("wordmark.png", markBytes)
		mark := canvas.NewImageFromResource(res)
		mark.ScaleMode = canvas.ImageScaleSmooth
		mark.SetMinSize(fyne.NewSize(26, 26))
		markHBox := container.NewHBox(container.NewCenter(mark))
		name := canvas.NewText("TorrSyncPlayer", theme.Color(theme.ColorNameForeground))
		name.TextStyle = fyne.TextStyle{Bold: true}
		name.TextSize = theme.TextSize() + 2
		brand = container.NewHBox(markHBox, name)
	} else {
		name := canvas.NewText("TorrSyncPlayer", theme.Color(theme.ColorNameForeground))
		name.TextStyle = fyne.TextStyle{Bold: true}
		name.TextSize = theme.TextSize() + 2
		brand = name
	}

	// The magnet field in its own capsule: the magnet glyph left, the field
	// itself inside, the whole pill one object.
	magnetIcon := container.NewPadded(widget.NewIcon(torrMagnetIcon))
	search := container.NewBorder(nil, nil, magnetIcon, nil, w.entry)
	searchPill := panel(search, 20, 2)

	w.addButton.Importance = widget.HighImportance
	w.addFile.Importance = widget.LowImportance

	refresh := widget.NewButtonWithIcon("", torrRefreshIcon, w.refresh)
	refresh.Importance = widget.LowImportance
	shortcuts := widget.NewButtonWithIcon("", torrInfoIcon, w.showShortcuts)
	shortcuts.Importance = widget.LowImportance

	actions := container.NewHBox(
		w.addButton,
		w.addFile,
		refresh,
		shortcuts,
		widget.NewButtonWithIcon("", torrSettingsIcon, func() { w.ShowSettings(w.parentWindow()) }),
	)

	return panel(
		container.NewBorder(nil, nil, brand, actions, container.NewPadded(searchPill)),
		16, 5,
	)
}
