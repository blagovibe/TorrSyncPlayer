// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package utils

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeClock is a controllable clock for deterministic debouncer tests.
type fakeClock struct {
	mu           sync.Mutex
	now          time.Time
	timers       map[*fakeTimer]struct{}
	advanceC     chan struct{}
	inFlight     int
	inFlightDone chan struct{}
}

type fakeTimer struct {
	clock *fakeClock
	d     time.Duration
	f     func()
	c     chan time.Time
	done  bool
}

func newFakeClock() *fakeClock {
	return &fakeClock{
		now:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		timers:       make(map[*fakeTimer]struct{}),
		advanceC:     make(chan struct{}, 1),
		inFlightDone: make(chan struct{}),
	}
}

func (fc *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	ft := &fakeTimer{
		clock: fc,
		d:     d,
		f:     f,
		c:     make(chan time.Time, 1),
	}
	fc.mu.Lock()
	fc.timers[ft] = struct{}{}
	fc.mu.Unlock()
	return ft
}

func (fc *fakeClock) NewTimer(d time.Duration) Timer {
	return fc.AfterFunc(d, func() {})
}

func (ft *fakeTimer) Stop() bool {
	ft.clock.mu.Lock()
	defer ft.clock.mu.Unlock()
	if ft.done {
		return false
	}
	delete(ft.clock.timers, ft)
	ft.done = true
	return true
}

func (ft *fakeTimer) C() <-chan time.Time {
	return ft.c
}

// Advance moves fake time forward by d, firing any timers that expire.
// Callbacks are run in separate goroutines to match real time.AfterFunc behavior.
func (fc *fakeClock) Advance(d time.Duration) {
	fc.mu.Lock()
	fc.now = fc.now.Add(d)
	// Collect expired timers
	var expired []*fakeTimer
	for t := range fc.timers {
		if t.d <= d {
			expired = append(expired, t)
		}
	}
	// Reduce remaining timers
	for t := range fc.timers {
		t.d -= d
	}
	// Fire expired outside the lock
	for _, t := range expired {
		delete(fc.timers, t)
		t.done = true
		close(t.c)
	}
	fc.mu.Unlock()

	// Call callbacks in separate goroutines (like real time.AfterFunc)
	for _, t := range expired {
		fc.mu.Lock()
		fc.inFlight++
		fc.mu.Unlock()
		go func(t *fakeTimer) {
			t.f()
			fc.mu.Lock()
			fc.inFlight--
			if fc.inFlight == 0 {
				close(fc.inFlightDone)
				fc.inFlightDone = make(chan struct{})
			}
			fc.mu.Unlock()
		}(t)
	}

	// Signal any waiters
	select {
	case fc.advanceC <- struct{}{}:
	default:
	}
}

// WaitForCallbacks blocks until all callbacks fired by the last Advance have completed.
func (fc *fakeClock) WaitForCallbacks(timeout time.Duration) bool {
	fc.mu.Lock()
	done := fc.inFlightDone
	inFlight := fc.inFlight
	fc.mu.Unlock()

	if inFlight == 0 {
		return true
	}
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestDebouncerCollapsesBurst(t *testing.T) {
	t.Run("many triggers run the callback once", func(t *testing.T) {
		clock := newFakeClock()
		var mu sync.Mutex
		calls := 0
		d := NewDebouncerWithClock(20*time.Millisecond, func() {
			mu.Lock()
			calls++
			mu.Unlock()
		}, clock)
		defer d.Stop()

		for range 10 {
			d.Trigger()
		}

		clock.Advance(30 * time.Millisecond)
		clock.WaitForCallbacks(100 * time.Millisecond)

		mu.Lock()
		assert.Equal(t, 1, calls, "burst must collapse into a single run")
		mu.Unlock()

		// Further advance must not cause extra runs
		clock.Advance(100 * time.Millisecond)
		clock.WaitForCallbacks(100 * time.Millisecond)
		mu.Lock()
		assert.Equal(t, 1, calls, "no extra runs after window settles")
		mu.Unlock()
	})

	t.Run("re-trigger restarts the window", func(t *testing.T) {
		clock := newFakeClock()
		var mu sync.Mutex
		calls := 0
		d := NewDebouncerWithClock(60*time.Millisecond, func() {
			mu.Lock()
			calls++
			mu.Unlock()
		}, clock)
		defer d.Stop()

		d.Trigger()
		clock.Advance(30 * time.Millisecond)
		d.Trigger() // must push the run out, not queue a second one

		clock.Advance(20 * time.Millisecond)
		clock.WaitForCallbacks(100 * time.Millisecond)
		mu.Lock()
		assert.Equal(t, 0, calls, "callback must not fire before the window closes")
		mu.Unlock()

		clock.Advance(50 * time.Millisecond)
		clock.WaitForCallbacks(100 * time.Millisecond)
		mu.Lock()
		assert.Equal(t, 1, calls, "callback fires after full window from last trigger")
		mu.Unlock()
	})
}

func TestDebouncerStop(t *testing.T) {
	t.Run("cancels a pending callback", func(t *testing.T) {
		clock := newFakeClock()
		var mu sync.Mutex
		calls := 0
		d := NewDebouncerWithClock(50*time.Millisecond, func() {
			mu.Lock()
			calls++
			mu.Unlock()
		}, clock)

		d.Trigger()
		d.Stop()

		clock.Advance(150 * time.Millisecond)
		clock.WaitForCallbacks(100 * time.Millisecond)
		mu.Lock()
		assert.Equal(t, 0, calls, "stopped debouncer must not run")
		mu.Unlock()
	})

	t.Run("trigger after stop is a no-op", func(t *testing.T) {
		clock := newFakeClock()
		var mu sync.Mutex
		calls := 0
		d := NewDebouncerWithClock(20*time.Millisecond, func() {
			mu.Lock()
			calls++
			mu.Unlock()
		}, clock)

		d.Stop()
		d.Trigger()
		clock.Advance(120 * time.Millisecond)
		clock.WaitForCallbacks(100 * time.Millisecond)

		mu.Lock()
		assert.Equal(t, 0, calls, "trigger after stop must not run")
		mu.Unlock()
	})

	t.Run("waits for an in-flight callback", func(t *testing.T) {
		clock := newFakeClock()
		entered := make(chan struct{})
		release := make(chan struct{})
		finished := make(chan struct{})
		d := NewDebouncerWithClock(time.Millisecond, func() {
			close(entered)
			<-release
			close(finished)
		}, clock)

		d.Trigger()
		clock.Advance(10 * time.Millisecond)
		<-entered // callback is now running and blocked on release

		stopped := make(chan struct{})
		go func() {
			d.Stop()
			close(stopped)
		}()

		select {
		case <-stopped:
			t.Fatal("Stop returned while the callback was still running")
		case <-time.After(50 * time.Millisecond):
		}

		close(release)
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
			t.Fatal("Stop did not return after the callback finished")
		}
		<-finished
	})
}

func TestDebouncerWait(t *testing.T) {
	t.Run("reports false when idle", func(t *testing.T) {
		clock := newFakeClock()
		d := NewDebouncerWithClock(time.Hour, func() {}, clock)
		defer d.Stop()
		assert.False(t, d.Wait(), "idle debouncer has nothing in flight")
	})

	t.Run("reports true and blocks until callback finishes", func(t *testing.T) {
		clock := newFakeClock()
		entered := make(chan struct{})
		release := make(chan struct{})
		finished := make(chan struct{})
		d := NewDebouncerWithClock(time.Millisecond, func() {
			close(entered)
			<-release
			close(finished)
		}, clock)
		defer d.Stop()

		d.Trigger()
		clock.Advance(10 * time.Millisecond)
		<-entered // callback is now running and blocked on release

		// Now Wait() should see in-flight callback and block
		go func() {
			close(release)
			clock.WaitForCallbacks(100 * time.Millisecond)
		}()
		assert.True(t, d.Wait(), "Wait should return true when callback was in flight")
		<-finished
	})
}
