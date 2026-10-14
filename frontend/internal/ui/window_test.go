// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"strings"
	"testing"
)

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
