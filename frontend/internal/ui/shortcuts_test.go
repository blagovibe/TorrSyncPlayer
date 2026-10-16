// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
)

// The dialog and the bindings read one list, so this pins the list itself:
// three rows, the exact keys, and actions that run without a backend or a
// display. A row added to the dialog but not the bindings — or the reverse —
// fails here first.
func TestTheWindowListsItsOwnShortcuts(t *testing.T) {
	_ = test.NewApp()
	tr := func(key string) string { return catalog.Lookup(catalog.English, key) }
	client := api.New("http://127.0.0.1:1", "test-token")
	parent := test.NewWindow(widget.NewLabel("parent"))
	defer parent.Close()

	w, content := newWindow(client, "peer-test", nil, WindowOptions{Parent: parent})
	parent.SetContent(content)
	specs := w.shortcutSpecs()
	if len(specs) != 3 {
		t.Fatalf("the window promises three shortcuts, got %d", len(specs))
	}

	type row struct {
		keys string
		desc string
		name fyne.KeyName
	}
	want := []row{
		{"Ctrl+L", "shortcut.focus", fyne.KeyL},
		{"Ctrl+R", "shortcut.refresh", fyne.KeyR},
		{"Ctrl+,", "shortcut.settings", fyne.KeyComma},
	}
	for i, r := range want {
		if specs[i].keys != r.keys || specs[i].descKey != r.desc {
			t.Errorf("row %d reads %q/%q, want %q/%q", i, specs[i].keys, specs[i].descKey, r.keys, r.desc)
		}
		cut, ok := specs[i].cut.(*desktop.CustomShortcut)
		if !ok || cut.KeyName != r.name || cut.Modifier != fyne.KeyModifierControl {
			t.Errorf("row %d binds %v, want Ctrl+%s", i, specs[i].cut, r.name)
		}
	}

	if got, wantText := shortcutsText(tr, specs),
		"Ctrl+L — Focus the magnet field\nCtrl+R — Refresh the torrent list\nCtrl+, — Open settings"; got != wantText {
		t.Errorf("got %q, want %q", got, wantText)
	}

	// Every action must run headless: focus lands on the entry, refresh
	// fails fast against the dead backend, settings bails without a store.
	for _, s := range specs {
		s.do()
	}
	if parent.Canvas().Focused() != w.entry {
		t.Error("the focus shortcut must land on the magnet field")
	}
}
