// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
)

// The regression this file exists for: tapping a poster of a single-file
// torrent produced "Выберите файл для воспроизведения" and nothing else —
// the one visible affordance that used to be the file list was hidden for
// single-file torrents, so the film could never start. The window now picks
// the obvious file itself and starts playback.
//
// The stand-in mpv is told its pipe and log file through the environment,
// not through --input-ipc-server, which keeps the name byte-identical
// between the window's dialer and the stand-in without either having to
// know the platform's spelling. It appends everything it receives to the
// log until it sees "loadfile" — at which point the assertion has its
// evidence and the process exits cleanly.

const standInLoggingUnix = `//go:build !windows
// +build !windows

package main

import (
	"bytes"
	"io"
	"net"
	"os"
)

func main() {
	sock := os.Getenv("MPV_PIPE")
	log := os.Getenv("MPV_LOG_FILE")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		os.Stderr.WriteString("stand-in listen: " + err.Error() + "\n")
		os.Exit(2)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			os.Exit(3)
		}
		var buf []byte
		b := make([]byte, 64*1024)
		for {
			n, err := c.Read(b)
			if n > 0 {
				buf = append(buf, b[:n]...)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
		}
		_ = c.Close()
		appendAndMaybeExit(log, buf)
	}
}

func appendAndMaybeExit(log string, data []byte) {
	f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(4)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		os.Exit(4)
	}
	_ = f.Close()
	all, err := os.ReadFile(log)
	if err == nil && bytes.Contains(all, []byte("loadfile")) {
		os.Exit(0)
	}
}
`

const standInLoggingWindows = `//go:build windows
// +build windows

package main

import (
	"bytes"
	"os"

	"golang.org/x/sys/windows"
)

func main() {
	pipePath := os.Getenv("MPV_PIPE")
	log := os.Getenv("MPV_LOG_FILE")

	pipe, err := windows.CreateNamedPipe(
		ws(pipePath),
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1,
		0, 0, 0,
		nil,
	)
	if err != nil {
		os.Stderr.WriteString("stand-in create pipe: " + err.Error() + "\n")
		os.Exit(2)
	}
	for {
		if err := windows.ConnectNamedPipe(pipe, nil); err != nil {
			os.Exit(3)
		}
		data := readAll(pipe)
		_ = windows.DisconnectNamedPipe(pipe)
		appendAndMaybeExit(log, data)
	}
}

func readAll(pipe windows.Handle) []byte {
	buf := make([]byte, 64*1024)
	var out []byte
	for {
		var read uint32
		err := windows.ReadFile(pipe, buf, &read, nil)
		if err != nil {
			return out
		}
		if read == 0 {
			return out
		}
		out = append(out, buf[:read]...)
	}
}

func ws(s string) *uint16 {
	ptr, err := windows.UTF16PtrFromString(s)
	if err != nil {
		os.Stderr.WriteString("stand-in UTF16: " + err.Error() + "\n")
		os.Exit(2)
	}
	return ptr
}

func appendAndMaybeExit(log string, data []byte) {
	f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(4)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		os.Exit(4)
	}
	_ = f.Close()
	all, err := os.ReadFile(log)
	if err == nil && bytes.Contains(all, []byte("loadfile")) {
		os.Exit(0)
	}
}
`

func buildLoggingStandInMpv(t *testing.T) string {
	t.Helper()

	source := standInLoggingUnix
	if runtime.GOOS == "windows" {
		source = standInLoggingWindows
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "standin.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "standin")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", bin, path)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the logging stand-in: %v\n%s", err, output)
	}
	return bin
}

func fixtureFor(t *testing.T, filesJSON, torrentsJSON string, onSelect func(int)) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/torrents" && r.Method == http.MethodGet:
			_, _ = io.WriteString(w, torrentsJSON)
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = io.WriteString(w, filesJSON)
		case strings.HasSuffix(r.URL.Path, "/select") && r.Method == http.MethodPost:
			var body struct {
				FileIndex int `json:"fileIndex"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if onSelect != nil {
				onSelect(body.FileIndex)
			}
			_, _ = io.WriteString(w, `{"index":0,"name":"film.mkv"}`)
		case strings.HasSuffix(r.URL.Path, "/buffer/position"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/magnet"):
			_, _ = io.WriteString(w, `{"magnetUri":"magnet:?xt=urn:btih:abc&tr=http://tracker.example/x"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newFixtureWindow(t *testing.T, srv *httptest.Server, mpvBinary, socket, pipe string) *window {
	t.Helper()
	w, _ := newWindow(api.New(srv.URL, "test-token"), "peer-test", nil, WindowOptions{
		Language:     "en",
		OnDiagnostic: func(line string) { t.Logf("[diag] %s", line) },
	})
	w.socketPath = socket
	w.settings.PlayerBinary = mpvBinary
	t.Setenv("MPV_PIPE", pipe)
	return w
}

func socketAndPipe(t *testing.T) (socket, pipe string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		socket = "torrsync-testlog" + fmt.Sprint(time.Now().UnixNano())
		pipe = `\\.\pipe\` + socket
		return socket, pipe
	}
	// Unix sockets keep a short-name limit: the test's own temporary path
	// blows past it on macOS. The socket lives in the system temp dir.
	socket = filepath.Join(os.TempDir(), fmt.Sprintf("tsp-%d.sock", time.Now().UnixNano()))
	return socket, socket
}

func TestTappingAOneFilePosterSelectsAndPlays(t *testing.T) {
	_ = test.NewApp()

	mpvLog := filepath.Join(t.TempDir(), "mpv.log")
	t.Setenv("MPV_LOG_FILE", mpvLog)
	bin := buildLoggingStandInMpv(t)

	selected := make(chan int, 1)
	srv := fixtureFor(t,
		`{"files":[{"index":0,"name":"The.Odyssey.2026.BDRip.720p.x264_New-Team.mkv","size":8643334335}]}`,
		`{"torrents":[{"id":"abc123","name":"The.Odyssey.2026.BDRip.720p.x264_New-Team.mkv","size":8643334335,"progress":0.42,"status":"downloading","peers":3}]}`,
		func(i int) { selected <- i },
	)
	socket, pipe := socketAndPipe(t)
	w := newFixtureWindow(t, srv, bin, socket, pipe)

	w.refresh()
	if len(w.torrents) != 1 {
		t.Fatalf("fixture refresh saw %d torrents, want 1", len(w.torrents))
	}

	// Exactly what a poster tap does.
	w.play(&w.torrents[0])

	select {
	case idx := <-selected:
		if idx != 0 {
			t.Fatalf("selected fileIndex %d, want 0", idx)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("a tap on the poster never selected a file — the film cannot start")
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(mpvLog)
		if err == nil && bytes.Contains(data, []byte("loadfile")) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("mpv was never told to load the film")
}

func TestTappingASeasonPosterWaitsForAChoice(t *testing.T) {
	_ = test.NewApp()

	selected := make(chan int, 1)
	srv := fixtureFor(t,
		`{"files":[`+
			`{"index":0,"name":"Show.S01E01.mkv","size":800000000},`+
			`{"index":1,"name":"Show.S01E02.mkv","size":750000000},`+
			`{"index":2,"name":"Show.S01E03.mkv","size":790000000}`+
			`]}`,
		`{"torrents":[{"id":"season1","name":"Show.S01.Complete.mkv","size":2340000000,"progress":0.1,"status":"downloading","peers":2}]}`,
		func(i int) { selected <- i },
	)
	w := newFixtureWindow(t, srv, "", "", "") // no mpv: the tap must not reach it

	w.refresh()
	if len(w.torrents) != 1 {
		t.Fatalf("fixture refresh saw %d torrents, want 1", len(w.torrents))
	}

	w.play(&w.torrents[0])

	// The tap runs on its own goroutine; wait until the file list lands, the
	// way the person would eventually see it. Shared fields are read under
	// the same lock the window uses.
	deadline := time.Now().Add(10 * time.Second)
	for {
		w.mu.RLock()
		files := len(w.files)
		current := w.current
		w.mu.RUnlock()
		if files == 3 {
			if current == nil || current.ID != "season1" {
				t.Fatal("the season is not the current torrent")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("chips came from %d files, want 3", files)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// And nothing must be selected behind the person's back.
	select {
	case idx := <-selected:
		t.Fatalf("a tap on a season auto-selected fileIndex %d — a person must choose", idx)
	case <-time.After(500 * time.Millisecond):
	}
}
