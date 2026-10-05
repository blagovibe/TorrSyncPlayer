// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package utils

import (
	"sync"
	"time"
)

// Debouncer delays a callback until calls stop coming, then fires it once.
// Used to collapse bursts of state mutations into a single disk write.
type Debouncer struct {
	mu      sync.Mutex
	delay   time.Duration
	fn      func()
	timer   *time.Timer
	stopped bool

	// inFlight is non-nil while a callback is running. It is replaced with a
	// fresh channel before each run and closed once that run returns, so
	// waiters can block on exactly the run they observed.
	inFlight chan struct{}
}

// NewDebouncer creates a Debouncer that fires fn after delay has elapsed
// since the last Trigger call. Returns an initialized debouncer.
func NewDebouncer(delay time.Duration, fn func()) *Debouncer {
	return &Debouncer{
		delay: delay,
		fn:    fn,
	}
}

// Trigger schedules the callback, cancelling any pending one. Safe to call
// repeatedly; only the last call within the window runs the callback.
func (d *Debouncer) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.delay, d.fire)
}

// fire runs the callback in its own goroutine and publishes a completion
// channel for waiters. Running out-of-line keeps callers free to hold their
// own locks: the callback typically re-acquires them for reading.
func (d *Debouncer) fire() {
	d.mu.Lock()
	if d.stopped || d.inFlight != nil {
		// Already stopped, or a callback is still running: this fire is stale.
		d.mu.Unlock()
		return
	}
	done := make(chan struct{})
	d.inFlight = done
	d.mu.Unlock()

	defer func() {
		close(done)
		d.mu.Lock()
		d.inFlight = nil
		d.mu.Unlock()
	}()

	d.fn()
}

// Stop cancels any pending callback and blocks until an in-flight one has
// returned. After Stop, Trigger is a no-op.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.stopped = true
	done := d.inFlight
	d.mu.Unlock()

	if done != nil {
		<-done
	}
}

// Wait blocks until an in-flight callback finishes and reports whether one
// was in flight. Intended for tests that need a deterministic barrier.
func (d *Debouncer) Wait() bool {
	d.mu.Lock()
	done := d.inFlight
	d.mu.Unlock()

	if done == nil {
		return false
	}
	<-done
	return true
}
