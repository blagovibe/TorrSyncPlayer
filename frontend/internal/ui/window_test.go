// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
)

// TestTheWindowBuilds is the smoke test the constructor never had.
//
// Every other test in this package builds `&window{}` and calls one method on
// it, so the window builder — the one function every run of the application
// executes — was outside the suite. That is exactly where a missing widget
// hides: widget pointers are nil until assigned, and the subtitle chooser was
// never created, so the application died on launch while the suite stayed green.
//
// It builds through newWindow rather than NewWindow: NewWindow adds two loops
// that never stop, and a poller left behind keeps calling fyne.Do while later
// tests swap the current app under it — a race the detector is right about.
// The two lines NewWindow adds on top are the only thing not built here.
func TestTheWindowBuilds(t *testing.T) {
	// Widgets read the theme, so there has to be an app; nothing here starts
	// a goroutine, so it is safe to leave it behind.
	_ = test.NewApp()

	// A client aimed at a port nothing listens on: a failed poll must be
	// survivable, because that is the state of every launch — the window is
	// built before the backend answers.
	client := api.New("http://127.0.0.1:1", "test-token")

	w, content := newWindow(client, "peer-test", nil, WindowOptions{})
	if w == nil || content == nil {
		t.Fatal("the window constructor returned nothing")
	}
}

// A setting that nothing reads is worse than no setting: it is written, it is
// read back, it survives a restart, and it does nothing. The mpv binary was
// exactly that, and nothing about the settings file looked wrong.

// TestTheConfiguredPlayerIsTheOneLaunched locks the wiring from the settings
// file to the launcher.
//
// It does not prove mpv starts — that needs a real mpv speaking its control
// protocol. It proves the value reaches the launcher, which is the step that
// had silently vanished.
func TestTheConfiguredPlayerIsTheOneLaunched(t *testing.T) {
	w := &window{
		settings:   Settings{PlayerBinary: "/opt/custom/mpv"},
		socketPath: "/tmp/torrsync-test.sock",
	}

	options := w.mpvOptions()
	if options.Binary != "/opt/custom/mpv" {
		t.Errorf("the launcher would run %q, not the mpv that was configured", options.Binary)
	}
	if options.SocketPath != "/tmp/torrsync-test.sock" {
		t.Errorf("the control socket is %q", options.SocketPath)
	}
}

// TestAnEmptyPlayerBinaryLeavesTheChoiceToPATH is the other half: an empty
// setting must mean "the usual mpv", not "no player at all".
func TestAnEmptyPlayerBinaryLeavesTheChoiceToPATH(t *testing.T) {
	w := &window{socketPath: "/tmp/torrsync-test.sock"}

	if got := w.mpvOptions().Binary; got != "" {
		t.Errorf("an unset binary became %q, which asks the launcher for a file called that", got)
	}
}

// TestTheSocketPathIsPerProcess keeps two instances from sharing a control
// socket.
//
// This is not hypothetical: the project's own scenario tests run two backends,
// and a second application pointed at the first one's socket would drive
// somebody else's player — silently, because every command would succeed.
func TestTheSocketPathIsPerProcess(t *testing.T) {
	first := &window{socketPath: "/tmp/torrsync-1.sock"}
	second := &window{socketPath: "/tmp/torrsync-2.sock"}

	if first.mpvOptions().SocketPath == second.mpvOptions().SocketPath {
		t.Fatal("two windows would fight over one control socket")
	}
	if !strings.Contains(first.mpvOptions().SocketPath, ".sock") {
		t.Error("the socket path does not look like a socket path")
	}
}

// The --signaler flag is a deliberate override for one run: a saved setting
// must not silently outrank the person who typed the flag, and nobody having
// said anything must mean the public default rather than a dead address.
func TestWhereRoomsMeetComesFromTheFlagThenTheSetting(t *testing.T) {
	_ = test.NewApp()

	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := store.Save(Settings{Signaler: "ws://saved/"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	client := api.New("http://127.0.0.1:1", "test-token")

	flagged, _ := newWindow(client, "peer", store, WindowOptions{Signaler: "ws://flag/"})
	if got := flagged.signaler(); got != "ws://flag/" {
		t.Errorf("the flag was not the one used: got %q", got)
	}

	saved, _ := newWindow(client, "peer", store, WindowOptions{})
	if got := saved.signaler(); got != "ws://saved/" {
		t.Errorf("the saved signaler was not used: got %q", got)
	}

	def, _ := newWindow(client, "peer", nil, WindowOptions{})
	if got := def.signaler(); got != DefaultSignaler {
		t.Errorf("nobody chose a signaler and the window chose %q", got)
	}
}

// The --lang flag used to reach only the tray: the window kept reading the
// store, so --lang changed the tray menu and nothing else. The window is the
// surface where the choice is most visible, so it is the one locked here.
func TestTheLanguageFlagReachesTheWindow(t *testing.T) {
	withSystemLocale(t, "de")
	_ = test.NewApp()

	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := store.Save(Settings{Language: catalog.LangRussian}); err != nil {
		t.Fatalf("save: %v", err)
	}
	client := api.New("http://127.0.0.1:1", "test-token")

	flagged, _ := newWindow(client, "peer", store, WindowOptions{Language: "en"})
	if flagged.language != catalog.English {
		t.Errorf("--lang en left the window speaking %q", flagged.language)
	}

	saved, _ := newWindow(client, "peer", store, WindowOptions{})
	if saved.language != catalog.LangRussian {
		t.Errorf("without the flag the window ignored the saved language: %q", saved.language)
	}
}
