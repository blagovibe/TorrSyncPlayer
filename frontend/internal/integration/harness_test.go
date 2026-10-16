// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package integration runs the real client against the real backend.
//
// This is the gate for retiring the Qt client. Everything else is a unit test
// with a stubbed server; this one starts the actual binary, opens actual rooms,
// and checks that a guest ends up with the same torrent as the host — which is
// the one claim the whole split of the planes rests on.
//
// Nothing here talks to a display: the client code under test is api, room and
// player, all of which run headless. The widgets are deliberately not in the
// way, because what is being checked is behaviour, not appearance.
package integration

import (
	stdbase64 "encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// backend binary and process management.

var (
	buildOnce  sync.Once
	binaryPath string
	buildErr   error
)

// base64Std is the encoding the API expects for a .torrent upload.
var base64Std = stdbase64.StdEncoding

// backendBinary builds the real backend once per test run and returns its path.
func backendBinary(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "torrsync-backend")
		if err != nil {
			buildErr = err
			return
		}
		// go build appends .exe to -o on Windows, and it appends it *silently*:
		// the command succeeds and the binary is at a different name than the one
		// the tests then try to exec, which fails as "executable file not found"
		// on every scenario. Naming it here is cheaper than guessing later.
		name := "torrsyncplayer"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binaryPath = filepath.Join(dir, name)

		cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/server")
		cmd.Dir = filepath.Join(repoRoot(t), "backend")
		if output, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("build the backend: %v\n%s", err, output)
		}
	})

	if buildErr != nil {
		t.Fatalf("%v", buildErr)
	}
	return binaryPath
}

// repoRoot walks up from the test's directory to the repository root.
//
// The marker is the backend's go.mod rather than a directory name: this
// package sits in frontend/internal/integration, and "backend" exists there
// too, so matching on the name alone finds the wrong one and the build fails
// with a confusing path.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}

	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatal("could not find the repository root (no backend/go.mod above the test)")
	return ""
}

// freePort asks the kernel for a port nobody is using.
func freePort(t *testing.T) int {
	t.Helper()

	backend := backendBinary(t)
	_ = backend

	cmd := exec.Command("sh", "-c", "python3 -c \"import socket;s=socket.socket();s.bind(('',0));print(s.getsockname()[1]);s.close()\"")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}

	var port int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &port); err != nil {
		t.Fatalf("free port output was %q", output)
	}
	return port
}

// server is a running backend.
type server struct {
	BaseURL string
	Token   string
	Client  string

	cmd    *exec.Cmd
	logDir string
}

// startServer launches a backend on a free port with its own data directory.
func startServer(t *testing.T, clientID string) *server {
	t.Helper()
	return startServerWith(t, clientID, t.TempDir(), false)
}

// startServerWith launches a backend, optionally with its torrent pieces written
// to disk in dataDir.
//
// A seeder needs disk storage: the pieces have to exist before the client starts,
// because "the host already has the film" is the whole premise of everyone else
// pulling from the swarm. In memory there is nothing to hand out.
func startServerWith(t *testing.T, clientID, dataDir string, disk bool) *server {
	t.Helper()

	token := fmt.Sprintf("token-%d", time.Now().UnixNano())
	port := freePort(t)
	logPath := filepath.Join(t.TempDir(), "backend.log")

	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("backend log: %v", err)
	}

	args := []string{
		"--port", fmt.Sprint(port),
		"--token", token,
		"--data-dir", dataDir,
	}
	if disk {
		args = append(args, "--disk-storage")
	}

	cmd := exec.Command(backendBinary(t), args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		t.Fatalf("start the backend: %v", err)
	}

	srv := &server{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		Token:   token,
		Client:  clientID,
		cmd:     cmd,
		logDir:  logPath,
	}

	t.Cleanup(func() {
		if srv.cmd.Process != nil {
			_ = srv.cmd.Process.Kill()
			_, _ = srv.cmd.Process.Wait()
		}
		_ = logFile.Close()
		if t.Failed() {
			t.Logf("backend log (%s):\n%s", logPath, tailFile(logPath, 40))
		}
	})

	srv.waitReady(t)
	return srv
}

func (s *server) waitReady(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(s.BaseURL + "/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the backend never became ready")
}

// do performs a request against a server as a given client.
func (s *server) do(t *testing.T, method, path string, body any, out any) int {
	t.Helper()

	var reader *strings.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		reader = strings.NewReader(string(encoded))
	} else {
		reader = strings.NewReader("")
	}

	req, err := http.NewRequest(method, s.BaseURL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Access-Token", s.Token)
	req.Header.Set("X-Client-ID", s.Client)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func tailFile(path string, lines int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	all := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}

// encodeTorrent renders a .torrent the way the API takes it.
func encodeTorrent(t *testing.T, raw []byte) string {
	t.Helper()
	return base64Std.EncodeToString(raw)
}
