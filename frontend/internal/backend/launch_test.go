// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The launch grew from (path, port, token) into a struct because the child
// backend is told more than where to listen: which storage to use. Each of
// those has exactly one honest channel — the command line for what anyone
// may see, the environment for what must not travel in the process list —
// and the tests below pin the channel, not just the value.

// TestLaunchArgumentsCarryNoSecret is the deviation-0061 promise kept for the
// new fields: the access token travels on standard input, so it does not
// appear among the arguments where any local user can read it out of the
// process list.
func TestLaunchArgumentsCarryNoSecret(t *testing.T) {
	launch := Launch{
		Executable:  "torrsyncplayer",
		Port:        1234,
		Token:       "access-secret",
		DiskStorage: true,
		DataDir:     "/data/here",
	}

	args := Args(launch)
	if arg, ok := UnknownFlag(args); !ok {
		t.Fatalf("the backend would refuse %q", arg)
	}
	for _, arg := range args {
		if strings.Contains(arg, "access-secret") {
			t.Fatalf("the secret appears on the command line as %q", arg)
		}
	}
}

// TestLaunchArgumentsCarryTheStorageChoice locks the channel: storage travels
// as flags, not as the DATA_DIR/DISK_STORAGE environment the backend also
// accepts. A flag is not a default, so nothing inherited from a shell can
// stand in for the choice made in the window; the empty directory is passed on
// purpose when storage is memory, which is the backend's documented way to
// keep nothing on disk at all.
func TestLaunchArgumentsCarryTheStorageChoice(t *testing.T) {
	memory := strings.Join(Args(Launch{Port: 8889, DataDir: ""}), " ")
	if memory != "--addr 127.0.0.1:8889 --data-dir " {
		t.Fatalf("memory storage produced %q", memory)
	}

	disk := strings.Join(Args(Launch{Port: 8889, DiskStorage: true, DataDir: "/pieces"}), " ")
	if disk != "--addr 127.0.0.1:8889 --data-dir /pieces --disk-storage" {
		t.Fatalf("disk storage produced %q", disk)
	}
}

// TestLaunchEnvironmentReplacesRatherThanAppends covers the shell that exports
// DATA_DIR or DISK_STORAGE: the backend reads both as flag defaults, so a
// value the launch did not choose must be replaced, and one the launch does
// not set at all must not survive from the parent.
func TestLaunchEnvironmentReplacesRatherThanAppends(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"LANG=ru_RU.UTF-8",
		"DATA_DIR=/somebody/elses/pieces",
		"DISK_STORAGE=true",
	}
	launch := Launch{
		DiskStorage: true,
		DataDir:     "/chosen/pieces",
	}

	env := environment(LaunchEnv(parent, launch))

	if env["PATH"] != "/usr/bin" || env["LANG"] != "ru_RU.UTF-8" {
		t.Error("unrelated variables were dropped from the child's environment")
	}
	for _, stale := range []string{"DATA_DIR", "DISK_STORAGE"} {
		if value, ok := env[stale]; ok && value != "" {
			t.Errorf("%s reached the child as %q; storage is decided by flags", stale, value)
		}
	}
}

// TestMemoryStorageLeavesNoStorageBehind is the removal half: with no storage
// configured, a stale choice from the parent must not reach the child, where
// the backend would treat it as configuration.
func TestMemoryStorageLeavesNoStorageBehind(t *testing.T) {
	parent := []string{
		"DATA_DIR=/somewhere/else",
		"DISK_STORAGE=true",
	}

	env := environment(LaunchEnv(parent, Launch{}))

	for _, key := range []string{"DISK_STORAGE", "DATA_DIR"} {
		if value, ok := env[key]; ok && value != "" {
			t.Errorf("%s reached the child as %q", key, value)
		}
	}
}

// TestStartHandsTheChildTheLaunchEnvironment is the wiring the pure functions
// above cannot see. The environment reaching the actual process is the step
// that goes missing silently — the settings would look applied and the child
// would run with whatever the shell had.
//
// Storage is deliberately absent here: it travels as arguments (see
// TestLaunchArgumentsCarryTheStorageChoice), and this stand-in proves the
// scrubbing holds for a child start.
func TestStartHandsTheChildTheLaunchEnvironment(t *testing.T) {
	out := filepath.Join(t.TempDir(), "child-environment")
	standIn := script(t, `printf '%s|%s' "$DATA_DIR" "$DISK_STORAGE" > `+out)

	process, err := Start(Launch{
		Executable:  standIn,
		DataDir:     "/pieces",
		DiskStorage: true,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	select {
	case <-process.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the stand-in backend never finished")
	}
	if err := process.ExitError(); err != nil {
		t.Fatalf("the stand-in failed: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the stand-in wrote no environment: %v", err)
	}
	if string(got) != "|" {
		t.Fatalf("the child saw DATA_DIR|DISK_STORAGE = %q, want %q",
			string(got), "|")
	}
}

// environment is a test-side view of an environment list.
func environment(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		out[key] = value
	}
	return out
}
