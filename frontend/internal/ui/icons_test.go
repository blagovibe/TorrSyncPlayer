// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"bytes"
	"image"
	"testing"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

// Every icon in the set must parse and must actually put pixels on the
// canvas — a typo in a path string renders as a silent blank square in a
// real button, and nobody would know until someone stared at the window.
func TestTorrIconsParseAndDraw(t *testing.T) {
	for name, svgText := range torrIcons {
		icon, err := oksvg.ReadReplacingCurrentColor(bytes.NewReader([]byte(svgText)), "#FFFFFF")
		if err != nil {
			t.Errorf("%s: does not parse: %v", name, err)
			continue
		}
		icon.SetTarget(0, 0, 24, 24)
		img := image.NewRGBA(image.Rect(0, 0, 24, 24))
		scanner := rasterx.NewScannerGV(24, 24, img, img.Bounds())
		dasher := rasterx.NewDasher(24, 24, scanner)
		icon.Draw(dasher, 1.0)

		drawn := 0
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] != 0 {
				drawn++
			}
		}
		if drawn == 0 {
			t.Errorf("%s: parses but draws nothing — the glyph is invisible", name)
		}
	}
}

// The fullscreen pair must be distinguishable: they ship side by side in the
// transport row and a swapped pair turns the toggle into a mystery.
func TestTorrIconsAreDistinct(t *testing.T) {
	names := make(map[string]string, len(torrIcons))
	for name, svgText := range torrIcons {
		if previous, seen := names[svgText]; seen {
			t.Errorf("icons %q and %q are the same glyph", previous, name)
		}
		names[svgText] = name
	}
}

// A backplate path marked fill="none" must exist in every icon: without it
// oksvg still draws the glyph, but the canvas offers nothing to bound the
// icon to and themes that fill containers pick the wrong bounds.
func TestTorrIconsCarryATransparentBackplate(t *testing.T) {
	for name, svgText := range torrIcons {
		if !bytes.Contains([]byte(svgText), []byte(`fill="none"`)) {
			t.Errorf("%s: missing the fill=\"none\" backplate", name)
		}
	}
}
