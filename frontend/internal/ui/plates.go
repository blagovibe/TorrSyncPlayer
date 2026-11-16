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

// The library shelves: one rounded plate per row, content on it, air around
// it. The update pass has to find its own widgets inside the template, so
// the template's shape is fixed and documented here, and every step of the
// descent fails closed instead of panicking in the middle of a poll.
//
// template = Padded[ Stack[ surface(Rectangle), Border[ body(VBox), actions(HBox) ] ] ]
// padded.Objects[0]   = stack
// stack.Objects[1]    = border (0 is the plate)
// border.Objects[0]   = body (VBox): [name(0), sub(1), bar(2)]
// border.Objects[1]   = actions (HBox): [info(0), del(1)]

func newTorrentPlate() *fyne.Container {
	name := widget.NewLabel("")
	sub := canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
	sub.TextSize = theme.TextSize() - 3
	bar := widget.NewProgressBar()
	info := widget.NewButtonWithIcon("", torrInfoIcon, nil)
	info.Importance = widget.LowImportance
	del := widget.NewButtonWithIcon("", torrDeleteIcon, nil)
	del.Importance = widget.LowImportance

	body := container.NewVBox(name, sub, bar)
	actions := container.NewHBox(info, del)
	row := container.NewBorder(nil, nil, nil, actions, body)
	return plateRow(row)
}

// filePlate is the same shelf idea for a single line plus its index and
// size: quieter than its neighbour, same air around.
//
// template = Padded[ Stack[ surface(Rectangle), VBox[ name(0), sub(1) ] ] ]
func newFilePlate() *fyne.Container {
	name := widget.NewLabel("")
	name.Wrapping = fyne.TextTruncate
	sub := canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
	sub.TextSize = theme.TextSize() - 4
	return plateRow(container.NewVBox(name, sub))
}

// plateRow wraps a row in its rounded plate with air on every side.
func plateRow(content fyne.CanvasObject) *fyne.Container {
	surface := canvas.NewRectangle(theme.Color(theme.ColorNameButton))
	surface.CornerRadius = 8
	return container.NewPadded(container.NewStack(surface, content))
}

// torrentPlateParts reaches through the torrent template and hands back the
// widgets it was built from. ok=false means the template shape changed and
// this pass cannot safely touch it — the row is skipped, the poll keeps.
func torrentPlateParts(obj fyne.CanvasObject) (name *widget.Label, sub *canvas.Text, bar *widget.ProgressBar, info, del *widget.Button, ok bool) {
	padded, ok := obj.(*fyne.Container)
	if !ok || len(padded.Objects) != 1 {
		return nil, nil, nil, nil, nil, false
	}
	stack, ok := padded.Objects[0].(*fyne.Container)
	if !ok || len(stack.Objects) != 2 {
		return nil, nil, nil, nil, nil, false
	}
	border, ok := stack.Objects[1].(*fyne.Container)
	if !ok || len(border.Objects) != 2 {
		return nil, nil, nil, nil, nil, false
	}
	body, ok := border.Objects[0].(*fyne.Container)
	if !ok || len(body.Objects) != 3 {
		return nil, nil, nil, nil, nil, false
	}
	actions, ok := border.Objects[1].(*fyne.Container)
	if !ok || len(actions.Objects) != 2 {
		return nil, nil, nil, nil, nil, false
	}
	name, ok1 := body.Objects[0].(*widget.Label)
	sub, ok2 := body.Objects[1].(*canvas.Text)
	bar, ok3 := body.Objects[2].(*widget.ProgressBar)
	info, ok4 := actions.Objects[0].(*widget.Button)
	del, ok5 := actions.Objects[1].(*widget.Button)
	return name, sub, bar, info, del, ok1 && ok2 && ok3 && ok4 && ok5
}

// filePlateParts reaches through the file template. Same contract as
// torrentPlateParts.
func filePlateParts(obj fyne.CanvasObject) (name *widget.Label, sub *canvas.Text, ok bool) {
	padded, ok := obj.(*fyne.Container)
	if !ok || len(padded.Objects) != 1 {
		return nil, nil, false
	}
	stack, ok := padded.Objects[0].(*fyne.Container)
	if !ok || len(stack.Objects) != 2 {
		return nil, nil, false
	}
	body, ok := stack.Objects[1].(*fyne.Container)
	if !ok || len(body.Objects) != 2 {
		return nil, nil, false
	}
	name, ok1 := body.Objects[0].(*widget.Label)
	sub, ok2 := body.Objects[1].(*canvas.Text)
	return name, sub, ok1 && ok2
}
