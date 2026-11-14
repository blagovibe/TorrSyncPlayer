// SPDX-License-Identifier: MIT
// Copyright (c) 2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// computeRate is what the row's speed is made of. The edge cases are the
// whole story: a rate from zero elapsed time, from a byte count that went
// backwards (re-seed of the same torrent), or from no growth at all must be
// zero, never something that shows the person a fresh speed out of nothing.
func TestComputeRate(t *testing.T) {
	assert := func(prev, now int64, dt, want float64) {
		got := computeRate(prev, now, dt)
		if got != want {
			t.Errorf("computeRate(%d, %d, %.3f) = %.3f, want %.3f", prev, now, dt, got, want)
		}
	}
	assert(1000, 2000, 0.5, 2000)
	assert(1000, 2000, 0, 0)
	assert(1000, 2000, -1, 0)
	assert(2000, 1000, 1, 0) // a byte count that went backwards tells nothing about speed
	assert(0, 0, 1, 0)
	assert(1000, 1000, 1, 0)
}

// mpvCandidates is per-OS by construction; the test pins the shape of the
// answer rather than one particular box, so it survives machine differences.
func TestMpvCandidates(t *testing.T) {
	if paths := mpvCandidates("windows"); len(paths) == 0 || !strings.HasSuffix(paths[0], `mpv.exe`) {
		t.Fatalf("windows candidates are not executables: %v", paths)
	}
	if paths := mpvCandidates("darwin"); len(paths) == 0 || paths[0] != "/opt/homebrew/bin/mpv" {
		t.Fatalf("darwin candidates are not what brew installs say: %v", paths)
	}
	if paths := mpvCandidates("linux"); len(paths) == 0 || !strings.HasPrefix(paths[0], "/") {
		t.Fatalf("linux candidates should be absolute paths: %v", paths)
	}
}

// mpvExecutable covers the states that exist in the wild, without depending
// on the ambient machine: the configured binary wins, then PATH, then the
// well-known locations.
func TestMpvExecutable(t *testing.T) {
	fake, err := os.CreateTemp(t.TempDir(), "fake-mpv-*.exe")
	if err != nil {
		t.Fatal(err)
	}
	fakePath := fake.Name()
	_ = fake.Close()
	if err := os.Chmod(fakePath, 0o700); err != nil {
		t.Fatal(err)
	}

	// The configured binary wins over PATH.
	t.Setenv("PATH", t.TempDir())
	if path, ok := mpvExecutable(fakePath); !ok || path != fakePath {
		t.Fatalf("configured binary was not used: ok=%v path=%q", ok, path)
	}

	// A configured binary that does not exist falls back to PATH.
	pathEnv := t.TempDir()
	name := "mpv"
	if filepath.Separator == '\\' {
		name = "mpv.exe"
	}
	fakeOnPath := filepath.Join(pathEnv, name)
	if err := os.WriteFile(fakeOnPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pathEnv)
	if path, ok := mpvExecutable(fakePath + "-missing"); !ok || path == "" {
		t.Fatalf("a configured binary that is gone must fall back to PATH: ok=%v path=%q", ok, path)
	}
}

// mpvInstallHint must pick the right key per OS, and the text must carry the
// instruction a person needs, not a stack trace.
func TestMpvInstallHint(t *testing.T) {
	tr := func(key string) string {
		switch key {
		case "mpv.missing.body":
			return "mpv cannot be found"
		case "mpv.missing.note":
			return "install it"
		case "mpv.missing.linux":
			return "apt"
		case "mpv.missing.macos":
			return "brew"
		case "mpv.missing.windows":
			return "winget"
		default:
			return ""
		}
	}
	linux := mpvInstallHint(tr, "linux")
	if !strings.Contains(linux, "apt") {
		t.Fatalf("the linux hint does not name apt: %q", linux)
	}
	darwin := mpvInstallHint(tr, "darwin")
	if !strings.Contains(darwin, "brew") {
		t.Fatalf("the macOS hint does not name brew: %q", darwin)
	}
	windows := mpvInstallHint(tr, "windows")
	if !strings.Contains(windows, "winget") {
		t.Fatalf("the windows hint does not name winget: %q", windows)
	}
	other := mpvInstallHint(tr, "plan9")
	if !strings.Contains(other, "apt") {
		t.Fatalf("an unknown OS should get the linux hint, not an empty one: %q", other)
	}
}
