// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package room

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The byte offset is what the piece prioritiser steers from. Getting it wrong
// does not degrade gracefully — it aims the download window at the wrong part
// of the file, which is worse than not reporting at all.
func TestBytePositionMapsSecondsIntoTheFile(t *testing.T) {
	offset, ok := BytePosition(50, 100, 1000)
	require.True(t, ok)
	assert.Equal(t, int64(500), offset)
}

func TestBytePositionClampsPastTheEnd(t *testing.T) {
	// A seek past the end, or a duration that drifts, must not produce an
	// offset beyond the file.
	offset, ok := BytePosition(120, 100, 1000)
	require.True(t, ok)
	assert.Equal(t, int64(1000), offset)
}

func TestBytePositionRefusesToGuess(t *testing.T) {
	// Before the file's size and duration are known, any number would be a
	// guess, and a guess here points the prioritiser at the wrong window.
	for _, tc := range []struct {
		name               string
		position, duration float64
		size               int64
	}{
		{"duration is zero", 10, 0, 1000},
		{"duration is negative", 10, -5, 1000},
		{"size is zero", 10, 100, 0},
		{"position is negative", -1, 100, 1000},
	} {
		if _, ok := BytePosition(tc.position, tc.duration, tc.size); ok {
			t.Errorf("%s: a position was invented from nothing", tc.name)
		}
	}
}

func TestReporterSendsTheWindowThePrioritiserNeeds(t *testing.T) {
	var got struct {
		torrent string
		offset  int64
	}

	reporter := NewBufferReporter(ReporterOptions{
		Interval: 10 * time.Millisecond,
		Report: func(_ context.Context, torrentID string, offset int64) error {
			got.torrent, got.offset = torrentID, offset
			return nil
		},
		Position: func() (float64, error) { return 25, nil },
	})
	reporter.Track("abc123", 2000, 100, true)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	reporter.Run(ctx, "abc123")

	assert.Equal(t, "abc123", got.torrent)
	assert.Equal(t, int64(500), got.offset)
}

func TestReporterStaysQuietUntilTheFileIsKnown(t *testing.T) {
	// Reporting before the size is known is how the prioritiser ends up aiming
	// at the start of the file no matter where playback is.
	calls := 0
	reporter := NewBufferReporter(ReporterOptions{
		Interval: 5 * time.Millisecond,
		Report:   func(context.Context, string, int64) error { calls++; return nil },
		Position: func() (float64, error) { return 25, nil },
	})
	// Track was never called: nothing is playing yet.

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	reporter.Run(ctx, "abc123")

	assert.Zero(t, calls, "the reporter spoke about a file it knows nothing about")
}

func TestReporterSurvivesAReadingFailure(t *testing.T) {
	// A player that cannot be asked must not stop the ticker: the next reading
	// 500 ms later is the one that matters.
	calls := 0
	readings := 0

	reporter := NewBufferReporter(ReporterOptions{
		Interval: 5 * time.Millisecond,
		Report:   func(context.Context, string, int64) error { calls++; return nil },
		Position: func() (float64, error) {
			readings++
			if readings <= 3 {
				return 0, assert.AnError
			}
			return 25, nil
		},
	})
	reporter.Track("abc123", 2000, 100, true)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	reporter.Run(ctx, "abc123")

	assert.Positive(t, calls, "the reporter never recovered from a failed reading")
}

func TestReporterStopsWithItsContext(t *testing.T) {
	// A reporter that outlives the playback session keeps hammering the
	// backend for a file nobody is watching.
	calls := make(chan struct{}, 16)
	reporter := NewBufferReporter(ReporterOptions{
		Interval: 5 * time.Millisecond,
		Report:   func(context.Context, string, int64) error { calls <- struct{}{}; return nil },
		Position: func() (float64, error) { return 25, nil },
	})
	reporter.Track("abc123", 2000, 100, true)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { reporter.Run(ctx, "abc123"); close(done) }()

	<-calls
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the reporter kept running after its context ended")
	}
}
