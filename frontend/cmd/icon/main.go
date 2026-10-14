// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE for full license text

// Command icon writes the application icon as a PNG file.
//
// The mark is drawn in code (internal/ui.IconPNG) rather than shipped as an
// asset, because an asset has to be kept in step with the tray, which is drawn
// for whatever density a desktop asks for. Packaging needs the same mark as
// bytes on disk — an AppDir and a .desktop entry do not take a function — so it
// is rendered here at packaging time from the same code that draws the tray.
//
//	cd frontend
//	go run ./cmd/icon -size 256 -out ../assets/TorrSyncPlayer.png
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/ui"
)

func main() {
	size := flag.Int("size", 256, "width and height in pixels")
	out := flag.String("out", "TorrSyncPlayer.png", "file to write")
	flag.Parse()

	if err := write(*out, *size); err != nil {
		fmt.Fprintln(os.Stderr, "icon:", err)
		os.Exit(1)
	}
	fmt.Println(*out)
}

func write(out string, size int) error {
	png, err := ui.IconPNG(size)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(out); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(out, png, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}
	return nil
}