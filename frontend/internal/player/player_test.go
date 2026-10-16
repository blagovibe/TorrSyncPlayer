// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestArgsTellMpvToSendTheHeader(t *testing.T) {
	// The whole point of running mpv as a process: it accepts custom headers,
	// so the media endpoint is an ordinary protected route. Without this flag
	// no one can play anything, because /stream requires the header.
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

// TestArgsJoinHeadersIntoOneFlag pins the shape mpv actually honors: repeated
// --http-header-fields do not accumulate, the last flag wins and the earlier
// headers never leave the machine. Two headers must ride one comma-joined
// flag, or the backend answers 401 to a player that was given everything
// (bug-0089). Proven against the real binary.
func TestArgsJoinHeadersIntoOneFlag(t *testing.T) {
	args := Args("/tmp/mpv.sock", []string{"X-Access-Token: tok", "X-Client-ID: peer-7"})

	count := 0
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--http-header-fields") {
			continue
		}
		count++
		if arg != "--http-header-fields=X-Access-Token: tok,X-Client-ID: peer-7" {
			t.Fatalf("mpv was told %q, not both headers in one flag", arg)
		}
	}
	if count != 1 {
		t.Fatalf("mpv was told %d header flags, wanted exactly one", count)
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
	if _, err := FormatHeader("X-Client-ID", "a,b"); err == nil {
		t.Fatal("a value with a comma was accepted, and mpv would split it into two headers")
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

	socket := testSocketPath(t, "position")

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

// The buffer reporter turns a position in seconds into an offset in bytes, and
// it can only do that against the file's full length: the same position is
// halfway through a short clip and a rounding error in a long film. The length
// is mpv's to know — it read the file's header — so the question is asked of
// mpv, over the same IPC every other property travels on.
func TestDurationAsksMpvForTheFileLength(t *testing.T) {
	fake := startFakeMPV(map[string]any{"error": "success", "data": 3520.5})

	duration, err := fake.play("fake").Duration(context.Background())
	if err != nil {
		t.Fatalf("reading the duration: %v", err)
	}
	if duration != 3520.5 {
		t.Errorf("the duration came back as %v, mpv said 3520.5", duration)
	}

	select {
	case request := <-fake.requests:
		command, _ := request["command"].([]any)
		if len(command) != 2 || command[0] != "get_property" || command[1] != "duration" {
			t.Fatalf("mpv was sent %v, wanted get_property duration", command)
		}
	case <-timeAfter():
		t.Fatal("the fake mpv never saw the question")
	}
}

// TestDurationBeforeAnythingIsLoadedIsAnErrorNotAZero is the state at every
// launch: mpv has no file and answers "property unavailable". A zero returned
// as success would tell the reporter the film is zero seconds long, which is
// the one number it must never guess — it would aim the download window at
// nowhere.
func TestDurationBeforeAnythingIsLoadedIsAnErrorNotAZero(t *testing.T) {
	fake := startFakeMPV(map[string]any{"error": "property unavailable"})

	if _, err := fake.play("fake").Duration(context.Background()); err == nil {
		t.Fatal("a missing duration came back as a number")
	}
}

// TestRealMpvReportsTheFileDuration checks the property name against the real
// binary. "duration" is what this project's reading says mpv calls it, and a
// wrong name fails exactly like "the file is still loading" — a poll that
// waits forever — rather than with an error worth reading.
func TestRealMpvReportsTheFileDuration(t *testing.T) {
	if !mpvAvailable() {
		t.Skip("mpv is not installed")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH, so there is no file to measure")
	}

	// The multi-track file is two seconds of synthetic media; anything
	// between one second and five says mpv measured the file rather than
	// answering a constant.
	media := buildMultiTrackFile(t, ffmpeg)
	socket := testSocketPath(t, "duration")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, err := Start(ctx, Options{
		Binary:     "mpv",
		SocketPath: socket,
		ExtraArgs:  []string{"--vo=null", "--ao=null"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if err := p.Play(ctx, media); err != nil {
		t.Fatalf("mpv refused the file: %v", err)
	}

	var duration float64
	waitFor(t, 15*time.Second, func() bool {
		reading, err := p.Duration(ctx)
		if err == nil && reading > 0 {
			duration = reading
			return true
		}
		return false
	})

	if duration < 1 || duration > 5 {
		t.Errorf("mpv says the two-second file is %v seconds long", duration)
	}
}

func TestRealMpvSendsTheHeaderItWasGiven(t *testing.T) {
	// The claim that started idea-0066, checked against the real binary: mpv
	// puts the configured header on the stream request.
	recorder := newHeaderRecorder(t)
	socket := testSocketPath(t, "position")

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
		SocketPath: testSocketPath(t, "missing"),
	}, nil)
	if err == nil {
		t.Fatal("starting a nonexistent binary reported success")
	}
}

// testSocketPath is what mpv should listen on, short enough and in the right
// shape for every platform.
//
// Three different platform limits meet here, and all three were found by a real
// run rather than by reading:
//
//   - macOS caps unix socket paths at 104 bytes, and t.TempDir() on a runner is
//     already most of that before the test's own name is appended — hence /tmp.
//   - Windows has no unix sockets at all, so net.Listen("unix") there is
//     "invalid argument"; the transport is a named pipe.
//   - A Windows named pipe takes a *name*, and `\\.\pipe\` is added by dial.
//     A path is not a valid pipe name, which is why mpv is given a bare name
//     here and the full path is refused on purpose.
//
// The name carries the pid so two players in one run cannot collide, and the
// cleanup removes the socket rather than leaving one for the next run.
func testSocketPath(t *testing.T, label string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		name := fmt.Sprintf("torrsync-%s-%d", label, os.Getpid())
		t.Cleanup(func() {})
		return name
	}

	path := filepath.Join("/tmp", fmt.Sprintf("torrsync-%s-%d.sock", label, os.Getpid()))
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
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
