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
// backend is told more than where to listen: which storage to use and whether
// an htorrent gateway owns the torrents. Each of those has exactly one honest
// channel — the command line for what anyone may see, the environment for what
// must not travel in the process list — and the tests below pin the channel,
// not just the value.

// TestLaunchArgumentsCarryNoSecret is the deviation-0061 promise kept for the
// new fields: the access token travels on standard input and the gateway
// password in the environment, so neither appears among the arguments where
// any local user can read it out of the process list.
func TestLaunchArgumentsCarryNoSecret(t *testing.T) {
	launch := Launch{
		Executable:    "torrsyncplayer",
		Port:          1234,
		Token:         "access-secret",
		DiskStorage:   true,
		DataDir:       "/data/here",
		GatewayURL:    "https://gateway.example.com",
		GatewayUser:   "friend",
		GatewayPass:   "gateway-secret",
		GatewayCACert: "/etc/ca.pem",
	}

	args := Args(launch)
	if arg, ok := UnknownFlag(args); !ok {
		t.Fatalf("the backend would refuse %q", arg)
	}
	for _, arg := range args {
		for _, secret := range []string{"access-secret", "gateway-secret"} {
			if strings.Contains(arg, secret) {
				t.Fatalf("the secret %q appears on the command line as %q", secret, arg)
			}
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

// TestLaunchEnvironmentReplacesRatherThanAppends covers the shell that
// exports DATA_DIR or GATEWAY_URL: the backend reads both as flag defaults, so
// a value the launch did not choose must be replaced, and one the launch does
// not set at all (a switched-off gateway) must not survive from the parent.
func TestLaunchEnvironmentReplacesRatherThanAppends(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"LANG=ru_RU.UTF-8",
		"DATA_DIR=/somebody/elses/pieces",
		"DISK_STORAGE=true",
		"GATEWAY_URL=https://stale.example.com",
		"GATEWAY_USER=stale",
		"GATEWAY_PASS=stale",
		"GATEWAY_CA_CERT=/stale.pem",
	}
	launch := Launch{
		DiskStorage: true,
		DataDir:     "/chosen/pieces",
		GatewayURL:  "https://gateway.example.com",
		GatewayUser: "friend",
		GatewayPass: "secret",
	}

	env := environment(LaunchEnv(parent, launch))

	if env["GATEWAY_URL"] != "https://gateway.example.com" {
		t.Errorf("GATEWAY_URL is %q", env["GATEWAY_URL"])
	}
	if env["GATEWAY_USER"] != "friend" {
		t.Errorf("GATEWAY_USER is %q", env["GATEWAY_USER"])
	}
	if env["GATEWAY_PASS"] != "secret" {
		t.Errorf("GATEWAY_PASS is %q", env["GATEWAY_PASS"])
	}
	if _, ok := env["GATEWAY_CA_CERT"]; ok {
		t.Error("an unset CA certificate was inherited from somewhere else")
	}
	if env["PATH"] != "/usr/bin" || env["LANG"] != "ru_RU.UTF-8" {
		t.Error("unrelated variables were dropped from the child's environment")
	}
	for _, stale := range []string{"DATA_DIR", "DISK_STORAGE"} {
		if value, ok := env[stale]; ok && value != "" {
			t.Errorf("%s reached the child as %q; storage is decided by flags", stale, value)
		}
	}
}

// TestMemoryStorageLeavesNoGatewayBehind is the removal half: with no gateway
// configured, a stale login from the parent must not reach the child, where
// the backend would treat it as configuration.
func TestMemoryStorageLeavesNoGatewayBehind(t *testing.T) {
	parent := []string{
		"DATA_DIR=/somewhere/else",
		"DISK_STORAGE=true",
		"GATEWAY_URL=https://stale.example.com",
		"GATEWAY_USER=stale",
		"GATEWAY_PASS=stale",
		"GATEWAY_CA_CERT=/stale.pem",
	}

	env := environment(LaunchEnv(parent, Launch{}))

	for _, key := range []string{"DISK_STORAGE", "GATEWAY_URL", "GATEWAY_USER", "GATEWAY_PASS", "GATEWAY_CA_CERT"} {
		if _, ok := env[key]; ok {
			t.Errorf("%s reached a child that does not use it", key)
		}
	}
	if value, ok := env["DATA_DIR"]; ok && value != "" {
		t.Errorf("DATA_DIR reached the child as %q", value)
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
	standIn := script(t, `printf '%s|%s|%s' "$DATA_DIR" "$DISK_STORAGE" "$GATEWAY_PASS" > `+out)

	process, err := Start(Launch{
		Executable:  standIn,
		DataDir:     "/pieces",
		DiskStorage: true,
		GatewayURL:  "https://gateway.example.com",
		GatewayUser: "friend",
		GatewayPass: "gateway-secret",
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
	if string(got) != "||gateway-secret" {
		t.Fatalf("the child saw DATA_DIR|DISK_STORAGE|GATEWAY_PASS = %q, want %q",
			string(got), "||gateway-secret")
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
