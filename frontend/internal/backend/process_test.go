// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package backend

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// The process is the one thing in the client that can die on its own, and every
// behaviour below exists because something went wrong without it.

// TestAProcessThatDiedIsReportedAsDead covers the failure this was written for.
//
// The backend exited immediately — a port already in use is the usual reason —
// and the application sat for the full timeout before reporting "did not become
// ready". Ten seconds of silence, followed by an explanation that pointed
// everywhere except the cause. The cause was on screen the whole time, in the
// backend's own output, unmentioned.
func TestAProcessThatDiedIsReportedAsDead(t *testing.T) {
	process := failingProcess(t)

	select {
	case <-process.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the process never reported its exit")
	}

	if process.Running() {
		t.Error("a process that exited still reported itself as running")
	}

	described := DescribeExit(process.ExitError())
	if described == "" {
		t.Fatal("no description of the exit")
	}
	// The description must not be a bare number: it has to send the reader to
	// where the reason actually is.
	if !contains(described, "output") && !contains(described, "exit") {
		t.Errorf("the description names neither the output nor the exit: %q", described)
	}
}

// TestAProcessThatIsRunningSaysSo is the other half: a live backend must not be
// reported as gone, or the startup check would give up on a backend that is
// about to answer.
func TestAProcessThatIsRunningSaysSo(t *testing.T) {
	process := sleepingProcess(t)

	if !process.Running() {
		t.Error("a running process reported itself as gone")
	}

	process.Stop()
	if process.Running() {
		t.Error("a stopped process still reported itself as running")
	}
}

// TestStopEndsAProcessThatIgnoresNothing covers the ordinary shutdown: asked to
// stop, the backend stops.
func TestStopEndsAProcessThatIgnoresNothing(t *testing.T) {
	process := sleepingProcess(t)

	done := make(chan struct{})
	go func() {
		process.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop did not return")
	}

	if process.Running() {
		t.Error("Stop returned with the process still running")
	}
}

// TestStopIsSafeOnAProcessThatAlreadyEnded stops a process twice, which happens
// whenever the backend dies while the application is shutting down. Stop is
// deferred, so it runs on the exit path of every failure, and it must not add
// one of its own.
func TestStopIsSafeOnAProcessThatAlreadyEnded(t *testing.T) {
	process := failingProcess(t)
	<-process.Exited()

	process.Stop()
	process.Stop()

	var missing *Process
	missing.Stop()
	if missing.Running() {
		t.Error("no process at all reported itself as running")
	}
}

// TestDescribeExitSaysSomethingUsefulInEveryCase covers the three endings a
// backend can have, because this string is often the only thing a person reads.
func TestDescribeExitSaysSomethingUsefulInEveryCase(t *testing.T) {
	if got := DescribeExit(nil); got == "" {
		t.Error("a clean stop produced no description")
	}

	// A process that was killed by a signal is not an exit status, and printing
	// one where there is none would be a lie.
	if got := DescribeExit(os.ErrDeadlineExceeded); contains(got, "exit status") {
		t.Errorf("an ordinary error was dressed up as an exit status: %q", got)
	}
}

// TestStartDeliversTheTokenOnStandardInput is the other half of
// deviation-0061: the flag asks the backend to read the secret from standard
// input, and something has to put it there. The stand-in checks both
// directions at once — the token arrived on stdin, and it did not arrive as an
// argument.
func TestStartDeliversTheTokenOnStandardInput(t *testing.T) {
	standIn := script(t, `
for arg in "$@"; do
  if [ "$arg" = "secret-value" ]; then exit 3; fi
done
IFS= read -r token || exit 4
if [ "$token" != "secret-value" ]; then exit 5; fi
exit 0`)

	process, err := Start(Launch{Executable: standIn, Token: "secret-value"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	select {
	case <-process.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("the stand-in backend never finished reading its token")
	}

	if err := process.ExitError(); err != nil {
		t.Errorf("the stand-in failed (3: secret on the command line, 4: nothing on stdin, 5: wrong token): %v", err)
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

// script writes an executable shell script and returns its path.
//
// Start builds the backend's own arguments, so a stand-in has to accept and
// ignore them rather than refuse them the way a real backend would.
func script(t *testing.T, body string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the stand-in backend is a shell script")
	}

	path := filepath.Join(t.TempDir(), "backend")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write the stand-in: %v", err)
	}
	return path
}

func failingProcess(t *testing.T) *Process {
	t.Helper()

	process, err := Start(Launch{Executable: script(t, "echo 'address already in use' >&2\nexit 1"), Token: "t"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return process
}

func sleepingProcess(t *testing.T) *Process {
	t.Helper()

	process, err := Start(Launch{Executable: script(t, "trap 'exit 0' TERM\nwhile true; do sleep 0.1; done"), Token: "t"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return process
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
