// SPDX-License-Identifier: MIT
package buffer

import (
	"context"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
)

// Exact priority maps. The loose assertions (a piece here, a piece there)
// let every band boundary move silently; only the whole map pins the
// now → high → normal → readahead → none cascade with its range clamps.

func prioritiesOf(f *mockFile) []torrent.PiecePriority {
	out := make([]torrent.PiecePriority, len(f.torrent.pieces))
	for i, p := range f.torrent.pieces {
		out[i] = p.priority
	}
	return out
}

func repeat(t *testing.T, prio torrent.PiecePriority, n int) []torrent.PiecePriority {
	t.Helper()
	out := make([]torrent.PiecePriority, n)
	for i := range out {
		out[i] = prio
	}
	return out
}

// TestPriorityMapAllBands drives every band at once: with an 80% buffer the
// window spans 80 pieces, past the high band (50) into normal, and the
// readahead tail runs past the window to the end of the file.
func TestPriorityMapAllBands(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	f := newMockFile(1024*1024, 100*1024*1024) // 100 pieces
	registerTestFile(s, "tor-1", f)
	s.mu.Lock()
	s.torrentBuffers["tor-1"].BufferPercent = 80
	s.mu.Unlock()

	require.NoError(t, s.UpdatePosition(context.Background(), "tor-1", 0))

	var want []torrent.PiecePriority
	want = append(want, repeat(t, torrent.PiecePriorityNow, 11)...)       // 0-10
	want = append(want, repeat(t, torrent.PiecePriorityHigh, 40)...)      // 11-50
	want = append(want, repeat(t, torrent.PiecePriorityNormal, 30)...)    // 51-80
	want = append(want, repeat(t, torrent.PiecePriorityReadahead, 19)...) // 81-99
	assert.Equal(t, want, prioritiesOf(f))
}

// TestPriorityMapMidFile pins both outsides: pieces before the window and
// past the readahead tail go None, and the low edge (piece 59) proves the
// scan starts at startPiece-1 rather than at zero or at the window.
func TestPriorityMapMidFile(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	f := newMockFile(1024*1024, 100*1024*1024) // 100 pieces, 10% buffer
	registerTestFile(s, "tor-1", f)

	require.NoError(t, s.UpdatePosition(context.Background(), "tor-1", 60*1024*1024))

	var want []torrent.PiecePriority
	want = append(want, repeat(t, torrent.PiecePriorityNone, 60)...)      // 0-59
	want = append(want, repeat(t, torrent.PiecePriorityNow, 11)...)       // 60-70
	want = append(want, repeat(t, torrent.PiecePriorityReadahead, 20)...) // 71-90
	want = append(want, repeat(t, torrent.PiecePriorityNone, 9)...)       // 91-99
	assert.Equal(t, want, prioritiesOf(f))
}

// TestZeroBufferWindow covers the guarded division: with a zero window the
// percent stays 0 rather than NaN, and the room treats it as buffering.
func TestZeroBufferWindow(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	f := newMockFile(1024*1024, 10*1024*1024)
	registerTestFile(s, "tor-1", f)
	s.mu.Lock()
	s.torrentBuffers["tor-1"].BufferPercent = 0
	s.mu.Unlock()

	require.NoError(t, s.UpdatePosition(context.Background(), "tor-1", 0))

	info, err := s.GetBufferInfo(context.Background(), "tor-1")
	require.NoError(t, err)
	assert.Equal(t, 0.0, info.BufferedPercent)
	assert.True(t, info.IsBuffering)
}

// TestCloseForgetsTorrents pins that Close is a reset, not just a stop: a
// closed service answers NotFound, it does not serve stale windows.
func TestCloseForgetsTorrents(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	f := newMockFile(1024*1024, 10*1024*1024)
	registerTestFile(s, "tor-1", f)
	s.Close()

	_, err := s.GetBufferInfo(context.Background(), "tor-1")
	require.Error(t, err)
	err = s.UpdatePosition(context.Background(), "tor-1", 0)
	require.Error(t, err)
}

// TestStopWithoutStart and restart pin the lifecycle edges: stopping a fresh
// service returns instead of waiting forever, and starting twice restarts
// rather than leaking the first loop.
func TestStopWithoutStart(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)
	s.StopPeriodicUpdate()
	assert.Nil(t, s.cancelFunc)
}

func TestStartTwiceRestarts(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	f := newMockFile(1024*1024, 10*1024*1024)
	registerTestFile(s, "tor-1", f)

	s.StartPeriodicUpdate(10 * time.Millisecond)
	s.StartPeriodicUpdate(10 * time.Millisecond)
	s.StopPeriodicUpdate()
	assert.Nil(t, s.cancelFunc)

	// And the service still starts afterwards: a restart must not wedge it.
	s.StartPeriodicUpdate(10 * time.Millisecond)
	assert.NotNil(t, s.cancelFunc)
	s.StopPeriodicUpdate()
	assert.Nil(t, s.cancelFunc)
}

// TestUnregisterThenUse pins that removal is visible: the window is gone,
// not lingering.
func TestUnregisterThenUse(t *testing.T) {
	s := NewService(constants.DefaultMaxBufferSize)

	f := newMockFile(1024*1024, 10*1024*1024)
	registerTestFile(s, "tor-1", f)
	s.UnregisterTorrent("tor-1")

	_, err := s.GetBufferInfo(context.Background(), "tor-1")
	require.Error(t, err)
	require.Error(t, s.UpdatePosition(context.Background(), "tor-1", 0))
}
