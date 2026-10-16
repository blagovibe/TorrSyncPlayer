// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package room

import (
	"context"
	"sync"
	"time"
)

// BufferReporter tells the local backend where playback is, twice a second, so
// the piece prioritiser knows which window to fetch next.
//
// This is the difference between "starts playing eventually" and "starts playing
// now". The backend steers priorities from this number; without it the only
// signal is the seek a person just made, and the download window drifts behind
// the film until buffering.
//
// It goes to the DATA plane — the local backend. The room is not told: the room
// cares about play, pause and buffering, not about which bytes anybody is
// fetching.
type BufferReporter struct {
	report func(ctx context.Context, torrentID string, positionBytes int64) error

	interval time.Duration

	mu      sync.Mutex
	playing bool
	// size and duration are needed to turn a position in seconds into one in
	// bytes. Until both are known the reporter stays quiet rather than
	// guessing: a wrong byte offset points the prioritiser at the wrong part
	// of the file, which is worse than no signal at all.
	size     int64
	duration float64
	position func() (float64, error)
}

// ReporterOptions configures a BufferReporter.
type ReporterOptions struct {
	// Report sends the byte position to the backend.
	Report func(ctx context.Context, torrentID string, positionBytes int64) error
	// Position reads the current playback position in seconds. Supplied by the
	// caller so this package never has to know what a player is.
	Position func() (float64, error)
	// Interval between reports. Zero means twice a second.
	Interval time.Duration
}

// NewBufferReporter builds a reporter.
func NewBufferReporter(opts ReporterOptions) *BufferReporter {
	interval := opts.Interval
	if interval == 0 {
		interval = 500 * time.Millisecond
	}
	return &BufferReporter{
		report:   opts.Report,
		position: opts.Position,
		interval: interval,
	}
}

// Track declares what is being played.
//
// A torrent is routinely a season of episodes, so the size that matters is the
// selected file's, not the whole torrent's.
func (b *BufferReporter) Track(torrentID string, fileSize int64, duration float64, playing bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.size = fileSize
	b.duration = duration
	b.playing = playing
}

// BytePosition converts a playback position in seconds into a byte offset.
func BytePosition(positionSeconds, durationSeconds float64, sizeBytes int64) (int64, bool) {
	// Guessing here would aim the prioritiser at the wrong part of the file,
	// so an unknown duration or size means no report at all.
	if durationSeconds <= 0 || sizeBytes <= 0 || positionSeconds < 0 {
		return 0, false
	}
	if positionSeconds > durationSeconds {
		positionSeconds = durationSeconds
	}
	return int64((positionSeconds / durationSeconds) * float64(sizeBytes)), true
}

// Run reports the position until ctx ends.
func (b *BufferReporter) Run(ctx context.Context, torrentID string) {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.reportOnce(ctx, torrentID)
		}
	}
}

func (b *BufferReporter) reportOnce(ctx context.Context, torrentID string) {
	b.mu.Lock()
	size, duration := b.size, b.duration
	reading := b.position
	b.mu.Unlock()

	if reading == nil || torrentID == "" {
		return
	}

	position, err := reading()
	if err != nil {
		return
	}

	offset, ok := BytePosition(position, duration, size)
	if !ok {
		return
	}

	if b.report == nil {
		return
	}
	// A failure here is not worth surfacing: the next tick is 500 ms away and
	// the prioritiser simply uses the previous window.
	_ = b.report(ctx, torrentID, offset)
}
