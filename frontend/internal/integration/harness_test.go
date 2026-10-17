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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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

// freePort asks the kernel for a port nobody is listening on.
//
// The backend serves HTTP on this number, and HTTP is TCP. The torrent client
// inside the backend listens on UDP with a number of its own, chosen there and
// checked on both protocols (bug-0118) — so this is the only port the test has
// to find.
//
// It used to shell out to python3 to ask the same question, which made the
// suite depend on python being installed on the runner; a runner without it
// failed with "find a free port", which says nothing about what is wrong.
func freePort(t *testing.T) int {
	t.Helper()

	// Asking for the binary here keeps its one-time build in one place: the
	// port is useless without a backend to listen on it.
	_ = backendBinary(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address %T", listener.Addr())
	}
	return addr.Port
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
			log := tailFile(logPath, 40)
			t.Logf("backend log (%s):\n%s", logPath, log)
			t.Logf("what this process can do with the ports in play:\n%s", portVerdict(t, log, port))
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

// TestAFreePortIsFreeForBothListeners says what the backend is about to be told,
// on the platforms that differ. A port free for TCP is the old promise and it is
// not enough: the same number has to be bindable for UDP too, or the torrent
// client inside the backend refuses to start and the test waits for a server
// that will never answer (bug-0118).
func TestAFreePortIsFreeForBothListeners(t *testing.T) {
	port := freePort(t)

	tcp, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("the port %d was handed out and cannot be bound for TCP: %v", port, err)
	}
	defer func() { _ = tcp.Close() }()

	udp, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("the port %d was handed out and cannot be bound for UDP: %v", port, err)
	}
	_ = udp.Close()
}

// portVerdict says what this process can do with the port the backend was given,
// and what the machine says about it.
//
// The backend died on `listen udp4 :P: bind: ... access permissions` on Windows,
// and the two explanations call for opposite fixes: if this process cannot bind
// it either, the machine is refusing the number and the answer is a different
// one; if this process binds it happily, the refusal is something the backend
// itself does with the socket, and choosing another port would change nothing.
// A red job that says which one is worth more than another guess (bug-0118).
func portVerdict(t *testing.T, log string, httpPort int) string {
	t.Helper()

	var out strings.Builder

	// The number the backend actually refused is in its own log; the port the
	// test handed out is a different one, and it is the refused number that
	// matters.
	refused := refusedPort(log)
	fmt.Fprintf(&out, "  HTTP port given to the backend: %d\n", httpPort)
	fmt.Fprintf(&out, "  port the backend refused: %d\n", refused)

	port := httpPort
	if refused != 0 {
		port = refused
	}
	tcp, terr := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if terr != nil {
		fmt.Fprintf(&out, "  TCP bind on %d: %v\n", port, terr)
	} else {
		_ = tcp.Close()
		fmt.Fprintf(&out, "  TCP bind on %d: fine\n", port)
	}
	udp, uerr := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if uerr != nil {
		fmt.Fprintf(&out, "  UDP bind on %d: %v\n", port, uerr)
	} else {
		_ = udp.Close()
		fmt.Fprintf(&out, "  UDP bind on %d: fine\n", port)
	}

	if runtime.GOOS == "windows" {
		// Windows keeps ranges of ports for itself, and says so here. A port
		// inside one cannot be bound at all, which is the answer when the two
		// binds above disagree with what the backend met.
		netsh := exec.Command("netsh", "interface", "ipv4", "show", "excludedportrange", "protocol=udp")
		if text, err := netsh.CombinedOutput(); err == nil {
			if rows := reservedUDPRanges(string(text)); len(rows) > 0 {
				fmt.Fprintf(&out, "  UDP reserved ranges: %s\n", describeRanges(rows))
				if refused != 0 {
					fmt.Fprintf(&out, "  refused port %d is %s\n", refused, inRanges(refused, rows))
				}
			}
		}
	}
	return out.String()
}

// describeRanges prints the ranges the way netsh does, so the line in a CI log
// can be read next to the same command's output.
func describeRanges(ranges [][2]int) string {
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		parts = append(parts, fmt.Sprintf("%d-%d", r[0], r[1]))
	}
	return strings.Join(parts, ", ")
}

// reservedUDPRanges reads the pairs of numbers netsh prints, one "first last"
// row per range.
func reservedUDPRanges(netshOutput string) [][2]int {
	var ranges [][2]int
	for _, line := range strings.Split(netshOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		first, err1 := strconv.Atoi(fields[0])
		last, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil || first <= 0 || last < first {
			continue
		}
		ranges = append(ranges, [2]int{first, last})
	}
	return ranges
}

func inRanges(port int, ranges [][2]int) string {
	for _, r := range ranges {
		if port >= r[0] && port <= r[1] {
			return "inside a reserved range"
		}
	}
	return "not inside any reserved range"
}

// refusedPort reads the number the backend was refused on out of its log.
//
// The log says it in one line: "subsequent listen: listen udp4 :63515: bind:
// ...". The number is what decides the question — whether the machine refused
// it or the backend did something with the socket — and the log is where the
// number is.
func refusedPort(log string) int {
	for _, line := range strings.Split(log, "\n") {
		idx := strings.Index(line, "listen udp4 :")
		if idx < 0 {
			continue
		}
		rest := line[idx+len("listen udp4 :"):]
		end := strings.IndexAny(rest, " :")
		if end < 0 {
			continue
		}
		if port, err := strconv.Atoi(rest[:end]); err == nil {
			return port
		}
	}
	return 0
}
