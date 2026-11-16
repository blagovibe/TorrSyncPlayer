// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// The plates render correctly in isolation, or the whole shelf idea is
// wrong: paint three fixture rows and look at them.
func TestThePlatesRender(t *testing.T) {
	a := test.NewApp()
	InstallTorrTheme(a, true)

	rows := []struct {
		name  string
		sub   string
		value float64
	}{
		{"Jimmy Neutron: Boy Genius (2001).mkv", "Скачивание · 42% · 2.8 MB/s", 0.42},
		{"Moscow Zero (S01E01).mkv", "Раздача", 1},
	}
	vbox := container.NewVBox()
	for _, r := range rows {
		p := newTorrentPlate()
		name, sub, bar, _, _, ok := torrentPlateParts(p)
		if !ok {
			t.Fatal("newTorrentPlate does not match the parts accessor")
		}
		name.SetText(r.name)
		sub.Text = r.sub
		sub.Refresh()
		bar.SetValue(r.value)
		vbox.Add(p)
	}

	list := widget.NewList(
		func() int { return len(rows) },
		func() fyne.CanvasObject { return newTorrentPlate() },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			row, sub, bar, _, _, ok := torrentPlateParts(obj)
			if !ok {
				return
			}
			row.SetText(rows[id].name)
			sub.Text = rows[id].sub
			sub.Refresh()
			bar.SetValue(rows[id].value)
		},
	)

	content := container.NewVScroll(list)
	c := software.NewCanvas()
	c.SetContent(content)
	c.Resize(fyne.NewSize(640, 300))

	img := c.Capture()
	path := os.Getenv("TSP_SHOT")
	if path == "" {
		path = filepath.Join(t.TempDir(), "plate.png")
	} else {
		ext := filepath.Ext(path)
		path = strings.TrimSuffix(path, ext) + "-plate" + ext
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
	t.Logf("frame written to %s", path)
}
