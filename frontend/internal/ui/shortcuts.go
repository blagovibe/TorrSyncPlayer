// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// shortcutSpec is one window hotkey: the keys as a person presses them, the
// catalogue key describing the action, and the action itself. Data, not
// wiring, so the suite pins the whole list without a canvas to press keys
// on. Player keys are not here on purpose: playback lives in mpv's window,
// and its keys live or die with it (see bug-0101).
type shortcutSpec struct {
	keys    string
	descKey string
	cut     fyne.Shortcut
	do      func()
}

// shortcutSpecs is the window's whole keyboard surface: focus the magnet
// field, refresh the list, open settings. A fourth row starts here, not in
// a handler — the dialog and the bindings both read this list.
func (w *window) shortcutSpecs() []shortcutSpec {
	return []shortcutSpec{
		{"Ctrl+L", "shortcut.focus", &desktop.CustomShortcut{KeyName: fyne.KeyL, Modifier: fyne.KeyModifierControl}, func() {
			if w.parent != nil {
				w.parent.Canvas().Focus(w.entry)
			}
		}},
		{"Ctrl+R", "shortcut.refresh", &desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl}, func() { w.refresh() }},
		{"Ctrl+,", "shortcut.settings", &desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierControl}, func() {
			w.showSettings()
		}},
	}
}

// bindShortcuts hangs the specs off the application window. Without a
// parent there is no canvas to hang them off, and silently binding to
// someone else's window would be worse than no shortcuts.
func (w *window) bindShortcuts() {
	if w.parent == nil {
		return
	}
	canvas := w.parent.Canvas()
	for _, spec := range w.shortcutSpecs() {
		canvas.AddShortcut(spec.cut, func(fyne.Shortcut) { spec.do() })
	}
}

// shortcutsText renders the dialog list: keys beside their descriptions.
// Key names are not translated — Ctrl+L is Ctrl+L in every language —
// while what each one does goes through the catalogue like everything else.
func shortcutsText(tr func(string) string, specs []shortcutSpec) string {
	lines := make([]string, 0, len(specs))
	for _, s := range specs {
		lines = append(lines, s.keys+" — "+tr(s.descKey))
	}
	return strings.Join(lines, "\n")
}

// showShortcuts opens the hotkey list. Like every dialog it needs a window
// to hang off; without one there is nothing to open.
func (w *window) showShortcuts() {
	parent := mainWindow()
	if parent == nil {
		return
	}
	dialog.NewCustom(w.tr("shortcuts.title"), w.tr("action.ok"),
		widget.NewLabel(shortcutsText(w.tr, w.shortcutSpecs())), parent).Show()
}
