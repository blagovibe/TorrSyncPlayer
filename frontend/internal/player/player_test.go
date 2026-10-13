// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArgsTellMpvToSendTheHeader(t *testing.T) {
	// The whole point of running mpv as a process: it accepts custom headers,
	// so the media endpoint can be a protected route instead of a public one
	// with a bearer ticket in the URL. Without this flag the whole arrangement
	// collapses back into deviation-0029.
	args := Args("/tmp/mpv.sock", []string{"X-Access-Token: abc"}, "--volume=50")

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--http-header-fields=X-Access-Token: abc") {
		t.Fatalf("mpv was not told to send the header: %v", args)
	}
	if !strings.Contains(joined, "--input-ipc-server=/tmp/mpv.sock") {
		t.Fatalf("mpv was not told where its IPC socket is: %v", args)
	}
	if args[len(args)-1] != "--volume=50" {
		t.Fatal("extra arguments must come last so they win over the defaults")
	}
}

func TestArgsCarryNoHeaderWhenThereIsNone(t *testing.T) {
	args := Args("/tmp/mpv.sock", nil)

	for _, arg := range args {
		if strings.HasPrefix(arg, "--http-header-fields") {
			t.Fatalf("an empty header was passed: %q", arg)
		}
	}
}

func TestFormatHeaderRejectsANewlineInTheValue(t *testing.T) {
	// A newline would split one header into two, and the second half would be
	// sent as a header of its own — the client's own doing, unaudited.
	if _, err := FormatHeader("X-Access-Token", "abc\r\nX-Admin: true"); err == nil {
		t.Fatal("a value with an escaped newline was accepted")
	}
	if _, err := FormatHeader("", "abc"); err == nil {
		t.Fatal("an empty header name was accepted")
	}
	got, err := FormatHeader("X-Access-Token", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got != "X-Access-Token: abc123" {
		t.Fatalf("got %q", got)
	}
}

// The tests below drive a real mpv. Everything above checks only what we tell
// it to do; this checks that it does it.

func mpvAvailable() bool {
	_, err := exec.LookPath("mpv")
	return err == nil
}

func startRealPlayer(t *testing.T) *Player {
	t.Helper()

	if !mpvAvailable() {
		t.Skip("mpv is not installed")
	}

	socket := filepath.Join(t.TempDir(), "mpv.sock")

	header, err := FormatHeader("X-Access-Token", "letmein")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p, err := Start(ctx, Options{
		SocketPath: socket,
	}, []string{header})
	if err != nil {
		t.Fatalf("mpv did not start: %v", err)
	}
	t.Cleanup(p.Close)
	return p
}

func TestRealMpvOpensItsSocket(t *testing.T) {
	p := startRealPlayer(t)

	if err := p.Pause(context.Background()); err != nil {
		t.Fatalf("mpv did not accept a command over IPC: %v", err)
	}
}

func TestRealMpvPositionBeforePlayingIsZero(t *testing.T) {
	// Before anything is loaded there is no position. Reporting it as an
	// error rather than a zero keeps the caller from treating "no position"
	// as "the start of the film" and reporting it to the room.
	p := startRealPlayer(t)

	position, err := p.Position(context.Background())
	if err == nil && position != 0 {
		t.Fatalf("position before playing is %v, expected 0", position)
	}
}

func TestRealMpvSendsTheHeaderItWasGiven(t *testing.T) {
	// The claim that started idea-0066, checked against the real binary: mpv
	// puts the configured header on the stream request.
	recorder := newHeaderRecorder(t)
	socket := filepath.Join(t.TempDir(), "mpv.sock")

	header, err := FormatHeader("X-Access-Token", "letmein")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	withHeader, err := Start(ctx, Options{
		Binary:     "mpv",
		SocketPath: socket,
	}, []string{header})
	if err != nil {
		t.Fatal(err)
	}
	defer withHeader.Close()

	// A URL that answers 401 or a broken stream: either way mpv must have made
	// the request, and what matters is what arrived.
	_ = withHeader.Play(ctx, recorder.URL)

	waitFor(t, 3*time.Second, func() bool { return recorder.Seen() })
}

func TestSocketPathMustBeGiven(t *testing.T) {
	_, err := Start(context.Background(), Options{}, nil)
	if err == nil {
		t.Fatal("mpv was started with no IPC socket path")
	}
}

func TestStartReportsAMissingBinary(t *testing.T) {
	_, err := Start(context.Background(), Options{
		Binary:     filepath.Join(t.TempDir(), "definitely-not-mpv"),
		SocketPath: filepath.Join(t.TempDir(), "mpv.sock"),
	}, nil)
	if err == nil {
		t.Fatal("starting a nonexistent binary reported success")
	}
}

func waitFor(t *testing.T, within time.Duration, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition was not met in time")
}
