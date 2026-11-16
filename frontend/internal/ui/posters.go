// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"bytes"
	"image/png"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The shelf: a horizontal rail of drawn posters — the browsing surface of
// everything this player holds. Tap a poster and that film plays; the one
// bound to the player carries the amber ring drawn into its own art.

// posterTile is one shelf unit: the art, plus a quiet dots menu in the
// corner for the item's own actions. The art itself is a cache-keyed image,
// so rebuilding the rail twice a second costs nothing on the GPU.
type posterTile struct {
	widget.BaseWidget
	content fyne.CanvasObject
	app     *window
	t       *Torrent
}

func newPosterTile(w *window, t *Torrent, playing bool) fyne.CanvasObject {
	img, err := posterArtFor(w.tr, *t, playing)
	if err != nil {
		// A poster that cannot draw is a grey tile with the name, not a crash.
		fallback := canvas.NewRectangle(torrPanelHi)
		fallback.CornerRadius = 12
		return container.NewStack(
			fallback,
			container.New(vpad(8), widget.NewLabel(t.Name)),
		)
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, img); err != nil {
		return widget.NewLabel(t.Name)
	}
	res := fyne.NewStaticResource("poster-"+t.ID+".png", pngBytes.Bytes())
	poster := canvas.NewImageFromResource(res)
	poster.FillMode = canvas.ImageFillContain
	poster.ScaleMode = canvas.ImageScaleSmooth
	poster.SetMinSize(fyne.NewSize(146, 150))

	// The item's own actions, two quiet glyphs on the face of the poster —
	// one tap, no hidden menu.
	info := widget.NewButtonWithIcon("", torrInfoIcon, func() { w.showDescription(t) })
	info.Importance = widget.LowImportance
	del := widget.NewButtonWithIcon("", torrDeleteIcon, func() { w.removeTorrent(t) })
	del.Importance = widget.LowImportance

	top := container.NewBorder(nil, nil, nil, container.NewHBox(info, del))

	tile := &posterTile{app: w, t: t}
	tile.content = container.NewStack(
		poster,
		container.NewPadded(top),
	)
	tile.ExtendBaseWidget(tile)
	return tile
}

func (p *posterTile) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.content)
}

func (p *posterTile) Tapped(_ *fyne.PointEvent) {
	p.app.play(p.t)
}

// shelf rebuilds the rail's tiles for the current library, playing marked
// on the one bound to the player. Callers hold no lock: the snapshot is
// copied under it here.
func (w *window) rebuildShelf() {
	w.mu.RLock()
	torrents := make([]Torrent, len(w.torrents))
	copy(torrents, w.torrents)
	playing := ""
	if w.current != nil && w.ctlOn {
		playing = w.current.ID
	}
	w.mu.RUnlock()

	onMain(func() {
		if w.railBox == nil {
			return
		}
		w.railBox.RemoveAll()
		if len(torrents) == 0 {
			empty := canvas.NewText(w.tr("library.empty"), theme.Color(theme.ColorNamePlaceHolder))
			empty.TextSize = theme.CaptionTextSize()
			w.railBox.Add(container.NewCenter(empty))
			w.railBox.Refresh()
			return
		}
		for i := range torrents {
			t := torrents[i]
			w.railBox.Add(newPosterTile(w, &t, t.ID == playing))
		}
		w.railBox.Refresh()
	})
}

// vpad pads only vertically — poster art has its own side margins.
type vpadT struct{ h float32 }

func vpad(h float32) fyne.Layout { return vpadT{h: h} }

func (v vpadT) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var s fyne.Size
	for _, o := range objects {
		m := o.MinSize()
		if m.Width > s.Width {
			s.Width = m.Width
		}
		if m.Height > s.Height {
			s.Height = m.Height
		}
	}
	s.Height += 2 * v.h
	return s
}

func (v vpadT) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Move(fyne.NewPos(0, v.h))
		o.Resize(fyne.NewSize(size.Width, size.Height-2*v.h))
	}
}
