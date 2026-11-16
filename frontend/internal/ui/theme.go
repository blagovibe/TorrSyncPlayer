// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The window is a screen of a player: what plays is the largest thing on it.
// The stock Fyne theme inherits every colour from the operating system, which
// is the safest and the blandest choice — the window looked flat and every
// strip weighed the same. idea-0187 was filed for this.
//
// The palette below is the promise of that card: a warm near-black backdrop
// with cream text, muted clay figureheads, and a single terracotta accent —
// the colour of the film kind of red where there is one film kind of red.
// Nothing else gets to compete for attention; the theme system exists so the
// two variants (dark warm and light paper) do not fork into two codebases.

// Dark warm, per the palette direction the owner signed off on.
var (
	torrBackground  = color.NRGBA{R: 0x17, G: 0x14, B: 0x12, A: 0xff} // #171412
	torrSurface     = color.NRGBA{R: 0x20, G: 0x1c, B: 0x19, A: 0xff} // #201C19
	torrText        = color.NRGBA{R: 0xed, G: 0xe6, B: 0xdd, A: 0xff} // #EDE6DD
	torrMuted       = color.NRGBA{R: 0xa5, G: 0x9c, B: 0x90, A: 0xff} // #A59C90
	torrAccent      = color.NRGBA{R: 0xe0, G: 0x76, B: 0x4a, A: 0xff} // #E0764A
	torrSeparator   = color.NRGBA{R: 0x2e, G: 0x29, B: 0x26, A: 0xff} // #2E2926
	torrInputBG     = color.NRGBA{R: 0x1d, G: 0x19, B: 0x16, A: 0xff} // #1D1916
	torrSelection   = color.NRGBA{R: 0xe0, G: 0x76, B: 0x4a, A: 0x2e} // accent @ ~18%
	torrPressed     = color.NRGBA{R: 0xc8, G: 0x66, B: 0x3f, A: 0xff} // accent pressed darker
	torrError       = color.NRGBA{R: 0xe0, G: 0x76, B: 0x4a, A: 0xff} // error stays warm
	torrSuccess     = color.NRGBA{R: 0x8a, G: 0xc9, B: 0x62, A: 0xff}
	torrWarning     = color.NRGBA{R: 0xe0, G: 0xc5, B: 0x76, A: 0xff}
	torrScrollBarBG = color.NRGBA{R: 0x24, G: 0x20, B: 0x1d, A: 0xff}
	torrDisabled    = color.NRGBA{R: 0x6e, G: 0x66, B: 0x5d, A: 0xff}

	// Light paper, the mirror warm side of the same palette.
	torrBackgroundLight  = color.NRGBA{R: 0xfa, G: 0xf7, B: 0xf2, A: 0xff} // #FAF7F2
	torrSurfaceLight     = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // #FFFFFF
	torrTextLight        = color.NRGBA{R: 0x2a, G: 0x24, B: 0x1f, A: 0xff} // #2A241F
	torrMutedLight       = color.NRGBA{R: 0x6b, G: 0x62, B: 0x5a, A: 0xff} // #6B625A
	torrAccentLight      = color.NRGBA{R: 0xc9, G: 0x5f, B: 0x3b, A: 0xff} // #C95F3B
	torrSeparatorLight   = color.NRGBA{R: 0xe7, G: 0xe1, B: 0xd8, A: 0xff} // #E7E1D8
	torrInputBGLight     = color.NRGBA{R: 0xf6, G: 0xf3, B: 0xee, A: 0xff} // #F6F3EE
	torrSelectionLight   = color.NRGBA{R: 0xc9, G: 0x5f, B: 0x3b, A: 0x2e}
	torrPressedLight     = color.NRGBA{R: 0xa8, G: 0x4f, B: 0x2f, A: 0xff}
	torrErrorLight       = color.NRGBA{R: 0xc9, G: 0x5f, B: 0x3b, A: 0xff}
	torrSuccessLight     = color.NRGBA{R: 0x5e, G: 0xa3, B: 0x3f, A: 0xff}
	torrWarningLight     = color.NRGBA{R: 0xb3, G: 0x90, B: 0x33, A: 0xff}
	torrScrollBarBGLight = color.NRGBA{R: 0xef, G: 0xeb, B: 0xe4, A: 0xff}
	torrDisabledLight    = color.NRGBA{R: 0xa8, G: 0xa2, B: 0x9a, A: 0xff}
)

var torrPalette = map[fyne.ThemeColorName]map[fyne.ThemeVariant]color.Color{
	theme.ColorNameBackground:                {theme.VariantDark: torrBackground, theme.VariantLight: torrBackgroundLight},
	theme.ColorNameButton:                    {theme.VariantDark: torrSurface, theme.VariantLight: torrSurfaceLight},
	theme.ColorNameDisabledButton:            {theme.VariantDark: torrBackground, theme.VariantLight: torrBackgroundLight},
	theme.ColorNameDisabled:                  {theme.VariantDark: torrDisabled, theme.VariantLight: torrDisabledLight},
	theme.ColorNameError:                     {theme.VariantDark: torrError, theme.VariantLight: torrErrorLight},
	theme.ColorNameFocus:                     {theme.VariantDark: torrAccent, theme.VariantLight: torrAccentLight},
	theme.ColorNameForeground:                {theme.VariantDark: torrText, theme.VariantLight: torrTextLight},
	theme.ColorNameForegroundOnError:         {theme.VariantDark: torrBackground, theme.VariantLight: torrBackgroundLight},
	theme.ColorNameForegroundOnPrimary:       {theme.VariantDark: torrBackground, theme.VariantLight: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}},
	theme.ColorNameForegroundOnSuccess:       {theme.VariantDark: torrBackground, theme.VariantLight: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}},
	theme.ColorNameForegroundOnWarning:       {theme.VariantDark: torrBackground, theme.VariantLight: torrTextLight},
	theme.ColorNameHeaderBackground:          {theme.VariantDark: torrSurface, theme.VariantLight: torrSurfaceLight},
	theme.ColorNameHover:                     {theme.VariantDark: torrAccent, theme.VariantLight: torrAccentLight},
	theme.ColorNameHyperlink:                 {theme.VariantDark: torrAccent, theme.VariantLight: torrAccentLight},
	theme.ColorNameInnerWindowBorder:         {theme.VariantDark: torrSeparator, theme.VariantLight: torrSeparatorLight},
	theme.ColorNameInnerWindowBorderInactive: {theme.VariantDark: torrSeparator, theme.VariantLight: torrSeparatorLight},
	theme.ColorNameInputBackground:           {theme.VariantDark: torrInputBG, theme.VariantLight: torrInputBGLight},
	theme.ColorNameInputBorder:               {theme.VariantDark: torrSeparator, theme.VariantLight: torrSeparatorLight},
	theme.ColorNameMenuBackground:            {theme.VariantDark: torrSurface, theme.VariantLight: torrSurfaceLight},
	theme.ColorNameOverlayBackground:         {theme.VariantDark: torrBackground, theme.VariantLight: torrBackgroundLight},
	theme.ColorNamePlaceHolder:               {theme.VariantDark: torrMuted, theme.VariantLight: torrMutedLight},
	theme.ColorNamePressed:                   {theme.VariantDark: torrPressed, theme.VariantLight: torrPressedLight},
	theme.ColorNamePrimary:                   {theme.VariantDark: torrAccent, theme.VariantLight: torrAccentLight},
	theme.ColorNameScrollBar:                 {theme.VariantDark: torrPressed, theme.VariantLight: torrPressedLight},
	theme.ColorNameScrollBarBackground:       {theme.VariantDark: torrScrollBarBG, theme.VariantLight: torrScrollBarBGLight},
	theme.ColorNameSelection:                 {theme.VariantDark: torrSelection, theme.VariantLight: torrSelectionLight},
	theme.ColorNameSeparator:                 {theme.VariantDark: torrSeparator, theme.VariantLight: torrSeparatorLight},
	theme.ColorNameShadow:                    {theme.VariantDark: color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xa0}, theme.VariantLight: color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x30}},
	theme.ColorNameSuccess:                   {theme.VariantDark: torrSuccess, theme.VariantLight: torrSuccessLight},
	theme.ColorNameWarning:                   {theme.VariantDark: torrWarning, theme.VariantLight: torrWarningLight},
}

var torrSizes = map[fyne.ThemeSizeName]float32{
	theme.SizeNameButtonRadius:       9,
	theme.SizeNameCardRadius:         10,
	theme.SizeNamePopupRadius:        10,
	theme.SizeNameDialogRadius:       12,
	theme.SizeNameInputRadius:        8,
	theme.SizeNameSelectionRadius:    6,
	theme.SizeNameLineSpacing:        1.05,
	theme.SizeNamePadding:            9,
	theme.SizeNameInnerPadding:       12,
	theme.SizeNameSeparatorThickness: 1,
	theme.SizeNameSplitThickness:     3,
}

type torrTheme struct {
	base fyne.Theme
}

func newTorrTheme() torrTheme {
	return torrTheme{base: theme.DefaultTheme()}
}

func (t torrTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if byName, ok := torrPalette[name]; ok {
		if c, ok := byName[variant]; ok {
			return c
		}
	}
	return t.base.Color(name, variant)
}

func (t torrTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.base.Font(style)
}

func (t torrTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t torrTheme) Size(name fyne.ThemeSizeName) float32 {
	if s, ok := torrSizes[name]; ok {
		return s
	}
	return t.base.Size(name)
}

// The theme is set on the app once, before any widget reads a colour from
// it. Fyne validates the theme's colours against its own contract — return
// NRGBA with a full byte for fully-opaque pixels, never alpha-out values —
// and falls back to "default theme" for every looked-up name that is not
// overridden here, which is the stock Fyne palette plus the Apple-adjacent
// sizing changes above.
func installTorrTheme(app fyne.App) {
	app.Settings().SetTheme(newTorrTheme())
}
