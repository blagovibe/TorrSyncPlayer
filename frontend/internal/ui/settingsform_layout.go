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

		container.NewVBox(headings("settings.zone.look"), rule(),
			labelRow(f.tr("settings.language"), f.language),
			f.dark,
			labelRow(f.tr("settings.mpv"), container.NewBorder(nil, nil, nil,
				chooseFileButton(f.tr, parent, f.mpv.SetText), f.mpv)),
		),

		container.NewVBox(headings("settings.storage"), rule(),
			f.storage.group,
			f.storage.row,
		),

		container.NewVBox(headings("settings.zone.rendezvous"), rule(),
			// Rendezvous is where rooms meet, not a gateway setting. Its own
			// heading is what stops it reading as one.
			labelRow(f.tr("settings.bootstrap"), f.bootstrap),
			labelRow(f.tr("settings.relays"), f.relays),
		),

		container.NewVBox(headings("settings.gateway"), rule(),
			labelRow(f.tr("settings.gatewayURL"), f.gatewayURL),
			labelRow(f.tr("settings.gatewayUser"), f.gatewayUser),
			labelRow(f.tr("settings.gatewayPass"), f.gatewayPass),
			labelRow(f.tr("settings.gatewayCACert"), container.NewBorder(nil, nil, nil,
				chooseFileButton(f.tr, parent, f.gatewayCACert.SetText), f.gatewayCACert)),
		),
	)
	return container.NewVScroll(rows)
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
