// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"

	"fyne.io/fyne/v2/test"
)

// The bug this test exists for: mpv died, and the window memoized its
// corpse. Every call after that went to a named pipe that was never there,
// and the user saw "player: open pipe torrsync-<pid>: The system cannot
// find the file specified." with no reason, forever — because the code
// checked `w.mpv != nil` and called it alive.
//
// The stand-in mpv opens a listener at whatever socket name it was handed
// and serves it until a second connection arrives, then exits non-zero.
// The test proves the window detects the death and relaunches instead of
// reusing the corpse.
func TestDeadPlayerIsRelaunched(t *testing.T) {
	_ = test.NewApp()

	bin := buildStandInMpv(t)

	socketPath := filepath.Join(t.TempDir(), fmt.Sprintf("torrsync-test-%d.sock", time.Now().UnixNano()))
	if runtime.GOOS == "windows" {
		socketPath = "torrsync-test"
	} else {
		socketPath = filepath.Join("/tmp", fmt.Sprintf("torrsync-test-%d.sock", time.Now().UnixNano()))
	}

	client := api.New("http://127.0.0.1:1", "test-token")

	w := &window{
		settings:     Settings{PlayerBinary: bin},
		socketPath:   socketPath,
		onDiagnostic: func(line string) { t.Logf("[diag] %s", line) },
		client:       client,
	}

	p1, err := player.Start(context.Background(), w.mpvOptions(), nil)
	if err != nil {
		t.Fatalf("the first player start: %v", err)
	}
	w.mpv = p1

	// The stand-in is alive until a second connection; making the window
	// ask for its state produces exactly that connection, and the child
	// exits from under the memo.
	_, _ = p1.Paused(context.Background())
	waitForDone(t, p1)

	// This is the line that mattered: the window had a dead player in its
	// slot; without the relaunch it would dial the dead pipe and fail.
	p2, err := w.startPlayer()
	if err != nil {
		t.Fatalf("the window did not relaunch mpv: %v", err)
	}
	if p2 == p1 {
		t.Fatal("the window returned the dead player instead of launching a new one")
	}
	if !p2.Alive() {
		t.Fatal("the relaunched player is not alive")
	}
	p2.Close()
}

const standInUnix = `//go:build !windows
// +build !windows

package main

import (
	"net"
	"os"
)

const (
	argIPCServer = "--input-ipc-server="
	exitOnSignal = 7
)

func main() {
	name := argValue(os.Args, argIPCServer)
	ln, err := net.Listen("unix", name)
	if err != nil {
		os.Stderr.WriteString("stand-in listen: " + err.Error() + "\n")
		os.Exit(2)
	}
	serveUntilSecondConnection(ln, name)
}

func argValue(args []string, prefix string) string {
	for _, arg := range args {
		if len(arg) > len(prefix) && arg[:len(prefix)] == prefix {
			return arg[len(prefix):]
		}
	}
	os.Stderr.WriteString("stand-in: " + prefix + " missing\n")
	os.Exit(2)
	return ""
}

func serveUntilSecondConnection(ln net.Listener, sockPath string) {
	n := 0
	for {
		c, err := ln.Accept()
		if err != nil {
			os.Exit(3)
		}
		n++
		c.Close()
		if n == 1 {
			continue
		}
		// The player's own Close removes the Unix socket file on exit;
		// the stand-in must do the same, or the second launch will not
		// find the slot free.
		_ = os.Remove(sockPath)
		os.Exit(exitOnSignal)
	}
}
`

const standInWindows = `//go:build windows
// +build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

const (
	argIPCServer = "--input-ipc-server="
	exitOnSignal = 7
)

func main() {
	name := argValue(os.Args, argIPCServer)
	pipe, err := windows.CreateNamedPipe(
		ws(` + "`\\\\.\\pipe\\`" + `+name),
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
	servicePipe(pipe)
}

func argValue(args []string, prefix string) string {
	for _, arg := range args {
		if len(arg) > len(prefix) && arg[:len(prefix)] == prefix {
			return arg[len(prefix):]
		}
	}
	os.Stderr.WriteString("stand-in: " + prefix + " missing\n")
	os.Exit(2)
	return ""
}

func ws(s string) *uint16 {
	ptr, err := windows.UTF16PtrFromString(s)
	if err != nil {
		os.Stderr.WriteString("stand-in UTF16: " + err.Error() + "\n")
		os.Exit(2)
	}
	return ptr
}

func servicePipe(pipe windows.Handle) {
	n := 0
	for {
		if err := windows.ConnectNamedPipe(pipe, nil); err != nil {
			os.Exit(3)
		}
		n++
		if err := windows.DisconnectNamedPipe(pipe); err != nil {
			os.Exit(4)
		}
		if n == 1 {
			continue
		}
		os.Exit(exitOnSignal)
	}
}
`

func buildStandInMpv(t *testing.T) string {
	t.Helper()

	source := standInUnix
	if runtime.GOOS == "windows" {
		source = standInWindows
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
		t.Fatalf("build the stand-in: %v\n%s", err, output)
	}
	return bin
}

func waitForDone(t *testing.T, p *player.Player) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("the stand-in never opened its exit door")
		default:
			if !p.Alive() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// Keep identifiers the compiler would otherwise flag for a single unused
// variable type in whichever platform this build is for.
var _ = fmt.Sprintf
