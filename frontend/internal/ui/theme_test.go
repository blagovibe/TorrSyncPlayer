// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

func TestTorrThemeAnswersWithWarmPalette(t *testing.T) {
	tt := newTorrTheme()

	if c := tt.Color(theme.ColorNameBackground, theme.VariantDark); c != torrBackground {
		t.Errorf("dark background resolved to %v, want the warm near-black %v", c, torrBackground)
	}
	if c := tt.Color(theme.ColorNameBackground, theme.VariantLight); c != torrBackgroundLight {
		t.Errorf("light background resolved to %v, want the warm paper %v", c, torrBackgroundLight)
	}
	if c := tt.Color(theme.ColorNamePrimary, theme.VariantDark); c != torrAccent {
		t.Errorf("dark primary resolved to %v, want the terracotta %v", c, torrAccent)
	}
	if c := tt.Color(theme.ColorNamePrimary, theme.VariantLight); c != torrAccentLight {
		t.Errorf("light primary resolved to %v, want the terracotta %v", c, torrAccentLight)
	}
}

// The theme has to fall through to something for every name it doesn't
// know. An absent override must not turn zero-value-nowhere choices into a
// blank window.
func TestTorrThemeFallsThroughToStockForUnknownNames(t *testing.T) {
	tt := newTorrTheme()
	if c := tt.Color("unknown-override-not-used", theme.VariantDark); c == (color.NRGBA{}) {
		t.Fatal("an unknown name resolved to the zero colour — the fallback is broken")
	}
	if f := tt.Font(fyne.TextStyle{}); f == nil {
		t.Fatal("the fallback font is nil")
	}
	if f := tt.Icon("confirm"); f == nil {
		t.Fatal("the fallback icon for a known name is nil")
	}
	if s := tt.Size("unknown"); s < 0 {
		t.Fatal("an unknown size resolved to a negative value")
	}
}
