// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
)

// The last untested seam in "watching together": the room's callbacks reach
// the window, and the window reaches the player. The room layer is proven
// over a real network (room_network_test.go in the rtc package), the player
// layer has its own tests against a real mpv, and nothing in between was ever
// exercised — so a callback wired to the wrong method, or a raw target handed
// to the player where a smoothed one belongs, would pass every test there is.
//
// What a follower must do: stop when the host stops, and move toward the
// host's position without teleporting past everyone else.

func mpvOnPath(t *testing.T) bool {
	t.Helper()
	_, err := exec.LookPath("mpv")
	return err == nil
}

// followerMedia builds a long enough file for a seek to land inside: the
// smoothed target of a jump from zero to a hundred seconds is the thirtieth
// second, and a two-second film would clamp the seek to its end and hide the
// very thing the test measures.
func followerMedia(t *testing.T) string {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH, so there is no file to play")
	}
	media := filepath.Join(t.TempDir(), "film.mp4")
	cmd := exec.Command(ffmpeg,
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=5:duration=120",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-y", media,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not build a file to play: %v\n%s", err, out)
	}
	return media
}

// startFollowerPlayer starts a real mpv with the file loaded.
//
// The label keeps two players of the same run apart: the socket path is
// shared per process, and one player would take it from under the other.
func startFollowerPlayer(t *testing.T, label, media string) *player.Player {
	t.Helper()

	// The socket path is short on purpose: macOS caps unix socket paths at
	// 104 bytes, and a test's own temporary directory can use most of it.
	socket := filepath.Join("/tmp", fmt.Sprintf("torrsync-follow-%s-%d.sock", label, os.Getpid()))
	t.Cleanup(func() { _ = os.Remove(socket) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, err := player.Start(ctx, player.Options{
		Binary:     "mpv",
		SocketPath: socket,
		ExtraArgs:  []string{"--vo=null", "--ao=null"},
	}, nil)
	if err != nil {
		t.Fatalf("start mpv: %v", err)
	}
	t.Cleanup(p.Close)

	if err := p.Play(ctx, media); err != nil {
		t.Fatalf("play the file: %v", err)
	}
	return p
}

// TestTheFollowerObeysTheHostsPause says the pause the host broadcasts reaches
// the player's own state. The room's OnPause callback is the only thing that
// stands between the room and the mpv here, and it was wired by hand.
func TestTheFollowerObeysTheHostsPause(t *testing.T) {
	if !mpvOnPath(t) {
		t.Skip("no mpv on PATH")
	}
	media := followerMedia(t)
	p := startFollowerPlayer(t, "pause", media)
	w := &window{mpv: p}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// A pause that arrives while mpv is still loading the file is lost: the
	// load sets the play state itself. So the file has to be open first.
	waitFor(t, 10*time.Second, "the file to be open", func() bool {
		tracks, err := p.Tracks(ctx, player.TrackVideo)
		return err == nil && len(tracks) > 0
	})

	// The pause arrives as a room callback; the window's handler is what the
	// room calls.
	w.applyPause(true)
	waitFor(t, 10*time.Second, "the player to pause", func() bool {
		paused, err := p.Paused(ctx)
		return err == nil && paused
	})

	w.applyPause(false)
	waitFor(t, 10*time.Second, "the player to play again", func() bool {
		paused, err := p.Paused(ctx)
		return err == nil && !paused
	})
}

// TestTheFollowerMovesTowardTheHostWithoutTeleporting is the smoothing, end
// to end. The host's raw target is a hundred seconds away; a follower that
// jumped there would be a hundred seconds ahead of everyone else, silently,
// because the number looks plausible. It moves part of the way — a third —
// and the rest arrives with the next heartbeat.
func TestTheFollowerMovesTowardTheHostWithoutTeleporting(t *testing.T) {
	if !mpvOnPath(t) {
		t.Skip("no mpv on PATH")
	}
	media := followerMedia(t)
	p := startFollowerPlayer(t, "seek", media)
	w := &window{mpv: p}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// The file has to be open before the room's commands mean anything. A
	// pause that arrives while mpv is still loading is lost: the load sets
	// the play state itself, and the command that came a moment earlier is
	// overwritten by it.
	waitFor(t, 10*time.Second, "the file to be open", func() bool {
		tracks, err := p.Tracks(ctx, player.TrackVideo)
		return err == nil && len(tracks) > 0
	})

	// Paused, so the position is a known number and the smoothed target is
	// exact: from zero to a hundred, a third of the way is thirty.
	w.applyPause(true)
	waitFor(t, 10*time.Second, "the player to pause", func() bool {
		paused, err := p.Paused(ctx)
		return err == nil && paused
	})

	w.applySeek(100)

	waitFor(t, 10*time.Second, "the player to move toward the host's position", func() bool {
		position, err := p.Position(ctx)
		if err != nil {
			return false
		}
		return position > 1
	})

	position, err := p.Position(ctx)
	if err != nil {
		t.Fatalf("read the position: %v", err)
	}
	if position > 45 {
		t.Errorf("the player is at %.1fs: it followed the raw target instead of the smoothed one", position)
	}
	if position < 15 {
		t.Errorf("the player is at %.1fs: it barely moved toward the host", position)
	}
}

// waitFor polls until the condition holds or the test gives up. The window
// applies a room command on its own goroutine, so the effect lands when it
// lands.
func waitFor(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", within, what)
}
