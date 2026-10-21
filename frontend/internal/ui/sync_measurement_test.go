//go:build measurement

// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
)

// The room's promise is that everyone sees the same moment in the same second.
// Nothing in the project measures that number: the tests prove that a command
// arrives, and the goal promises what a person sees.
//
// This is the measurement. Two real players, one real file, one real room over
// a real libp2p stream, and the difference between the two positions sampled
// every half second — while the room plays, and after the host jumps forward.
//
// It is behind the "measurement" tag because it takes minutes, not seconds,
// and it is a measurement rather than a check: it is run by its own job, and
// what it produces is a row in measurements/sync.md.
//
// What it drives is the code the application runs: the same `roomTick` the
// window's loop calls, the same `applySeek`, the same `FollowPosition`, the
// same mpv. The only thing left out is the painting of widgets — a
// measurement of how far apart two people are does not need a window to look
// at.

// The numbers the measurement produces.
const (
	// measurementTick is the loop's cadence: half a second, the same as the
	// window's own poll.
	measurementTick = 500 * time.Millisecond

	// measurementSettleSeconds is how long the room plays before anything is
	// measured, so the first samples — when the players are still opening the
	// file — do not count as divergence.
	measurementSettleSeconds = 10

	// measurementSteadySeconds is how long the settled room is watched. The
	// steady divergence is what a person sees when nothing happens.
	measurementSteadySeconds = 20

	// measurementJumpSeconds is how far the host jumps. A jump is the moment
	// the room's promise is hardest to keep: the follower is left behind on
	// purpose, and the question is how long it takes to come back.
	measurementJumpSeconds = 60

	// measurementTransientSeconds is how long the aftermath of the jump is
	// watched.
	measurementTransientSeconds = 60

	// measurementLandedSeconds is how long the room may take to put the
	// jump into effect. The host sends the position at once when it sees a
	// seek — it does not wait for the heartbeat — so the flight is the
	// host's half-second poll plus the network.
	measurementLandedSeconds = 2
)

// syncMeasurement is one sample of the room's divergence.
type syncMeasurement struct {
	at        time.Duration
	host      float64
	guest     float64
	diverged  float64
	afterJump bool
}

// TestTheRoomKeepsEveryoneInTheSameSecond measures the room's promise.
//
// It is a measurement, so it fails only on the number the owner settled on —
// the threshold is in the constant below, and the numbers it printed are the
// ones the row in measurements/sync.md is made of.
func TestTheRoomKeepsEveryoneInTheSameSecond(t *testing.T) {
	media := measurementMedia(t)
	hostPlayer := measurementPlayer(t, "measure-host", media)
	guestPlayer := measurementPlayer(t, "measure-guest", media)

	hostMesh, guestMesh := measurementRoom(t)
	hostWindow := &window{mpv: hostPlayer, clientID: "host-user"}
	guestWindow := &window{mpv: guestPlayer, clientID: "guest-user"}

	hostRoom := rtc.NewRoom(hostMesh, hostMesh.SelfID(), "host-user", true, measurementCallbacks(hostWindow))
	guestRoom := rtc.NewRoom(guestMesh, guestMesh.SelfID(), "guest-user", false, measurementCallbacks(guestWindow))
	go hostRoom.Run(context.Background())
	go guestRoom.Run(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	go hostWindow.tickLoop(ctx, hostPlayer, hostRoom)
	go guestWindow.tickLoop(ctx, guestPlayer, guestRoom)

	// The room has to be together before anything is measured: a divergence
	// between two participants that have not met yet is a meeting that has
	// not happened, not a room that has drifted.
	require.Eventually(t, func() bool {
		return len(hostRoom.Peers()) == 1 && len(guestRoom.Peers()) == 1
	}, 60*time.Second, 200*time.Millisecond, "the two participants never met")

	started := time.Now()
	var mu sync.Mutex
	var samples []syncMeasurement
	jumped := false
	var jumpedAt time.Duration

	// The sampler is the measurement: the difference between where the host
	// is and where the follower is, every half second.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(measurementTick):
			}
			hostPos, err := hostPlayer.Position(ctx)
			if err != nil {
				continue
			}
			guestPos, err := guestPlayer.Position(ctx)
			if err != nil {
				continue
			}
			mu.Lock()
			samples = append(samples, syncMeasurement{
				at:        time.Since(started),
				host:      hostPos,
				guest:     guestPos,
				diverged:  hostPos - guestPos,
				afterJump: jumped,
			})
			mu.Unlock()
		}
	}()

	// The settled room first: what a person sees when nothing happens.
	time.Sleep((measurementSettleSeconds + measurementSteadySeconds) * time.Second)

	// Then the jump: the host moves, and the follower is left behind on
	// purpose. The question is how long it takes to come back.
	require.NoError(t, hostPlayer.Seek(ctx, hostPlayerPosition(t, hostPlayer)+measurementJumpSeconds))
	mu.Lock()
	jumped = true
	jumpedAt = time.Since(started)
	mu.Unlock()

	time.Sleep(measurementTransientSeconds * time.Second)

	mu.Lock()
	defer mu.Unlock()
	report := summarise(samples, jumpedAt)
	t.Logf("steady divergence: max %.2fs over %d samples; the jump of %ds landed in %s, and the divergence after it is max %.2fs (during the flight it was %.2fs, which is the jump itself)",
		report.steadyMax, report.steadySamples, measurementJumpSeconds, report.landedAfter, report.afterLandingMax, report.duringFlightMax)

	// The numbers in a shape a script can read. The series in
	// measurements/sync.md is written from these lines, and a line nobody can
	// parse is a line nobody can keep.
	t.Logf("steadyDivergence: %.2f", report.steadyMax)
	t.Logf("landingSeconds: %.2f", report.landedAfter.Seconds())
	t.Logf("samples: %d", report.steadySamples)
	if report.steadyMax <= steadyDivergenceLimit && report.afterLandingMax <= steadyDivergenceLimit {
		t.Logf("outcome: within the promise")
	} else {
		t.Logf("outcome: outside the promise")
	}

	require.LessOrEqual(t, report.steadyMax, steadyDivergenceLimit,
		"the room drifted %0.2fs apart while nothing was happening", report.steadyMax)
	require.LessOrEqual(t, report.afterLandingMax, steadyDivergenceLimit,
		"after the jump the room stayed %0.2fs apart — the goal promises the same second",
		report.afterLandingMax)
	require.LessOrEqual(t, report.landedAfter, measurementLandedSeconds*time.Second,
		"the jump took %s to take effect", report.landedAfter)
}

// steadyDivergenceLimit is the divergence a room may reach while it simply
// plays, and the divergence it may keep after a jump. It is the goal's own
// number: the people in a room see the same second.
const steadyDivergenceLimit = 1.0

// syncReport is what the measurement found.
type syncReport struct {
	// steadyMax is the worst divergence while the room simply played.
	steadyMax float64
	// steadySamples is how many samples that was measured from.
	steadySamples int
	// landedAfter is how long the jump took to take effect in the room.
	landedAfter time.Duration
	// duringFlightMax is the divergence while the jump was still in flight.
	// It is the size of the jump itself: the follower is at the old position
	// until the command arrives, and no design makes that instant.
	duringFlightMax float64
	// afterLandingMax is the worst divergence after the jump took effect.
	afterLandingMax float64
}

// summarise reads the samples into the numbers the goal is about. jumpedAt is
// when the host jumped: the flight of the command is counted from there.
func summarise(samples []syncMeasurement, jumpedAt time.Duration) syncReport {
	var report syncReport
	landed := false
	for _, s := range samples {
		if !s.afterJump {
			// The first samples are the players still opening the file.
			if s.at < measurementSettleSeconds*time.Second {
				continue
			}
			report.steadySamples++
			if diff := abs(s.diverged); diff > report.steadyMax {
				report.steadyMax = diff
			}
			continue
		}
		if !landed {
			if abs(s.diverged) > steadyDivergenceLimit {
				// The command is still in flight.
				if diff := abs(s.diverged); diff > report.duringFlightMax {
					report.duringFlightMax = diff
				}
				continue
			}
			landed = true
			report.landedAfter = s.at - jumpedAt
		}
		if diff := abs(s.diverged); diff > report.afterLandingMax {
			report.afterLandingMax = diff
		}
	}
	return report
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// hostPlayerPosition reads where the host is, so the jump is relative.
func hostPlayerPosition(t *testing.T, p *player.Player) float64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	position, err := p.Position(ctx)
	require.NoError(t, err)
	return position
}

// tickLoop is the window's loop without the widgets: the same decisions, the
// same cadence, nothing painted.
func (w *window) tickLoop(ctx context.Context, mpv *player.Player, room *rtc.Room) {
	ticker := time.NewTicker(measurementTick)
	defer ticker.Stop()
	var st watchState
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		w.roomTick(mpv, room, &st)
	}
}

// measurementCallbacks are the room's effects on the player, without the
// window's painting: the same two calls the window makes.
func measurementCallbacks(w *window) *rtc.Callbacks {
	return &rtc.Callbacks{
		OnPause: func(paused bool) { w.applyPause(paused) },
		OnSeek:  func(seconds float64) { w.applySeek(seconds) },
	}
}

// measurementRoom puts two participants in the same room the way the product
// does it: the guest dials the address the code carried. The rendezvous is
// given an address that cannot answer, because the room does not need it —
// and a test that leans on somebody else's service fails for somebody else's
// reasons.
func measurementRoom(t *testing.T) (host, guest *rtc.Mesh) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	bootstrap := unreachableBootstrap()
	community, password, key := "measure-room", "measure-password", "measure-key"

	host = rtc.NewMesh(rtc.MeshOptions{
		Community: community,
		Password:  password,
		Key:       key,
		Bootstrap: bootstrap,
		UserID:    "host-user",
	})
	require.NoError(t, host.Join(ctx, true), "the host could not join")

	guest = rtc.NewMesh(rtc.MeshOptions{
		Community: community,
		Password:  password,
		Key:       key,
		Bootstrap: bootstrap,
		UserID:    "guest-user",
		// The addresses the code carried: the room meets by them.
		Addrs: host.Addrs(),
	})
	require.NoError(t, guest.Join(ctx, false), "the guest could not join")

	t.Cleanup(func() {
		_ = guest.Close()
		_ = host.Close()
	})
	return host, guest
}

// unreachableBootstrap is a rendezvous that cannot answer: a real peer ID on a
// closed port.
func unreachableBootstrap() string {
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		panic("rtc: no randomness for the unreachable bootstrap: " + err.Error())
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		panic("rtc: no peer id for the unreachable bootstrap: " + err.Error())
	}
	return fmt.Sprintf("/ip4/127.0.0.1/tcp/1/p2p/%s", id)
}

// measurementMedia builds a real file to play: long enough that the jump
// forward does not run into its end, small enough to make in a moment.
func measurementMedia(t *testing.T) string {
	t.Helper()

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatalf("no ffmpeg on PATH: the measurement needs a real file to play: %v", err)
	}
	media := filepath.Join(t.TempDir(), "film.mp4")
	cmd := exec.Command(ffmpeg,
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=5:duration=300",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-y", media,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg could not build a file to play: %v\n%s", err, out)
	}
	return media
}

// measurementPlayer starts a real mpv with the file loaded and playing.
func measurementPlayer(t *testing.T, label, media string) *player.Player {
	t.Helper()

	if _, err := exec.LookPath("mpv"); err != nil {
		t.Fatalf("no mpv on PATH: the measurement needs a real player: %v", err)
	}

	// The socket path is short on purpose: macOS caps unix socket paths at
	// 104 bytes, and a test's own temporary directory can use most of it.
	socket := filepath.Join("/tmp", fmt.Sprintf("torrsync-measure-%s-%d.sock", label, os.Getpid()))
	t.Cleanup(func() { _ = os.Remove(socket) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, err := player.Start(ctx, player.Options{
		Binary:     "mpv",
		SocketPath: socket,
		ExtraArgs:  []string{"--vo=null", "--ao=null"},
	}, nil)
	require.NoError(t, err, "start mpv")
	t.Cleanup(p.Close)

	require.NoError(t, p.Play(ctx, media), "play the file")
	return p
}
