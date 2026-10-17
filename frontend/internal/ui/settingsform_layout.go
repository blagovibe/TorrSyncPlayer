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
	"fyne.io/fyne/v2/widget"
)

// The settings form is four questions a person actually has, and they are
// not neighbours of one another: how the application looks, where the film
// data lives, how to reach the room network, and — for the few who host one
// — where the torrent gateway is.
//
// That last one is what the audit found: rendezvous and relays used to sit
// directly under the gateway block with no heading of their own, so the two
// most technical fields on the form read as four more gateway fields. Same
// complaint the window itself drew, and the same answer: name the zones and
// rule them.
func (f *settingsForm) content(parent fyne.Window) fyne.CanvasObject {
	headings := func(key string) fyne.CanvasObject {
		text := canvasTextBold(f.tr(key))
		return text
	}

	rows := container.NewVBox(
		// When each choice takes effect is the one thing a person cannot
		// guess, so it sits at the top where it is read before anything is
		// changed rather than discovered after.
		widget.NewLabelWithStyle(f.tr("settings.note"), fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),

		container.NewVBox(headings("settings.zone.look"), ruleHere(),
			labelRow(f.tr("settings.language"), f.language),
			f.dark,
			labelRow(f.tr("settings.mpv"), container.NewBorder(nil, nil, nil,
				chooseFileButton(f.tr, parent, f.mpv.SetText), f.mpv)),
			zoneGap(),
		),

		container.NewVBox(headings("settings.storage"), ruleHere(),
			f.storage.group,
			f.storage.row,
			zoneGap(),
		),

		container.NewVBox(headings("settings.zone.rendezvous"), ruleHere(),
			// Rendezvous is where rooms meet, not a gateway setting. Its own
			// heading is what stops it reading as one.
			labelRow(f.tr("settings.bootstrap"), f.bootstrap),
			labelRow(f.tr("settings.relays"), f.relays),
			zoneGap(),
		),

		container.NewVBox(headings("settings.gateway"), ruleHere(),
			labelRow(f.tr("settings.gatewayURL"), f.gatewayURL),
			labelRow(f.tr("settings.gatewayUser"), f.gatewayUser),
			labelRow(f.tr("settings.gatewayPass"), f.gatewayPass),
			labelRow(f.tr("settings.gatewayCACert"), container.NewBorder(nil, nil, nil,
				chooseFileButton(f.tr, parent, f.gatewayCACert.SetText), f.gatewayCACert)),
		),
	)
	// No scroll container around the form: a VScroll hands its children an
	// unbounded width, and a Border row inside it lets the control take the
	// whole width and height — which is what the audit screenshot showed:
	// the fields collapsed, and only their placeholders survived, floating
	// across an empty dialog (it was the same in v0.3.0). The form fits the
	// dialog it is opened at; if it ever stops fitting, the fix is a taller
	// dialog, not a scroll around a broken layout.
	return rows
}

// canvasTextBold is the heading face shared by the window zones and the
// settings zones, so "now playing" and "how the app looks" look like parts
// of the same application rather than two of them.
func canvasTextBold(text string) fyne.CanvasObject {
	t := canvas.NewText(text, theme.Color(theme.ColorNameForeground))
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.TextSize = theme.TextSize() + 2
	return t
}

// labelRow puts a label beside a control, the shape the whole form uses.
func labelRow(label string, control fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widget.NewLabel(label), nil, control)
}

// zoneGap is the breathing room under a zone. Without it the headings sit
// directly on the fields of the next zone and the form reads as one run of
// rows again — the flatness the audit was called for.
func zoneGap() fyne.CanvasObject {
	// Transparent, not themed: the dialog paints over the window's canvas
	// and its background is a different colour, so a themed rectangle shows
	// up as a grey band between the zones — which is exactly the band the
	// gap was meant to remove.
	gap := canvas.NewRectangle(color.Transparent)
	gap.SetMinSize(fyne.NewSize(0, theme.Padding()*2))
	return gap
}

// ruleHere is the hairline under a settings heading. A line, not a filled
// rectangle: a rectangle is stretched to the row it sits in and paints over
// whatever is below it, which the audit screenshot showed as a grey band
// cutting through every input. A Line keeps its stroke however far the row
// grows.
func ruleHere() fyne.CanvasObject {
	line := canvas.NewLine(theme.Color(theme.ColorNameSeparator))
	line.StrokeWidth = 1
	return line
}
