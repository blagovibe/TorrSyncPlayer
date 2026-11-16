// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"image/color"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

// The house is a darkened hall with a lit screen: a warm black up top
// cooling into pure black at the floor, a faint marquee glow bleeding down
// from the top edge, and frosted panels floating on it with a soft shadow.
// This file draws the room and gives the panels their physics.
//
// Light mood is the same geometry under daylight: paper, white frosted
// glass, the same shadow — softer, shallower.

// The house remembers its mood: theme colours pin one mood regardless of the
// desktop, and so does the wallpaper. InstallTorrTheme and applyTheme set
// this; everything else reads it.
var torrMoodDark atomic.Bool

func init() {
	torrMoodDark.Store(true)
}

func moodDark() bool {
	return torrMoodDark.Load()
}

// panelSurface is the colour of a frosted panel on the current mood.
func panelSurface() color.NRGBA {
	if moodDark() {
		return torrPanel
	}
	return torrPanelLight
}

// hairlineColor is the 1px edge every glass element carries — a whisper of
// light on dark, a whisper of ink on light. No opaque borders anywhere.
func hairlineColor() color.Color {
	if moodDark() {
		return torrHairline
	}
	return torrHairlineLight
}

// wallpaper paints the hall: a vertical gradient from the warm dark crown
// of the room to its black floor, with a faint radial glow mid-top where
// the projector would hang. The light mood mirrors it.
func wallpaper() fyne.CanvasObject {
	top, bottom, glow := torrHallTop, torrHallBottom, torrGlow
	if !moodDark() {
		top, bottom, glow = torrHallTopLight, torrHallBottomLight, torrGlowLight
	}
	gradient := canvas.NewVerticalGradient(top, bottom)
	bloom := canvas.NewRadialGradient(glow, color.Transparent)
	return container.NewStack(gradient, bloom)
}

// panel floats content on a frosted card: rounded surface, a whisper of
// hairline edge, a soft shadow beneath. Radius is generous on purpose —
// the Vision Pro reference rounds everything past ninety degrees of trust.
func panel(content fyne.CanvasObject, cornerRadius float32, pad float32) fyne.CanvasObject {
	surface := canvas.NewRectangle(panelSurface())
	surface.CornerRadius = cornerRadius
	surface.StrokeColor = hairlineColor()
	surface.StrokeWidth = 1
	surface.Shadow = canvas.Shadow{
		Color:      color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x60},
		BlurRadius: 22,
		Offset:     fyne.NewPos(0, 6),
		Variant:    canvas.BoxShadow,
	}
	if pad <= 0 {
		return container.NewStack(surface, content)
	}
	return container.NewStack(surface, container.NewPadded(content))
}

// zoneCaption is the quiet name of a shelf or a block: sentence case, dim,
// smaller than body text — a caption, not signage.
func zoneCaption(text string) fyne.CanvasObject {
	label := canvas.NewText(text, theme.Color(theme.ColorNamePlaceHolder))
	label.TextSize = theme.CaptionTextSize()
	return label
}

// filmVignette darkens the bottom of the key art so white type sits on it
// without a scrim box. A gradient, not a rectangle with a border — the
// poster fades, it never stops.
func filmVignette() fyne.CanvasObject {
	fade := canvas.NewVerticalGradient(
		color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00},
		color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xa8},
	)
	return fade
}
