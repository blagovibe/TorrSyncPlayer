// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The hero is the feature presentation on this player's screen: full-bleed
// key art, the film's name in display type, and beneath it a floating
// transport capsule — the way a streaming service introduces the one film
// it is showing you tonight. The art is drawn, never fetched: same seeded
// palette as the posters, a ghost of the film-strip emblem on the right,
// and a vignette so white type always finds a dark shoulder.
//
// The hero is pinned to the dark mood even in the light theme — key art
// is a dark room with the lights dimmed, whatever the hour.

const (
	heroW      = 1600
	heroH      = 560
	heroRadius = 34
)

var heroCache = struct {
	sync.Mutex
	m map[string]*image.NRGBA
}{m: map[string]*image.NRGBA{}}

// heroArtFor renders (or fetches) the art for a title.
func heroArtFor(name string) *image.NRGBA {
	key := name

	heroCache.Lock()
	defer heroCache.Unlock()
	if img, ok := heroCache.m[key]; ok {
		return img
	}
	if len(heroCache.m) > 32 {
		heroCache.m = map[string]*image.NRGBA{}
	}

	img := image.NewNRGBA(image.Rect(0, 0, heroW, heroH))
	top, bottom := posterFor(name)
	// The hero breathes a touch lighter than a poster: it's the lit screen.
	top = lighten(top, 0.16)
	bottom = lighten(bottom, 0.10)

	for y := 0; y < heroH; y++ {
		f := float64(y) / float64(heroH)
		f = f * f * (3 - 2*f) // smoothstep
		row := lerpNRGBA(top, bottom, f)
		for x := 0; x < heroW; x++ {
			img.SetNRGBA(x, y, row)
		}
	}

	// Type pop: the art is dimmed as a whole; the vignette deepens the floor.
	multiply(img, 0.52)
	drawGhostStrip(img)
	applyHeroVignette(img)

	mask := roundedMask(heroW, heroH, heroRadius)
	out := image.NewNRGBA(img.Bounds())
	draw.DrawMask(out, img.Bounds(), img, image.Point{}, mask, image.Point{}, draw.Over)

	heroCache.m[key] = out
	return out
}

func lighten(c color.NRGBA, f float64) color.NRGBA {
	mix := func(v uint8) uint8 {
		return uint8(float64(v) + (255-float64(v))*f)
	}
	return color.NRGBA{R: mix(c.R), G: mix(c.G), B: mix(c.B), A: 0xff}
}

func multiply(img *image.NRGBA, f float64) {
	scale := uint32(f * 256)
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = uint8((uint32(img.Pix[i+0]) * scale) >> 8)
		img.Pix[i+1] = uint8((uint32(img.Pix[i+1]) * scale) >> 8)
		img.Pix[i+2] = uint8((uint32(img.Pix[i+2]) * scale) >> 8)
	}
}

// drawGhostStrip strikes a giant film strip at the right third, barely
// there — the same watermark as the posters, an order of magnitude up.
func drawGhostStrip(img *image.NRGBA) {
	ink := color.NRGBA{R: 0xf4, G: 0xf2, B: 0xec, A: 0x2e} // ~18%
	w, h := 340, 236
	x, y := heroW-w-120, heroH/2-h/2
	strokeRect(img, x, y, w, h, 14, ink, true)
	for _, sx := range []int{36, 128, 220} {
		fillRect(img, x+sx, y+38, x+sx+74, y+78, ink)
		fillRect(img, x+sx, y+145, x+sx+74, y+185, ink)
	}
}

func applyHeroVignette(img *image.NRGBA) {
	// Top fade for the caption, deep bottom for the title and the pill.
	for y := 0; y < heroH; y++ {
		topFade := 1.0 - float64(y)/220.0
		if topFade > 0 {
			alpha := uint8(90 * topFade * topFade)
			for x := 0; x < heroW; x++ {
				c := img.NRGBAAt(x, y)
				c.R = uint8(uint16(c.R) * (0xff - uint16(alpha)) / 0xff)
				c.G = uint8(uint16(c.G) * (0xff - uint16(alpha)) / 0xff)
				c.B = uint8(uint16(c.B) * (0xff - uint16(alpha)) / 0xff)
				img.SetNRGBA(x, y, c)
			}
			continue
		}
		botFade := float64(y-heroH/2) / float64(heroH/2)
		if botFade > 0 {
			alpha := uint8(170 * botFade * botFade)
			for x := 0; x < heroW; x++ {
				c := img.NRGBAAt(x, y)
				c.R = uint8(uint16(c.R) * (0xff - uint16(alpha)) / 0xff)
				c.G = uint8(uint16(c.G) * (0xff - uint16(alpha)) / 0xff)
				c.B = uint8(uint16(c.B) * (0xff - uint16(alpha)) / 0xff)
				img.SetNRGBA(x, y, c)
			}
		}
	}
}

// heroCaptionFixed is the small line above the title, always over the art,
// always in warm screen-white, whatever the window mood.
var (
	heroCaptionColor = color.NRGBA{R: 0xc9, G: 0xc6, B: 0xbd, A: 0xd8}
	heroDimColor     = color.NRGBA{R: 0xc9, G: 0xc6, B: 0xbd, A: 0xff}
)

// heroChip is a small data capsule floating on the art.
func heroChip(text string) fyne.CanvasObject {
	surface := canvas.NewRectangle(color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x73})
	surface.CornerRadius = 10
	surface.StrokeColor = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x17}
	surface.StrokeWidth = 1
	label := canvas.NewText(text, heroDimColor)
	label.TextSize = 11
	return container.NewStack(surface, container.NewPadded(label))
}

// heroArtSeed is what the hero wears when nothing plays: the house itself,
// a calm warm grey with the house emblem's hue, indistinguishable from a
// title card.
const heroArtSeed = "TorrSyncPlayer"

func (w *window) buildHero() fyne.CanvasObject {
	w.heroArt = canvas.NewImageFromResource(staticHeroResource(heroArtFor(heroArtSeed)))
	w.heroArt.FillMode = canvas.ImageFillCover
	w.heroArt.ScaleMode = canvas.ImageScaleSmooth
	w.heroArt.SetMinSize(fyne.NewSize(640, 132))
	art := w.heroArt

	w.heroCaption = canvas.NewText(w.tr("zone.playback"), heroCaptionColor)
	w.heroCaption.TextSize = theme.CaptionTextSize()

	w.heroLbl = widget.NewLabelWithStyle(w.tr("playback.untitled"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	w.heroLbl.SizeName = theme.SizeNameHeadingText
	w.heroLbl.Wrapping = fyne.TextWrapWord

	w.heroSubLbl = canvas.NewText(w.tr("playback.idle"), heroDimColor)
	w.heroSubLbl.TextSize = theme.TextSize() - 1

	w.heroAdd = widget.NewButtonWithIcon(w.tr("action.add"), torrAddIcon, w.addTorrent)
	w.heroAdd.Importance = widget.HighImportance

	w.heroChips = container.NewHBox()
	w.heroChips.Hide()

	// The transport capsule: the film's remote, floating on the art.
	w.playBtn = widget.NewButtonWithIcon("", torrPlayIcon, w.onPlayPressed)
	w.playBtn.Importance = widget.HighImportance
	w.elapsedLbl = canvas.NewText(formatClock(0), heroDimColor)
	w.elapsedLbl.TextSize = theme.TextSize() - 1
	w.seekSlider = widget.NewSlider(0, 1)
	w.seekSlider = widget.NewSlider(0, 1)
	w.seekSlider.Step = 1
	w.seekSlider.OnChanged = w.onSeekMoved
	seekHolder := container.NewGridWrap(fyne.NewSize(240, 32), w.seekSlider)
	w.durationLbl = canvas.NewText(formatClock(0), heroDimColor)
	w.durationLbl.TextSize = theme.TextSize() - 1
	w.fsBtn = widget.NewButtonWithIcon("", torrFullscreenIcon, w.onFullscreenPressed)

	w.volSlider = widget.NewSlider(0, 100)
	w.volSlider.Step = 1
	w.volSlider.OnChanged = w.onVolumeMoved
	volHolder := container.NewGridWrap(fyne.NewSize(96, 28), w.volSlider)

	transport := container.NewBorder(nil, nil,
		container.NewHBox(w.playBtn, w.elapsedLbl),
		container.NewHBox(w.durationLbl, volHolder, w.fsBtn),
		seekHolder)
	pill := panel(transport, 22, theme.Padding())

	top := container.NewVBox(
		gap(2),
		w.heroCaption,
		gap(2),
		w.heroLbl,
		w.heroSubLbl,
		w.heroChips,
		w.preparing,
		container.NewHBox(w.heroAdd),
	)

	w.setControlsEnabled(false)
	w.setHeroIdle(true)

	// The art is a dark room in every mood; the whole subtree inherits it.
	hero := container.NewStack(
		art,
		container.NewPadded(container.NewBorder(top, container.NewCenter(pill), nil, nil)),
	)
	return container.NewThemeOverride(hero, torrFixedTheme{base: newTorrTheme(), variant: theme.VariantDark})
}

func staticHeroResource(img *image.NRGBA) *fyne.StaticResource {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return &fyne.StaticResource{StaticName: "hero-art.png", StaticContent: buf.Bytes()}
}

// rebuildHeroChips refreshes the data capsules floating on the art: how
// big, how long, how many peers. Rebuilt only when the numbers change —
// a two-second rebuild of static widgets reads as flicker.
func (w *window) rebuildHeroChips() {
	w.heroChips.RemoveAll()
	w.mu.RLock()
	name, peers := "", 0
	var size int64
	if w.current != nil {
		name, size = w.current.Name, w.current.Size
		for _, t := range w.torrents {
			if t.ID == w.current.ID {
				peers = t.Peers
			}
		}
	}
	duration := w.ctlDuration
	w.mu.RUnlock()

	if name == "" {
		w.heroChips.Hide()
		return
	}
	if size > 0 {
		w.heroChips.Add(heroChip(humanSize(size)))
	}
	if duration > 0 {
		w.heroChips.Add(heroChip(formatClock(duration)))
	}
	if peers > 0 {
		w.heroChips.Add(heroChip(w.trfi("hero.peers", peers)))
	}
	w.heroChips.Show()
}

// setHeroArt swaps the key art when the film changes.
func (w *window) setHeroArt(name string) {
	w.heroArt.Resource = staticHeroResource(heroArtFor(name))
	w.heroArt.Refresh()
}
