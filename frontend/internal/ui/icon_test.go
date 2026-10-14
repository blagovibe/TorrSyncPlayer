// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE for full license text

package ui

import (
	"bytes"
	"image/png"
	"testing"
)

// The tray icon is drawn in code rather than shipped as an asset, so nothing
// proves it is a valid image. A desktop that cannot decode it refuses the tray
// outright, and the application then starts without one — which is the failure
// the drawn icon was chosen to avoid.

// TestTheIconIsARealImageAtEverySizeAskesFor keeps the drawing honest at the
// sizes the tray and the packager actually use. The packager asks for 256: a
// mark that only renders correctly at 32 would scale into a blob on a desktop
// menu, and a blob in a menu is how an application looks uninstalled.
func TestTheIconIsARealImageAtEverySizeAskesFor(t *testing.T) {
	for _, size := range []int{trayIconSize, 128, 256} {
		icon, err := IconPNG(size)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		if len(icon) == 0 {
			t.Fatalf("size %d produced no bytes", size)
		}

		img, err := png.Decode(bytes.NewReader(icon))
		if err != nil {
			t.Fatalf("size %d did not decode as PNG: %v", size, err)
		}
		bounds := img.Bounds()
		if bounds.Dx() != size || bounds.Dy() != size {
			t.Errorf("asked for %d, got %dx%d", size, bounds.Dx(), bounds.Dy())
		}
	}
}

// TestAnAbsurdIconSizeIsStillAnImage: the size comes from a command line, and
// a value of zero must not produce an image no decoder will take.
func TestAnAbsurdIconSizeIsStillAnImage(t *testing.T) {
	icon, err := IconPNG(0)
	if err != nil {
		t.Fatalf("size 0: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(icon)); err != nil {
		t.Fatalf("size 0 did not decode: %v", err)
	}
}

// TestTheIconIsNotBlank catches the failure that matters most for packaging: a
// file that exists, decodes, and shows nothing. This is exactly what bug-0034
// shipped — sixteen PNGs that were eight bytes of magic with no image in them.
func TestTheIconIsNotBlank(t *testing.T) {
	icon, err := IconPNG(256)
	if err != nil {
		t.Fatalf("icon: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	opaque := 0
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			if r == 0 && g == 0 && b == 0 {
				continue
			}
			opaque++
		}
	}
	if opaque < 1000 {
		t.Errorf("only %d visible pixels in a 256x256 icon — this is the eight-byte-blob bug again", opaque)
	}
}
