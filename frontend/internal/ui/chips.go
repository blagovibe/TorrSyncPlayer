// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"fmt"

	"fyne.io/fyne/v2/widget"
)

// The file picker for multi-file torrents: numbered capsules the length of
// their names. One tap plays one file; the active one wears the amber. A
// single-file torrent hides the row entirely — there is nothing to choose.
func (w *window) rebuildFileChips() {
	w.mu.RLock()
	files := append([]fileEntry(nil), w.files...)
	selected := w.selectedFile
	w.mu.RUnlock()

	onMain(func() {
		w.fileChips.RemoveAll()
		if len(files) < 2 {
			w.fileChips.Hide()
			return
		}
		for i, f := range files {
			if i >= 12 {
				break // the footer is a whisper, not a table
			}
			file := f
			btn := widget.NewButton(fmt.Sprintf("%d · %s", file.Index, truncateRunes(file.Name, 42)), func() {
				w.playFile(file)
			})
			if file.Index == selected {
				btn.Importance = widget.HighImportance
			} else {
				btn.Importance = widget.LowImportance
			}
			w.fileChips.Add(btn)
		}
		w.fileChips.Show()
		w.fileChips.Refresh()
	})
}
