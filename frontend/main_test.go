// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/backend"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/ui"
)

// Startup is where a broken build shows up first, and it used to show up as a
// timeout rather than as a reason.

// TestWaitReturnsAsSoonAsTheBackendIsUp is the ordinary path.
func TestWaitReturnsAsSoonAsTheBackendIsUp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/version" {
			_, _ = w.Write([]byte(`{"version":"test"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	started := time.Now()
	if err := waitForBackend(clientFor(server.URL), processAlive(t), 5*time.Second); err != nil {
		t.Fatalf("a backend that answered was not accepted: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("it took %s to notice a backend that was already up", elapsed)
	}
}

// TestWaitReportsADeadBackendInsteadOfWaiting is the bug, fixed.
//
// The backend exited on startup and the application waited out the entire
// timeout — ten seconds of nothing — and then blamed the network. The reason was
// already printed above it. The fix is to notice the process is gone.
func TestWaitReportsADeadBackendInsteadOfWaiting(t *testing.T) {
	// Nothing is listening here, which is the situation after a backend dies:
	// every attempt fails and would otherwise fail until the clock runs out.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	dead := processDead(t)

	started := time.Now()
	err := waitForBackend(clientFor(server.URL), dead, 30*time.Second)
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("waiting on a dead backend reported success")
	}
	if elapsed > 5*time.Second {
		t.Errorf("it waited %s on a process that had already exited", elapsed)
	}

	message := err.Error()
	if strings.Contains(message, "did not become ready") {
		t.Errorf("a dead backend was reported as a timeout: %q", message)
	}
	if !strings.Contains(message, "stopped") {
		t.Errorf("the error does not say the backend stopped: %q", message)
	}
}

// TestWaitGivesUpOnALiveBackendThatNeverAnswers keeps the timeout honest: a
// backend that is running but not serving is a different failure, and it should
// still be reported as one rather than as a death.
func TestWaitGivesUpOnALiveBackendThatNeverAnswers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not yet", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	err := waitForBackend(clientFor(server.URL), processAlive(t), 700*time.Millisecond)
	if err == nil {
		t.Fatal("waiting on a backend that never answered reported success")
	}
	if !strings.Contains(err.Error(), "did not become ready") {
		t.Errorf("a live backend that never answered was not reported as a timeout: %q", err)
	}
	if strings.Contains(err.Error(), "stopped") {
		t.Errorf("a live backend was reported as dead: %q", err)
	}
}

func clientFor(url string) *api.Client {
	client := api.New(url, "t")
	client.SetClientID("test")
	return client
}

func processAlive(t *testing.T) *backend.Process {
	t.Helper()

	// A process that is running but will never serve: exactly the case the
	// timeout exists for.
	process, err := backend.Start(backend.Launch{
		Executable: stubBackend(t, "trap 'exit 0' TERM\nwhile true; do sleep 0.1; done"),
		Port:       0,
		Token:      "t",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(process.Stop)
	return process
}

func processDead(t *testing.T) *backend.Process {
	t.Helper()

	process, err := backend.Start(backend.Launch{
		Executable: stubBackend(t, "exit 1"),
		Port:       0,
		Token:      "t",
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	<-process.Exited()
	return process
}

// stubBackend writes an executable that accepts the backend's flags and does
// whatever the test needs. Start always passes --port and --data-dir, and the
// token on standard input, so a stand-in that objected to any of them would
// fail for the wrong reason.
func stubBackend(t *testing.T, body string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the stand-in backend is a shell script")
	}

	path := filepath.Join(t.TempDir(), "backend")
	script := "#!/bin/sh\nwhile [ $# -gt 0 ]; do shift; done\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write the stand-in: %v", err)
	}
	return path
}

// The window is not the backend: everything it chose has to cross the launch
// description, or the dialog says one thing and the child does another. The
// mapping is small and silent, which is exactly why it is tested.
func TestTheLaunchCarriesTheStorageChoice(t *testing.T) {
	launch := backendLaunch(
		ui.Settings{StorageMode: ui.StorageDisk, DataDir: "/tmp/pieces"},
		"/opt/backend", 7777, "tok")

	if launch.Executable != "/opt/backend" || launch.Port != 7777 || launch.Token != "tok" {
		t.Errorf("the child was described as %q on port %d", launch.Executable, launch.Port)
	}
	if !launch.DiskStorage || launch.DataDir != "/tmp/pieces" {
		t.Errorf("the storage choice did not reach the child: disk=%v dir=%q",
			launch.DiskStorage, launch.DataDir)
	}
}

// Memory is the absence of disk storage, and the absence has to travel as an
// absence: the backend reads DATA_DIR from the environment as a flag default,
// so a leftover would put the pieces on disk anyway.
func TestTheLaunchKeepsMemoryOffTheDisk(t *testing.T) {
	launch := backendLaunch(ui.Settings{StorageMode: ui.StorageMemory}, "/opt/backend", 1, "t")

	if launch.DiskStorage {
		t.Error("memory storage was described as disk storage")
	}
	if launch.DataDir != "" {
		t.Errorf("memory storage carried a directory: %q", launch.DataDir)
	}
}

func TestTheLaunchCarriesTheGateway(t *testing.T) {
	launch := backendLaunch(ui.Settings{
		GatewayURL:    "https://gateway.example.com",
		GatewayUser:   "friend",
		GatewayPass:   "secret",
		GatewayCACert: "/etc/ca.pem",
	}, "/opt/backend", 1, "t")

	if launch.GatewayURL != "https://gateway.example.com" {
		t.Errorf("the gateway address did not reach the child: %q", launch.GatewayURL)
	}
	if launch.GatewayUser != "friend" || launch.GatewayPass != "secret" {
		t.Error("the gateway login did not reach the child")
	}
	if launch.GatewayCACert != "/etc/ca.pem" {
		t.Errorf("the CA certificate did not reach the child: %q", launch.GatewayCACert)
	}
}

// An empty gateway is off, and off has to travel as empty: a half-configured
// gateway would make the backend proxy every request to nowhere.
func TestAnEmptyGatewayIsOff(t *testing.T) {
	launch := backendLaunch(ui.Settings{}, "/opt/backend", 1, "t")

	if launch.GatewayURL != "" || launch.GatewayUser != "" ||
		launch.GatewayPass != "" || launch.GatewayCACert != "" {
		t.Errorf("an empty form produced a gateway: url=%q user=%q cert=%q",
			launch.GatewayURL, launch.GatewayUser, launch.GatewayCACert)
	}
}
