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
	"sync"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
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

	ctx, cancel := context.WithTimeout(context.Background(), followerWait*3)
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

	ctx, cancel := context.WithTimeout(context.Background(), followerWait*3)
	defer cancel()

	// A pause that arrives while mpv is still loading the file is lost: the
	// load sets the play state itself. So the file has to be open first.
	waitFor(t, followerWait, "the file to be open", func() bool {
		tracks, err := p.Tracks(ctx, player.TrackVideo)
		return err == nil && len(tracks) > 0
	})

	// The pause arrives as a room callback; the window's handler is what the
	// room calls.
	w.applyPause(true)
	waitFor(t, followerWait, "the player to pause", func() bool {
		paused, err := p.Paused(ctx)
		return err == nil && paused
	})

	w.applyPause(false)
	waitFor(t, followerWait, "the player to play again", func() bool {
		paused, err := p.Paused(ctx)
		return err == nil && !paused
	})
}

// TestTheFollowerLandsWhereTheHostIs is the follower's half of the promise,
// end to end. The host's target is a hundred seconds away, and the follower
// goes there — at once, and all the way.
//
// It used to move a third of the way and let the next heartbeat bring the
// rest, so that a follower a minute behind crawled back for a minute and a
// half while everyone else watched on. The goal says the people in a room see
// the same second, and the measurement that found the old behaviour put the
// divergence at 59.8 seconds after a seek (idea-0142, idea-0146).
func TestTheFollowerLandsWhereTheHostIs(t *testing.T) {
	if !mpvOnPath(t) {
		t.Skip("no mpv on PATH")
	}
	media := followerMedia(t)
	p := startFollowerPlayer(t, "seek", media)
	w := &window{mpv: p}

	ctx, cancel := context.WithTimeout(context.Background(), followerWait*3)
	defer cancel()

	// The file has to be open before the room's commands mean anything. A
	// pause that arrives while mpv is still loading is lost: the load sets
	// the play state itself, and the command that came a moment earlier is
	// overwritten by it.
	waitFor(t, followerWait, "the file to be open", func() bool {
		tracks, err := p.Tracks(ctx, player.TrackVideo)
		return err == nil && len(tracks) > 0
	})

	// Paused, so the position is a known number and the target is exact.
	w.applyPause(true)
	waitFor(t, followerWait, "the player to pause", func() bool {
		paused, err := p.Paused(ctx)
		return err == nil && paused
	})

	w.applySeek(100)

	waitFor(t, followerWait, "the player to land on the host's position", func() bool {
		position, err := p.Position(ctx)
		if err != nil {
			return false
		}
		return position > 95
	})

	position, err := p.Position(ctx)
	if err != nil {
		t.Fatalf("read the position: %v", err)
	}
	if position < 95 {
		t.Errorf("the player is at %.1fs: it did not go where the host is", position)
	}
}

// waitFor polls until the condition holds or the test gives up. The window
// applies a room command on its own goroutine, so the effect lands when it
// lands.
// followerWait is how long the follower tests give mpv to do what a person
// would see at once: open a file, pause, come back to the room's state.
//
// Ten seconds was tuned on a quiet machine and turned out to be a coin flip
// on a shared CI runner, where mpv starts cold, the whole module runs at
// once and the race detector makes every wait longer. Thirty seconds was the
// same story one step later: on a loaded runner even that was not enough for
// mpv to open the file, and the test failed on the machine's mood rather
// than on the room's behaviour. A test that fails on the machine's mood is a
// test about the machine.
const followerWait = 90 * time.Second

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

// fakeRoom is a room transport that records what was sent and says nothing
// back: the test needs a room that remembers a state, not a conversation.
type fakeRoom struct {
	mu   sync.Mutex
	sent []any
}

func (f *fakeRoom) Send(ctx context.Context, msg any) error {
	f.mu.Lock()
	f.sent = append(f.sent, msg)
	f.mu.Unlock()
	return nil
}

func (f *fakeRoom) Next(ctx context.Context) (string, any, bool) {
	<-ctx.Done()
	return "", nil, false
}

func (f *fakeRoom) Peers() []string { return nil }
func (f *fakeRoom) SelfID() string  { return "host-peer" }
func (f *fakeRoom) Close() error    { return nil }

// DialHint does nothing here: the fake room is a follower, and a follower
// only follows.
func (f *fakeRoom) DialHint(peers []rtc.PeerAddr) {}

// TestTheFollowerComesBackToTheRoomsPlayState is the room leading, as decided.
//
// The play state travels only when it changes, so a pause can be lost: it
// arrives while the file is still loading, and the load overwrites it. The
// room keeps what it last heard, and the follower comes back to it — on the
// heartbeat, the same beat on which the host repeats its position, so a pause
// meant for a moment survives a few seconds and does not fight the person who
// pressed it.
func TestTheFollowerComesBackToTheRoomsPlayState(t *testing.T) {
	if !mpvOnPath(t) {
		t.Skip("no mpv on PATH")
	}
	media := followerMedia(t)
	p := startFollowerPlayer(t, "converge", media)
	w := &window{mpv: p}

	ctx, cancel := context.WithTimeout(context.Background(), followerWait*3)
	defer cancel()

	waitFor(t, followerWait, "the file to be open", func() bool {
		tracks, err := p.Tracks(ctx, player.TrackVideo)
		return err == nil && len(tracks) > 0
	})

	// The room says the room is paused — as it would after a pause it
	// broadcast and remembered.
	room := rtc.NewRoom(&fakeRoom{}, "host-peer", "host-user", true, nil)
	if err := room.Pause(ctx); err != nil {
		t.Fatalf("the room could not pause: %v", err)
	}
	paused, known := room.PlayState()
	if !known || !paused {
		t.Fatalf("the room does not remember that it is paused: %v %v", paused, known)
	}

	// The follower is playing, because the pause was lost while its file was
	// loading.
	waitFor(t, followerWait, "the player to be playing", func() bool {
		paused, err := p.Paused(ctx)
		return err == nil && !paused
	})

	// And the window's convergence brings it back to the room.
	w.convergeToRoomState(room, false, nil)
	waitFor(t, followerWait, "the player to come back to the room's state", func() bool {
		paused, err := p.Paused(ctx)
		return err == nil && paused
	})
}
