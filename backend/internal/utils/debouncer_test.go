// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package utils

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebouncerCollapsesBurst(t *testing.T) {
	t.Run("many triggers run the callback once", func(t *testing.T) {
		var mu sync.Mutex
		calls := 0
		d := NewDebouncer(20*time.Millisecond, func() {
			mu.Lock()
			calls++
			mu.Unlock()
		})
		defer d.Stop()

		for range 10 {
			d.Trigger()
		}

		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return calls == 1
		}, 2*time.Second, 10*time.Millisecond)

		// No extra runs may appear after the debounce window settles.
		time.Sleep(80 * time.Millisecond)
		mu.Lock()
		assert.Equal(t, 1, calls, "burst must collapse into a single run")
		mu.Unlock()
	})

	t.Run("re-trigger restarts the window", func(t *testing.T) {
		var mu sync.Mutex
		calls := 0
		d := NewDebouncer(60*time.Millisecond, func() {
			mu.Lock()
			calls++
			mu.Unlock()
		})
		defer d.Stop()

		d.Trigger()
		time.Sleep(30 * time.Millisecond)
		d.Trigger() // must push the run out, not queue a second one

		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		assert.Equal(t, 0, calls, "callback must not fire before the window closes")
		mu.Unlock()

		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return calls == 1
		}, 2*time.Second, 10*time.Millisecond)
	})
}

func TestDebouncerStop(t *testing.T) {
	t.Run("cancels a pending callback", func(t *testing.T) {
		var mu sync.Mutex
		calls := 0
		d := NewDebouncer(50*time.Millisecond, func() {
			mu.Lock()
			calls++
			mu.Unlock()
		})

		d.Trigger()
		d.Stop()

		time.Sleep(150 * time.Millisecond)
		mu.Lock()
		assert.Equal(t, 0, calls, "stopped debouncer must not run")
		mu.Unlock()
	})

	t.Run("trigger after stop is a no-op", func(t *testing.T) {
		var mu sync.Mutex
		calls := 0
		d := NewDebouncer(20*time.Millisecond, func() {
			mu.Lock()
			calls++
			mu.Unlock()
		})

		d.Stop()
		d.Trigger()
		time.Sleep(120 * time.Millisecond)

		mu.Lock()
		assert.Equal(t, 0, calls, "trigger after stop must not run")
		mu.Unlock()
	})

	t.Run("waits for an in-flight callback", func(t *testing.T) {
		entered := make(chan struct{}) // closed by the callback on entry
		release := make(chan struct{}) // unblocks the callback
		finished := make(chan struct{})
		d := NewDebouncer(time.Millisecond, func() {
			close(entered)
			<-release
			close(finished)
		})

		d.Trigger()
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
		d := NewDebouncer(time.Hour, func() {})
		defer d.Stop()
		assert.False(t, d.Wait(), "idle debouncer has nothing in flight")
	})
}
