// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The room's stop rests on two facts about mpv, and both of them are easy to
// lose in an mpv upgrade.
//
// The first: mpv keeps saying "stalled" while the player is paused. The room
// stops for the one who is behind, and the host stops its own player to do
// it — so if the pause hid the stall, the host would report a recovery it
// did not have, the room would let go, and everyone would race ahead of the
// participant that is still waiting for data.
//
// The second: the stall clears when the data arrives, even while the player
// is paused. Without it the room would wait for a recovery that never comes,
// and the pause would be forever.
//
// The source serves a third of a real film and then holds the rest, which is
// exactly a torrent with only its head downloaded.

// TestMpvKeepsSayingStalledWhileThePlayerIsPaused pins both facts.
func TestMpvKeepsSayingStalledWhileThePlayerIsPaused(t *testing.T) {
	if !mpvAvailable() {
		t.Skip("mpv is not installed")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH, so there is no file to play")
	}

	media := buildPlayableFile(t, ffmpeg)
	src := newStallingSource(t, media, int(fileSize(t, media)/3))

	socket := testSocketPath(t, "stalled")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	p, err := Start(ctx, Options{
		Binary:     "mpv",
		SocketPath: socket,
		ExtraArgs:  []string{"--vo=null", "--ao=null"},
	}, nil)
	require.NoError(t, err, "start mpv")
	defer p.Close()

	require.NoError(t, p.Play(ctx, src.URL()), "play the film")

	// The film runs out of data and the player says so.
	waitForStalled(t, ctx, p)

	// The room stops: the player is paused. The pause is fire-and-forget, so
	// the property changes a moment after the command is written.
	require.NoError(t, p.Pause(ctx))
	waitForPaused(t, ctx, p, true)

	time.Sleep(time.Second)
	stalled, err := p.Stalled(ctx)
	require.NoError(t, err)
	require.True(t, stalled,
		"mpv must keep saying stalled while the player is paused: otherwise stopping the room hides the stall that stopped it")

	// And the data arrives: the stall clears even though the player is still
	// paused, so the room can be let go.
	src.SendTheRest()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		stalled, err = p.Stalled(ctx)
		require.NoError(t, err)
		if !stalled {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.False(t, stalled,
		"the stall must clear when the data arrives, even while the player is paused: otherwise the room waits for a recovery that never comes")
}

// waitForPaused waits until the player says whether it is paused.
func waitForPaused(t *testing.T, ctx context.Context, p *Player, want bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		paused, err := p.Paused(ctx)
		if err == nil && paused == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("the player did not report paused=%v", want)
}

// waitForStalled waits until the player says it is out of data.
func waitForStalled(t *testing.T, ctx context.Context, p *Player) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		stalled, err := p.Stalled(ctx)
		if err == nil && stalled {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the player never said it was out of data")
}
