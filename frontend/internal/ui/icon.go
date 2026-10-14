// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE for full license text

package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

// The application's mark: a disc with a play triangle on it.
//
// Drawn rather than shipped, for the reason the tray needs it drawn: an asset
// has to be regenerated for every density a platform asks for, and a desktop
// that cannot use the tray icon refuses to show a tray at all — which turns a
// cosmetic detail into a failed start. Packaging needs the same mark as a real
// file (an AppImage and a .desktop entry want bytes on disk), so it is drawn
// once at whatever size is being asked for rather than kept as an asset that
// has to be kept in step with the drawing.

// trayIconSize is what the tray asks for.
const trayIconSize = 32

// IconPNG returns the application icon at the given size, as PNG bytes.
//
// A size below 8 is raised to 8: the mark is a disc with a triangle inside it,
// and below that the triangle is a few pixels of noise that no desktop would
// render legibly anyway.
func IconPNG(size int) ([]byte, error) {
	if size < 8 {
		size = 8
	}

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	disc := color.NRGBA{R: 0x1f, G: 0x6f, B: 0xeb, A: 0xff}
	triangle := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	centre, radius := float64(size)/2, float64(size)/2-1
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x)+0.5-centre, float64(y)+0.5-centre
			dist := math.Sqrt(dx*dx + dy*dy)

			// A one-pixel feather on the edge, because a hard-edged circle at
			// small sizes reads as a jagged blob.
			alpha := clamp01(radius - dist + 0.5)
			if alpha == 0 {
				continue
			}

			pixel := color.NRGBA{
				R: uint8(float64(disc.R) * alpha),
				G: uint8(float64(disc.G) * alpha),
				B: uint8(float64(disc.B) * alpha),
				A: uint8(255 * alpha),
			}

			// The play triangle, sized to the icon so it stays the same mark at
			// 32 pixels in a tray strip and at 256 on a desktop menu.
			if insideTriangle(float64(x), float64(y),
				centre-unit(size, 4), centre-unit(size, 6),
				centre-unit(size, 4), centre+unit(size, 6),
				centre+unit(size, 7), centre) {
				pixel = triangle
			}
			img.SetNRGBA(x, y, pixel)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode icon: %w", err)
	}
	return buf.Bytes(), nil
}

// unit scales a 32-pixel design measurement to whatever size is being drawn, so
// the triangle keeps its proportions instead of swallowing the disc.
func unit(size int, at32 float64) float64 {
	return at32 * float64(size) / trayIconSize
}
