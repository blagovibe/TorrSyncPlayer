// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"errors"
	"testing"
	"time"
)

// errBusy stands in for ERROR_PIPE_BUSY here; the real one is a Windows errno
// that this file cannot name, and the decision the helper makes does not depend
// on which error it is.
var errBusy = errors.New("busy")

// TestRetryWhileBusyWaitsOutAServerThatIsStillHandingBackTheLastOne is the
// whole reason the helper exists, and it is the Windows job's red: mpv's pipe
// takes one instance at a time, the window asks it several times a second, and
// the pipe was still busy.
func TestRetryWhileBusyWaitsOutAServerThatIsStillHandingBackTheLastOne(t *testing.T) {
	var tries int
	err := retryWhileBusy(func() error {
		tries++
		if tries < 3 {
			return errBusy
		}
		return nil
	}, func(err error) bool { return errors.Is(err, errBusy) }, time.Second, time.Millisecond)

	if err != nil {
		t.Fatalf("a pipe that came free on the third try was reported as %v", err)
	}
	if tries != 3 {
		t.Errorf("asked %d times, and the point is that it asks again", tries)
	}
}

// TestRetryWhileBusyReturnsAFailureThatIsNotBusy pins the other half: only a
// busy server is worth waiting for. A missing pipe, a refused name or a dead
// player must come back at once — retried, it becomes a slow failure that says
// less than the first one.
func TestRetryWhileBusyReturnsAFailureThatIsNotBusy(t *testing.T) {
	dead := errors.New("no such pipe")
	var tries int
	err := retryWhileBusy(func() error {
		tries++
		return dead
	}, func(err error) bool { return errors.Is(err, errBusy) }, time.Minute, time.Millisecond)

	if !errors.Is(err, dead) {
		t.Fatalf("got %v, want the failure the server gave", err)
	}
	if tries != 1 {
		t.Errorf("a failure that was never going to pass was retried %d times", tries-1)
	}
}

// TestRetryWhileBusyGivesUp guards the other end: a server that stays busy
// forever must not keep the caller waiting forever. The window's poll has a
// context of its own, but a poll that never returns is a window that never
// repaints.
func TestRetryWhileBusyGivesUp(t *testing.T) {
	started := time.Now()
	var tries int
	err := retryWhileBusy(func() error {
		tries++
		return errBusy
	}, func(err error) bool { return errors.Is(err, errBusy) }, 20*time.Millisecond, 5*time.Millisecond)

	if !errors.Is(err, errBusy) {
		t.Fatalf("got %v, want the busyness it never recovered from", err)
	}
	if tries < 2 {
		t.Errorf("gave up after %d tries without waiting at all", tries)
	}
	if took := time.Since(started); took > 2*time.Second {
		t.Errorf("waited %s, which is not giving up", took)
	}
}

// TestRetryWhileBusyDoesNotWaitWhenItSucceeded guards the shape of the loop: a
// first try that works must not sleep even once, or every command in the window
// would carry the step's delay.
func TestRetryWhileBusyDoesNotWaitWhenItSucceeded(t *testing.T) {
	started := time.Now()
	if err := retryWhileBusy(func() error { return nil },
		func(error) bool { return true }, time.Minute, 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(started); took > 10*time.Millisecond {
		t.Errorf("waited %s for a pipe that was free", took)
	}
}
