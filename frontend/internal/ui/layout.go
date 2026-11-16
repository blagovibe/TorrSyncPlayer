// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// The window is a screen of a player, not a settings page: what plays is
// the largest thing on it, what it needs is next to it, and the library
// takes the rest. These are the zones, in that order.
//
// The complaint that produced this file was honest and specific: the window
// looked flat and nothing in it had a rank. Seven strips of equal weight in
// one column is a debug panel, not an application. Zone headings and rules
// give each part a name and a border, which is what tells the eye where to
// look first — the alternative, a native-looking toolkit, was refused on
// purpose (idea-0063: GTK4 would cost macOS and Windows).
// zoneHeading is the quiet name of a zone: sentence case, support grey, one
// step smaller than body text — a caption, not a shout. A streaming service
// does not letterpress "AUDIO AND SUBTITLES" onto its shelves.
func (w *window) zoneHeading(key string) fyne.CanvasObject {
	text := canvas.NewText(w.tr(key), theme.Color(theme.ColorNamePlaceHolder))
	text.TextSize = theme.TextSize() - 1
	return text
}

// rule is the line under a heading and between zones. It is a hairline of
// the separator colour, not a separator widget: those render as a fat grey
// band that reads as a patch over the window rather than as a border.
func rule() fyne.CanvasObject {
	// A line, not a filled rectangle: a rectangle is stretched to the row it
	// sits in and paints over whatever is below it — which is what the audit
	// screenshot showed, a grey band cutting through every input. A Line
	// keeps its stroke no matter how far the row grows.
	line := canvas.NewLine(theme.Color(theme.ColorNameSeparator))
	line.StrokeWidth = 1
	return line
}

// gap is the air between shelves: nothing drawn, just room. The height is a
// parameter because the library breathes deeper than the zones do.
func gap(height float32) fyne.CanvasObject {
	space := canvas.NewRectangle(color.Transparent)
	space.SetMinSize(fyne.NewSize(0, height))
	return space
}

// section is one named zone: a heading, a hairline under it, and its
// content.
func section(heading fyne.CanvasObject, content ...fyne.CanvasObject) fyne.CanvasObject {
	items := make([]fyne.CanvasObject, 0, len(content)+2)
	items = append(items, heading, rule())
	items = append(items, content...)
	return container.NewVBox(items...)
}

// contentStack lays zones out with a gap between them. Fyne's VBox gives no
// vertical padding of its own, so without this every zone touches the next
// and the rules stop reading as borders.
func contentStack(zones ...fyne.CanvasObject) fyne.CanvasObject {
	items := make([]fyne.CanvasObject, 0, len(zones)*2-1)
	for i, z := range zones {
		if i > 0 {
			gap := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
			gap.SetMinSize(fyne.NewSize(0, theme.Padding()))
			items = append(items, gap)
		}
		items = append(items, z)
	}
	return container.NewVBox(items...)
}
