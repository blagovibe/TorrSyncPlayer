// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
)

// A poster tile must put pixels on the shelf: draw two fixtures and look.
func TestPosterTilesDraw(t *testing.T) {
	a := test.NewApp()
	InstallTorrTheme(a, true)

	w := &window{language: "en", rates: map[string]rateSample{}}

	art, err := posterArtFor(w.tr, Torrent{Name: "Jimmy Neutron: Boy Genius (2001).mkv", Size: 2384185792, Progress: 0.42, Status: "downloading", Peers: 3}, false)
	if err != nil {
		t.Fatalf("poster art: %v", err)
	}

	bounds := art.Bounds()
	nonZero := 0
	for y := 0; y < bounds.Dy(); y += 7 {
		for x := 0; x < bounds.Dx(); x += 7 {
			if a := art.NRGBAAt(x, y).A; a > 0 {
				nonZero++
			}
		}
	}
	if nonZero == 0 {
		t.Fatal("the poster art is fully transparent — the mask or the gradient never landed")
	}
	t.Logf("poster art %dx%d, %d sampled opaque pixels", bounds.Dx(), bounds.Dy(), nonZero)

	tile := newPosterTile(w, &Torrent{Name: "Jimmy Neutron: Boy Genius (2001).mkv", Size: 2384185792, Progress: 0.42, Status: "downloading", Peers: 3}, false)
	c := software.NewCanvas()
	c.SetContent(container.NewStack(tile))
	c.Resize(fyne.NewSize(300, 460))

	path := os.Getenv("TSP_TILE")
	if path == "" {
		path = filepath.Join(t.TempDir(), "tile.png")
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer file.Close()
	if err := png.Encode(file, c.Capture()); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
	t.Logf("tile written to %s", path)
}
