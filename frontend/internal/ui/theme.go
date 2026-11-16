// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	_ "embed"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The look is a home cinema, not a utility: a darkened room, a clean black
// hall, a projected off-white, and one marquee light for the actions that
// begin something. idea-0187 was filed for this; the owner picked the
// «Кинозал», the dark room — the default mood. The window is a darkened
// hall with a lit screen: a warm black up top cooling into pure black below,
// frosted glass panels floating on it, and one marquee amber for actions.
var (
	torrBackground = color.NRGBA{R: 0x0f, G: 0x0f, B: 0x12, A: 0xff} // #0F0F12
	torrSurface    = color.NRGBA{R: 0x18, G: 0x18, B: 0x1d, A: 0xff} // #18181D
	torrText       = color.NRGBA{R: 0xf4, G: 0xf2, B: 0xec, A: 0xff} // #F4F2EC
	torrMuted      = color.NRGBA{R: 0x8b, G: 0x8b, B: 0x93, A: 0xff} // #8B8B93
	torrAccent     = color.NRGBA{R: 0xe8, G: 0xa3, B: 0x3d, A: 0xff} // #E8A33D
	torrSeparator  = color.NRGBA{R: 0x23, G: 0x23, B: 0x29, A: 0xff} // #232329
	torrInputBG    = color.NRGBA{R: 0x13, G: 0x13, B: 0x18, A: 0xff} // #131318
	torrPanel      = color.NRGBA{R: 0x17, G: 0x17, B: 0x1d, A: 0xff} // #17171D — frosted panels
	torrPanelHi    = color.NRGBA{R: 0x1f, G: 0x1f, B: 0x27, A: 0xff} // #1F1F27 — raised glass
	torrHairline   = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x14} // white @ ~8%
	torrHallTop    = color.NRGBA{R: 0x1c, G: 0x17, B: 0x12, A: 0xff} // warm dark hall, top
	torrHallBottom = color.NRGBA{R: 0x0b, G: 0x0b, B: 0x0f, A: 0xff} // pure hall, bottom
	torrGlow       = color.NRGBA{R: 0xe8, G: 0xa3, B: 0x3d, A: 0x0d} // marquee glow, faint

	// «Дневной зал», the same geometry under daylight.
	torrBackgroundLight = color.NRGBA{R: 0xfb, G: 0xfa, B: 0xf7, A: 0xff} // #FBFAF7
	torrSurfaceLight    = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // #FFFFFF
	torrTextLight       = color.NRGBA{R: 0x16, G: 0x15, B: 0x0f, A: 0xff} // #16150F
	torrMutedLight      = color.NRGBA{R: 0x6f, G: 0x6d, B: 0x64, A: 0xff} // #6F6D64
	torrAccentLight     = color.NRGBA{R: 0xc0, G: 0x7e, B: 0x22, A: 0xff} // #C07E22
	torrSeparatorLight  = color.NRGBA{R: 0xe7, G: 0xe4, B: 0xdc, A: 0xff} // #E7E4DC
	torrInputBGLight    = color.NRGBA{R: 0xf3, G: 0xf1, B: 0xec, A: 0xff} // #F3F1EC
	torrPanelLight      = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // white frosted panels
	torrPanelHiLight    = color.NRGBA{R: 0xf3, G: 0xf1, B: 0xec, A: 0xff} // sunken paper
	torrHairlineLight   = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x0f} // ink @ ~6%
	torrHallTopLight    = color.NRGBA{R: 0xf6, G: 0xf4, B: 0xef, A: 0xff}
	torrHallBottomLight = color.NRGBA{R: 0xec, G: 0xe9, B: 0xe2, A: 0xff}
	torrGlowLight       = color.NRGBA{R: 0xc0, G: 0x7e, B: 0x22, A: 0x12}

	torrSelection   = color.NRGBA{R: 0xe8, G: 0xa3, B: 0x3d, A: 0x2e} // accent @ ~18%
	torrPressed     = color.NRGBA{R: 0xc9, G: 0x85, B: 0x2a, A: 0xff} // accent pressed darker
	torrError       = color.NRGBA{R: 0xe3, G: 0x5b, B: 0x4f, A: 0xff}
	torrSuccess     = color.NRGBA{R: 0x7f, G: 0xb0, B: 0x69, A: 0xff}
	torrWarning     = color.NRGBA{R: 0xe4, G: 0xc4, B: 0x65, A: 0xff}
	torrScrollBarBG = color.NRGBA{R: 0x1b, G: 0x1b, B: 0x21, A: 0xff}
	torrScrollBar   = color.NRGBA{R: 0x3a, G: 0x3a, B: 0x44, A: 0xff}
	torrDisabled    = color.NRGBA{R: 0x56, G: 0x56, B: 0x60, A: 0xff}

	torrSelectionLight   = color.NRGBA{R: 0xc0, G: 0x7e, B: 0x22, A: 0x2e}
	torrPressedLight     = color.NRGBA{R: 0xa6, G: 0x67, B: 0x17, A: 0xff}
	torrErrorLight       = color.NRGBA{R: 0xb5, G: 0x3d, B: 0x33, A: 0xff}
	torrSuccessLight     = color.NRGBA{R: 0x4e, G: 0x8a, B: 0x3c, A: 0xff}
	torrWarningLight     = color.NRGBA{R: 0xb5, G: 0x8f, B: 0x2f, A: 0xff}
	torrScrollBarBGLight = color.NRGBA{R: 0xee, G: 0xec, B: 0xe6, A: 0xff}
	torrScrollBarLight   = color.NRGBA{R: 0xd8, G: 0xd5, B: 0xcc, A: 0xff}
	torrDisabledLight    = color.NRGBA{R: 0xa8, G: 0xa6, B: 0x99, A: 0xff}
)

var torrPalette = map[fyne.ThemeColorName]map[fyne.ThemeVariant]color.Color{
	theme.ColorNameBackground:                {theme.VariantDark: torrBackground, theme.VariantLight: torrBackgroundLight},
	theme.ColorNameButton:                    {theme.VariantDark: torrPanel, theme.VariantLight: torrPanelLight},
	theme.ColorNameDisabledButton:            {theme.VariantDark: torrBackground, theme.VariantLight: torrBackgroundLight},
	theme.ColorNameDisabled:                  {theme.VariantDark: torrDisabled, theme.VariantLight: torrDisabledLight},
	theme.ColorNameError:                     {theme.VariantDark: torrError, theme.VariantLight: torrErrorLight},
	theme.ColorNameFocus:                     {theme.VariantDark: torrAccent, theme.VariantLight: torrAccentLight},
	theme.ColorNameForeground:                {theme.VariantDark: torrText, theme.VariantLight: torrTextLight},
	theme.ColorNameForegroundOnError:         {theme.VariantDark: torrBackground, theme.VariantLight: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}},
	theme.ColorNameForegroundOnPrimary:       {theme.VariantDark: color.NRGBA{R: 0x18, G: 0x18, B: 0x1d, A: 0xff}, theme.VariantLight: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}},
	theme.ColorNameForegroundOnSuccess:       {theme.VariantDark: torrBackground, theme.VariantLight: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}},
	theme.ColorNameForegroundOnWarning:       {theme.VariantDark: torrBackground, theme.VariantLight: torrTextLight},
	theme.ColorNameHeaderBackground:          {theme.VariantDark: torrPanel, theme.VariantLight: torrPanelLight},
	theme.ColorNameHover:                     {theme.VariantDark: torrAccent, theme.VariantLight: torrAccentLight},
	theme.ColorNameHyperlink:                 {theme.VariantDark: torrAccent, theme.VariantLight: torrAccentLight},
	theme.ColorNameInnerWindowBorder:         {theme.VariantDark: torrSeparator, theme.VariantLight: torrSeparatorLight},
	theme.ColorNameInnerWindowBorderInactive: {theme.VariantDark: torrSeparator, theme.VariantLight: torrSeparatorLight},
	theme.ColorNameInputBackground:           {theme.VariantDark: torrInputBG, theme.VariantLight: torrInputBGLight},
	theme.ColorNameInputBorder:               {theme.VariantDark: torrSeparator, theme.VariantLight: torrSeparatorLight},
	theme.ColorNameMenuBackground:            {theme.VariantDark: torrPanel, theme.VariantLight: torrPanelLight},
	theme.ColorNameOverlayBackground:         {theme.VariantDark: torrBackground, theme.VariantLight: torrBackgroundLight},
	theme.ColorNamePlaceHolder:               {theme.VariantDark: torrMuted, theme.VariantLight: torrMutedLight},
	theme.ColorNamePressed:                   {theme.VariantDark: torrPressed, theme.VariantLight: torrPressedLight},
	theme.ColorNamePrimary:                   {theme.VariantDark: torrAccent, theme.VariantLight: torrAccentLight},
	theme.ColorNameScrollBar:                 {theme.VariantDark: torrScrollBar, theme.VariantLight: torrScrollBarLight},
	theme.ColorNameScrollBarBackground:       {theme.VariantDark: torrScrollBarBG, theme.VariantLight: torrScrollBarBGLight},
	theme.ColorNameSelection:                 {theme.VariantDark: torrSelection, theme.VariantLight: torrSelectionLight},
	theme.ColorNameSeparator:                 {theme.VariantDark: torrSeparator, theme.VariantLight: torrSeparatorLight},
	theme.ColorNameShadow:                    {theme.VariantDark: color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xa0}, theme.VariantLight: color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x30}},
	theme.ColorNameSuccess:                   {theme.VariantDark: torrSuccess, theme.VariantLight: torrSuccessLight},
	theme.ColorNameWarning:                   {theme.VariantDark: torrWarning, theme.VariantLight: torrWarningLight},
}

var torrSizes = map[fyne.ThemeSizeName]float32{
	theme.SizeNameText:               13,
	theme.SizeNameHeadingText:        26, // the display size: the film's own name
	theme.SizeNameSubHeadingText:     15,
	theme.SizeNameCaptionText:        11,
	theme.SizeNameButtonRadius:       8,
	theme.SizeNameCardRadius:         10,
	theme.SizeNamePopupRadius:        10,
	theme.SizeNameDialogRadius:       12,
	theme.SizeNameInputRadius:        8,
	theme.SizeNameSelectionRadius:    6,
	theme.SizeNameLineSpacing:        1.18,
	theme.SizeNameInlineIcon:         22,
	theme.SizeNamePadding:            9,
	theme.SizeNameInnerPadding:       12,
	theme.SizeNameSeparatorThickness: 1,
	theme.SizeNameSplitThickness:     3,
}

//go:embed assets/fonts/Inter-ExtraBold.ttf
var interExtraBold []byte

//go:embed assets/fonts/Inter-Regular.ttf
var interRegular []byte

//go:embed assets/fonts/Inter-Bold.ttf
var interBold []byte

var (
	interRegularRes   = &fyne.StaticResource{StaticName: "Inter-Regular.ttf", StaticContent: interRegular}
	interBoldRes      = &fyne.StaticResource{StaticName: "Inter-Bold.ttf", StaticContent: interBold}
	interExtraBoldRes = &fyne.StaticResource{StaticName: "Inter-ExtraBold.ttf", StaticContent: interExtraBold}
)

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

// The room's own typeface. Fyne embeds Noto Sans; this window speaks Inter,
// because the difference between a system toolkit and a designed product is
// felt in the letters before it is seen in the chrome.
func (t torrTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Bold {
		return interBoldRes
	}
	return interRegularRes
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

// torrFixedTheme pins a variant: the user's checkbox decides the mood of the
// window outright instead of deferring to the desktop, so the palette is the
// same on a dark laptop in daylight and a light one at night. This is what
// the stock DarkTheme/LightTheme pair used to do — and what kept the warm
// palette from ever reaching the person in v0.4.8–v0.4.10 (the checkbox
// overwrote TorrTheme with the stock theme at startup and after every
// settings save).
type torrFixedTheme struct {
	base    torrTheme
	variant fyne.ThemeVariant
}

func (t torrFixedTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return t.base.Color(name, t.variant)
}

func (t torrFixedTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.base.Font(style)
}

func (t torrFixedTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t torrFixedTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.base.Size(name)
}

// InstallTorrTheme pins the warm palette to the app in the requested
// mood. dark=true yields the kino-zal, false the daylight hall.
func InstallTorrTheme(app fyne.App, dark bool) {
	torrMoodDark.Store(dark)
	v := theme.VariantLight
	if dark {
		v = theme.VariantDark
	}
	app.Settings().SetTheme(torrFixedTheme{base: newTorrTheme(), variant: v})
}
