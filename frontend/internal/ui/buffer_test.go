// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/room"
)

// The window is the only place that knows all three things a buffer report
// needs: which torrent is playing, how large the selected file is, and where
// the player stands in it. The reporter itself has been finished and tested
// for a long time; what was missing was this glue — it was constructed and
// then never asked to do anything (bug-0082).

// TestTheWindowWaitsForMpvToLearnTheFileLength covers the first half of the
// glue: mpv answers nothing useful until it has read the file's header, and a
// zero taken for an answer would aim the prioritiser at nowhere.
func TestTheWindowWaitsForMpvToLearnTheFileLength(t *testing.T) {
	readings := 0
	duration, ok := waitForDuration(context.Background(), func(context.Context) (float64, error) {
		readings++
		if readings < 3 {
			// What mpv says while it is still reading the header.
			return 0, nil
		}
		return 5400, nil
	})

	if !ok {
		t.Fatal("the window gave up before mpv knew how long the file is")
	}
	if duration != 5400 {
		t.Errorf("the file's length came back as %v", duration)
	}
}

// TestTheFileLengthWaitStopsWithItsContext is the other half: switching files
// cancels the wait for the previous file's length. Without that, every file a
// person ever opened would keep a poll of its own alive for the rest of the
// run.
func TestTheFileLengthWaitStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	started := time.Now()
	_, ok := waitForDuration(ctx, func(context.Context) (float64, error) {
		calls++
		return 0, nil
	})

	if ok {
		t.Fatal("the wait reported a length after its context had ended")
	}
	if calls == 0 {
		t.Error("the wait never asked before its context ended")
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("the wait kept running for %v after its context ended", elapsed)
	}
}

// TestTheWindowReportsThePlayheadTheBackendNeeds is the reported offset — the
// whole point of the feature. A 2000-byte file that mpv says is 100 seconds
// long, with playback at 25 seconds, is byte 500, and that is the number the
// backend's download window steers from.
func TestTheWindowReportsThePlayheadTheBackendNeeds(t *testing.T) {
	type report struct {
		torrent string
		offset  int64
	}
	reported := make(chan report, 1)

	w := &window{}
	w.buffer = room.NewBufferReporter(room.ReporterOptions{
		Interval: 5 * time.Millisecond,
		// The report runs on the reporter's own goroutine, so it hands the
		// result over a channel: reading a variable it wrote would be a race
		// the detector is right about.
		Report: func(_ context.Context, torrentID string, offset int64) error {
			select {
			case reported <- report{torrent: torrentID, offset: offset}:
			default:
			}
			return nil
		},
		Position: func() (float64, error) { return 25, nil },
	})
	t.Cleanup(func() {
		w.mu.Lock()
		cancel := w.bufferCancel
		w.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	})

	w.reportBuffer("abc123", 2000, func(context.Context) (float64, error) { return 100, nil })

	select {
	case got := <-reported:
		if got.torrent != "abc123" {
			t.Errorf("the report named torrent %q", got.torrent)
		}
		if got.offset != 500 {
			t.Errorf("the report put the download window at byte %d, wanted 500", got.offset)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no report arrived")
	}
}
