// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// The release packages ship mpv one directory away from the application:
// beside the exe on Windows, in Contents/MacOS inside the .app on macOS.
// bundleCandidatesFor must say exactly that per OS — the released
// application promises the player works out of the box, and a candidate
// that misses the ship misses the promise on exactly the platforms where
// the user cannot fix PATH.
func TestBundleCandidatesFor(t *testing.T) {
	dir := t.TempDir()
	windows := bundleCandidatesFor("windows", dir)
	if len(windows) != 1 || windows[0] != filepath.Join(dir, "mpv.exe") {
		t.Fatalf("windows bundle candidate wrong: %v", windows)
	}
	darwin := bundleCandidatesFor("darwin", dir)
	if len(darwin) != 1 || darwin[0] != filepath.Join(dir, "mpv") {
		t.Fatalf("darwin bundle candidate wrong: %v", darwin)
	}
	linux := bundleCandidatesFor("linux", dir)
	if len(linux) != 1 || linux[0] != filepath.Join(dir, "mpv") {
		t.Fatalf("linux bundle candidate wrong: %v", linux)
	}
}

// The order of resolution is the contract: configured beats everything,
// PATH beats the bundle (a system mpv is the better-seated one), the
// bundle beats the well-known places (the release says mpv is inside),
// and the bundle is used when there is nothing else to say.
func TestMpvExecutablePrefersInThisOrder(t *testing.T) {
	dir := t.TempDir()
	name := "mpv"
	if filepath.Separator == '\\' {
		name = "mpv.exe"
	}
	configured := filepath.Join(dir, "configured-"+name)
	bundle := filepath.Join(dir, "bundle-mpv")
	wellKnown := filepath.Join(dir, "wellknown-mpv")
	// A shebang line, exactly what the PATH test above writes: Windows does
	// not treat extensionless files as executables without something that
	// proves it is a script.
	for _, path := range []string{configured, bundle, wellKnown} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// The configured binary wins over the bundle when PATH is empty of answers.
	t.Setenv("PATH", dir)
	if got, ok := mpvExecutableWithCandidates(configured, nil, []string{bundle}); !ok || got != configured {
		t.Errorf("configured should win over the bundle: ok=%v got=%q", ok, got)
	}

	// A PATH mpv beats the bundle (a system mpv is the better-seated one).
	pathDir := t.TempDir()
	onPath := filepath.Join(pathDir, name)
	if err := os.WriteFile(onPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pathDir)
	if got, ok := mpvExecutableWithCandidates("", nil, []string{bundle}); !ok || got != "mpv" {
		t.Errorf("PATH mpv should win over the bundle: ok=%v got=%q", ok, got)
	}

	// PATH empty, bundle present: the bundle is used, the well-known places are not.
	emptyPath := filepath.Join(dir, "emptypath")
	if err := os.Mkdir(emptyPath, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", emptyPath)
	if got, ok := mpvExecutableWithCandidates("", []string{wellKnown}, []string{bundle}); !ok || got != bundle {
		t.Errorf("the bundle should win over well-known locations: ok=%v got=%q", ok, got)
	}

	// Nothing anywhere works out: zero is an answer, not silence.
	if got, ok := mpvExecutableWithCandidates("", nil, nil); ok || got != "" {
		t.Errorf("no mpv anywhere must report none: ok=%v got=%q", ok, got)
	}
}
