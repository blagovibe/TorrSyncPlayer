// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

//
// The backend speaks: when something inside it goes wrong, the reason is
// printed to its stdout/stderr. The application this process is attached to
// is a GUI process; on Windows the console the backend writes to is usually
// invisible. A 500 from the API is then "Internal server error" with the
// panic reason riding on a pipe nobody reads.
//
// The fix is on this side of the pipe: Launch carries an OnOutput callback,
// and the stdout/stderr lines the child prints arrive on it. The main
// package wires that callback into the diagnostics file so every launch of
// the application carries its backend's voice, not just its own.

package backend

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestStartPipesChildOutputToTheLaunchCallback writes a small Go program as
// a stand-in backend, builds it, and checks the lines it prints arrive on
// the launch callback. The one thing that has to survive a GUI launch is
// the reason the backend failed; everything else is scaffolding.
func TestStartPipesChildOutputToTheLaunchCallback(t *testing.T) {
	bin := buildStandIn(t, `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("stand-in: stdout line")
	fmt.Fprintln(os.Stderr, "stand-in: stderr line")
}
`)

	var mu sync.Mutex
	var lines []string
	launch := Launch{
		Executable: bin,
		OnOutput: func(line string) {
			mu.Lock()
			lines = append(lines, line)
			mu.Unlock()
		},
	}

	process, err := Start(launch)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-process.Exited():
	case <-timeOut(5):
		t.Fatal("the stand-in backend never finished")
	}
	if err := process.ExitError(); err != nil {
		t.Fatalf("the stand-in failed: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), lines...)
	mu.Unlock()

	assertLines(t, got, []string{
		"stand-in: stdout line",
		"stand-in: stderr line",
	})
}

// TestStartWithoutOnOutputLeavesTheOutputWhereItAlreadyWent is the guard
// against silent regression for command-line users: with no callback, the
// child's output must still reach the process's stderr, the old behaviour.
func TestStartWithoutOnOutputLeavesTheOutputWhereItAlreadyWent(t *testing.T) {
	bin := buildStandIn(t, `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "line for stderr")
}
`)

	launch := Launch{Executable: bin}
	process, err := Start(launch)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-process.Exited():
	case <-timeOut(5):
		t.Fatal("the stand-in backend never finished")
	}
	if err := process.ExitError(); err != nil {
		t.Fatalf("the stand-in failed: %v", err)
	}
}

func buildStandIn(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	source := filepath.Join(dir, "standin.go")
	if err := os.WriteFile(source, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "standin")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", bin, source)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the stand-in: %v\n%s", err, output)
	}
	return bin
}

func timeOut(seconds float64) <-chan time.Time {
	return time.After(time.Duration(seconds * float64(time.Second)))
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %v", len(got), len(want), got)
	}
	for _, need := range want {
		found := false
		for _, line := range got {
			if line == need {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the output lost %q (got %v)", need, got)
		}
	}
}
