// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Subtitle files travel an endpoint (idea-0080, variant 2); mpv picks them up
// with sub-add at runtime, which fits the reused player: the process starts
// once per run, while films — and their subtitle files — change. What is
// checked here is the command shape and the bookkeeping around it; that mpv
// really loads an external file over authenticated HTTP is the real-binary
// test below.

func TestSubAddSendsTheURL(t *testing.T) {
	fake := startFakeMPV(map[string]any{"error": "success"})

	url := "http://127.0.0.1:8889/api/v1/torrents/abc/subs.srt"
	if err := fake.play("fake").AddSubtitles(context.Background(), url); err != nil {
		t.Fatalf("adding subtitles: %v", err)
	}

	select {
	case req := <-fake.requests:
		commands, ok := req["command"].([]any)
		if !ok || len(commands) != 2 || commands[0] != "sub-add" || commands[1] != url {
			t.Fatalf("mpv was told %v, not sub-add with the URL", req["command"])
		}
	case <-timeAfter():
		t.Fatal("mpv was never told to add the subtitles")
	}
}

func TestSubAddRefusesAnEmptyURL(t *testing.T) {
	fake := startFakeMPV(map[string]any{"error": "success"})

	if err := fake.play("fake").AddSubtitles(context.Background(), ""); err == nil {
		t.Fatal("an empty URL must not reach mpv")
	}

	select {
	case req := <-fake.requests:
		t.Fatalf("mpv was told %v for an empty URL", req["command"])
	case <-timeAfter():
	}
}

func TestSubAddReportsMpvRefusal(t *testing.T) {
	fake := startFakeMPV(map[string]any{"error": "invalid parameter"})

	err := fake.play("fake").AddSubtitles(context.Background(), "http://127.0.0.1/x.srt")
	if err == nil {
		t.Fatal("mpv refused the file and the caller heard success")
	}
}

func TestRemoveSubtitlesRemovesOnlyExternalOnes(t *testing.T) {
	fake := startFakeMPV(map[string]any{
		"error": "success",
		"data": []any{
			map[string]any{"id": 1, "type": "sub", "lang": "rus", "selected": true},
			map[string]any{"id": 2, "type": "sub", "lang": "eng", "external": true},
			map[string]any{"id": 3, "type": "sub", "lang": "deu", "external": true},
			map[string]any{"id": 1, "type": "audio", "lang": "jpn", "external": true},
		},
	})

	if err := fake.play("fake").RemoveExternalSubtitles(context.Background()); err != nil {
		t.Fatalf("removing external subtitles: %v", err)
	}

	removed := map[float64]bool{}
	for {
		select {
		case req := <-fake.requests:
			commands, _ := req["command"].([]any)
			if len(commands) == 0 || commands[0] == "get_property" {
				continue
			}
			if len(commands) != 2 || commands[0] != "sub-remove" {
				t.Fatalf("unexpected command to mpv: %v", req["command"])
			}
			id, ok := commands[1].(float64)
			if !ok {
				t.Fatalf("sub-remove id is %T, not a number", commands[1])
			}
			removed[id] = true
		case <-timeAfter():
			goto done
		}
	}
done:
	if len(removed) != 2 || !removed[2] || !removed[3] {
		t.Fatalf("removed %v, wanted exactly the two external subtitle tracks", removed)
	}
}

func TestRemoveSubtitlesWithNothingExternalSendsNothing(t *testing.T) {
	fake := startFakeMPV(map[string]any{
		"error": "success",
		"data": []any{
			map[string]any{"id": 1, "type": "sub", "lang": "rus", "selected": true},
		},
	})

	if err := fake.play("fake").RemoveExternalSubtitles(context.Background()); err != nil {
		t.Fatalf("removing nothing external: %v", err)
	}

	select {
	case req := <-fake.requests:
		commands, _ := req["command"].([]any)
		if len(commands) > 0 && commands[0] != "get_property" {
			t.Fatalf("mpv was told %v with nothing external to remove", req["command"])
		}
	case <-timeAfter():
	}
}

// TestRealMpvLoadsExternalSubtitlesOverAuthenticatedHTTP is the whole point of
// variant 2: mpv fetches the subtitle file itself, over HTTP, carrying the
// same header the stream uses — so the endpoint needs no ticket in the URL.
// ffmpeg builds a tiny mp4, an httptest server plays the subtitles endpoint
// (recording the header it saw), and mpv must both list the track as external
// and show the header on the fetch. Skipped with a precise reason where mpv
// or ffmpeg is absent.
func TestRealMpvLoadsExternalSubtitlesOverAuthenticatedHTTP(t *testing.T) {
	if !mpvAvailable() {
		t.Skip("no mpv on PATH")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH, so there is no file to play")
	}

	dir := t.TempDir()
	media := filepath.Join(dir, "film.mp4")
	cmd := exec.Command(ffmpeg,
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=5:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-y", media,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not build a file to play: %v\n%s", err, output)
	}

	const srt = "1\n00:00:00,000 --> 00:00:01,000\nhello\n\n"
	var mu sync.Mutex
	var sawToken string
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sawToken = r.Header.Get("X-Access-Token")
		hits++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(srt))
	}))
	defer server.Close()

	socket := testSocketPath(t, "extsubs")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	header, err := FormatHeader("X-Access-Token", "letmein")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Start(ctx, Options{
		Binary:     "mpv",
		SocketPath: socket,
		ExtraArgs:  []string{"--vo=null", "--ao=null"},
	}, []string{header})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if err := p.Play(ctx, media); err != nil {
		t.Fatalf("mpv refused the file: %v", err)
	}
	waitForLoadedPatiently(t, ctx, p, media)

	// Being in the playlist is not being loaded. mpv answers sub-add before
	// the file is open with "error running command" and says nothing else —
	// no log line, no hint — so a test that fires the command as soon as the
	// playlist count is right fails roughly one run in seven on a laptop and
	// reddened both the macOS and the Linux job on CI. A track is the proof
	// that mpv is past that point, which is the state sub-add needs.
	waitForTracks(t, ctx, p, TrackVideo, 1)

	if err := p.AddSubtitles(ctx, server.URL+"/subs.srt"); err != nil {
		t.Fatalf("mpv refused the external subtitles: %v\n--- what mpv said ---\n%s",
			err, tailLines(p.Log(), 25))
	}
	subs := waitForTracks(t, ctx, p, TrackSub, 1)
	found := false
	for _, s := range subs {
		if s.External {
			found = true
		}
	}
	if !found {
		t.Fatalf("mpv lists %+v, none of it external", subs)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		mu.Lock()
		token, n := sawToken, hits
		mu.Unlock()
		if n > 0 {
			if token != "letmein" {
				t.Fatalf("the subtitle fetch carried %q, not the access token", token)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("mpv never fetched the subtitle file over HTTP")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitForLoadedPatiently blocks until mpv admits the file is in its playlist.
//
// Same shape as waitForLoaded, but twice as patient: this test runs mpv
// alongside the file-building ffmpeg in a suite that spawns several players
// at once, and on a loaded machine the open once took longer than fifteen
// seconds. A longer deadline fails a truly broken open just as loudly — only
// later — while a tight one fails a slow machine for being slow.
func waitForLoadedPatiently(t *testing.T, ctx context.Context, p *Player, media string) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for {
		var reply struct {
			Error string `json:"error"`
			Data  int    `json:"data"`
		}
		if err := p.request(ctx, []any{"get_property", "playlist-count"}, &reply); err == nil && reply.Data >= 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("mpv never opened %s: it is not in the playlist\n--- what mpv said ---\n%s",
				media, tailLines(p.Log(), 25))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// mpv answers "error running command" for a sub-add sent before a file is
// loaded, and adds nothing to its log to say so — proven against the real
// binary, which is why this message once cost a CI run on two platforms. The
// person choosing a subtitle track sees the error verbatim in the window's
// status line, so the sentence has to carry the reason itself.
func TestAddSubtitlesSaysWhenThereIsNoFileToAttachThemTo(t *testing.T) {
	if !mpvAvailable() {
		t.Skip("no mpv on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, err := Start(ctx, Options{
		Binary:     "mpv",
		SocketPath: testSocketPath(t, "nofile"),
		ExtraArgs:  []string{"--vo=null", "--ao=null"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	err = p.AddSubtitles(ctx, "http://127.0.0.1:1/subs.srt")
	if err == nil {
		t.Fatal("mpv took subtitles with no file loaded, which is what this pins")
	}
	if !strings.Contains(err.Error(), "no file loaded") {
		t.Fatalf("the error does not say what is wrong: %v", err)
	}
}
