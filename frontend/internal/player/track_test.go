// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// timeAfter is the length of a fake mpv's patience: long enough that a missed
// command fails the test rather than hanging it.
func timeAfter() <-chan time.Time { return time.After(2 * time.Second) }

// A film rarely arrives with one voice. mpv knows every track in the file and
// will switch on command, and until now the application asked for none of it: a
// person watching a foreign film had no way to reach the dubbing or the subtitles
// that were sitting in the same torrent (idea-0080).
//
// The tests that matter here run against a fake mpv rather than the real one,
// because what is being checked is the protocol and the naming — what mpv sends,
// what we send back — and a real mpv would need a real decodable file to say
// anything at all. The real binary is checked separately, over a stream that
// carries tracks.

// fakeMPV is a JSON IPC server that answers one canned reply.
type fakeMPV struct {
	path   string
	reply  map[string]any
	gotCmd []string

	requests chan map[string]any
}

// startFakeMPV runs a fake IPC socket and returns it.
func startFakeMPV(t *testing.T, reply map[string]any) *fakeMPV {
	t.Helper()

	f := &fakeMPV{
		path:     filepath.Join(t.TempDir(), "mpv.sock"),
		reply:    reply,
		requests: make(chan map[string]any, 8),
	}

	listener, err := net.Listen("unix", f.path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return f
}

// startFakeMPVWithEvents runs a fake that speaks first: event lines, then the
// reply. That is the order a real mpv produces when the file is still loading
// while a query is already in flight.
func startFakeMPVWithEvents(t *testing.T, events []string, reply map[string]any) *fakeMPV {
	t.Helper()

	f := &fakeMPV{
		path:     filepath.Join(t.TempDir(), "mpv.sock"),
		reply:    reply,
		requests: make(chan map[string]any, 8),
	}

	listener, err := net.Listen("unix", f.path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer func() { _ = conn.Close() }()
				encoder := json.NewEncoder(conn)
				for _, event := range events {
					if _, err := conn.Write([]byte(event + "\n")); err != nil {
						return
					}
				}
				_ = encoder.Encode(reply)
			}(conn)
		}
	}()
	return f
}

// serve answers every request with the same reply, one line per request, the way
// mpv does. The commands are recorded so a test can assert what was asked.
func (f *fakeMPV) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var request map[string]any
		if err := json.Unmarshal(line, &request); err != nil {
			return
		}
		f.requests <- request
		if err := encoder.Encode(f.reply); err != nil {
			return
		}
	}
}

// playerOn is a Player pointed at a fake socket; nothing here launches mpv.
func playerOn(path string) *Player { return &Player{sock: path} }

// TestTracksComeBackAsTracks checks the wire shape: mpv answers get_property
// track-list with a heterogeneous list of everything in the file, and the caller
// asked for audio only. Handing that unfiltered to a chooser would put video
// tracks and chapter markers in a list of languages.
func TestTracksComeBackAsTracks(t *testing.T) {
	fake := startFakeMPV(t, map[string]any{
		"error": "success",
		"data": []any{
			map[string]any{"id": 1, "type": "video", "selected": true, "codec": "h264"},
			map[string]any{"id": 1, "type": "audio", "lang": "jpn", "title": "Japanese", "selected": true, "default": true, "codec": "aac"},
			map[string]any{"id": 2, "type": "audio", "lang": "rus", "title": "Russian", "codec": "ac3"},
			map[string]any{"id": 3, "type": "sub", "lang": "rus", "title": "Russian", "selected": true},
			map[string]any{"id": 2, "type": "sub", "lang": "eng", "title": "English"},
		},
	})

	tracks, err := playerOn(fake.path).Tracks(context.Background(), TrackAudio)
	if err != nil {
		t.Fatalf("listing the audio tracks: %v", err)
	}
	if len(tracks) != 2 {
		t.Fatalf("got %d audio tracks, wanted the two in the file", len(tracks))
	}
	if tracks[0].Lang != "jpn" || tracks[0].Title != "Japanese" {
		t.Errorf("the first audio track is %q/%q", tracks[0].Lang, tracks[0].Title)
	}
	if !tracks[0].Selected {
		t.Error("the track mpv called selected did not come back selected")
	}
	if tracks[1].ID != 2 || tracks[1].Lang != "rus" {
		t.Errorf("the second audio track is %d/%q", tracks[1].ID, tracks[1].Lang)
	}

	// And the subtitles are the same query with a different filter, not a
	// second command to learn.
	subs, err := playerOn(fake.path).Tracks(context.Background(), TrackSub)
	if err != nil {
		t.Fatalf("listing the subtitle tracks: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("got %d subtitle tracks, wanted the two in the file", len(subs))
	}
}

// TestATrackIsNamedByWhatItActuallyHas is the reason the label is a function and
// not an inline format string.
//
// mpv gives a title, a language, both, or neither, and every combination occurs
// in real files: a remux with "Director's commentary" and no language, a rip
// with "rus" and no title, and a track that is only an id. A label built by
// concatenating whatever is there produces " ( )" for the third kind, which is
// what a person sees when the useful half is missing.
func TestATrackIsNamedByWhatItActuallyHas(t *testing.T) {
	cases := []struct {
		name  string
		track Track
		want  string
	}{
		{"language and title", Track{ID: 2, Lang: "rus", Title: "Russian"}, "Russian (rus)"},
		{"title only", Track{ID: 2, Title: "Director's commentary"}, "Director's commentary"},
		{"language only", Track{ID: 2, Lang: "jpn"}, "jpn"},
		{"neither", Track{ID: 3}, "Track 3"},
	}

	for _, c := range cases {
		if got := c.track.Label(); got != c.want {
			t.Errorf("%s: label is %q, wanted %q", c.name, got, c.want)
		}
	}
}

// TestSelectingATrackSetsThePropertyMpvUnderstands pins the command, because the
// wrong one is silent: `aid` and `sid` are readable aliases, but the settable
// property is `audio` and `sub`, and setting the alias leaves the player exactly
// where it was with no error at all.
func TestSelectingATrackSetsThePropertyMpvUnderstands(t *testing.T) {
	fake := startFakeMPV(t, map[string]any{"error": "success", "data": nil})

	for _, c := range []struct {
		kind TrackKind
		id   int
		want string
	}{
		{TrackAudio, 2, "audio"},
		{TrackSub, 3, "sub"},
	} {
		if err := playerOn(fake.path).SelectTrack(context.Background(), c.kind, c.id); err != nil {
			t.Fatalf("selecting track %d on %s: %v", c.id, c.kind, err)
		}

		select {
		case request := <-fake.requests:
			command, _ := request["command"].([]any)
			if len(command) != 3 || command[0] != "set_property" || command[1] != c.want {
				t.Fatalf("mpv was sent %v, wanted set_property %s", command, c.want)
			}
			if id, _ := command[2].(float64); int(id) != c.id {
				t.Errorf("mpv was sent track %v, wanted %d", command[2], c.id)
			}
		case <-timeAfter():
			t.Fatal("the fake mpv never saw the command")
		}
	}
}

// TestTurningSubtitlesOffIsATrackChoice checks the off switch: zero is how mpv
// spells "no subtitles", and a chooser that cannot offer it forces a person to
// keep burnt-in subs or no subs at all.
func TestTurningSubtitlesOffIsATrackChoice(t *testing.T) {
	fake := startFakeMPV(t, map[string]any{"error": "success", "data": nil})

	if err := playerOn(fake.path).SelectTrack(context.Background(), TrackSub, 0); err != nil {
		t.Fatalf("turning subtitles off: %v", err)
	}

	select {
	case request := <-fake.requests:
		command, _ := request["command"].([]any)
		if id, _ := command[2].(float64); int(id) != 0 {
			t.Errorf("mpv was sent track %v, wanted 0 (off)", command[2])
		}
	case <-timeAfter():
		t.Fatal("the fake mpv never saw the command")
	}
}

// TestAPropertyThatDoesNotExistIsAnErrorNotAnEmptyList is the state before
// anything is loaded: mpv answers "property not found", and reporting that as
// "no tracks" would make the chooser claim the film has one voice.
func TestAPropertyThatDoesNotExistIsAnErrorNotAnEmptyList(t *testing.T) {
	fake := startFakeMPV(t, map[string]any{"error": "property not found"})

	if _, err := playerOn(fake.path).Tracks(context.Background(), TrackAudio); err == nil {
		t.Fatal("a missing property came back as an empty list of tracks")
	}
}

// The fake above checks the protocol. This one checks it against the real
// binary, because the protocol is exactly the kind of thing two pieces of code
// agree on while both being wrong: the property name, the shape of track-list,
// and the id that set_property takes are all things mpv decides, not us.
//
// It needs a file that really has more than one track, so ffmpeg builds a tiny
// mkv with two audio tracks and one subtitle. Skipped where ffmpeg is not
// installed — with a precise reason, rather than silently passing.
func TestRealMpvListsAndSwitchesTracks(t *testing.T) {
	if !mpvAvailable() {
		t.Skip("no mpv on PATH")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH, so there is no multi-track file to play")
	}

	media := buildMultiTrackFile(t, ffmpeg)
	socket := filepath.Join(t.TempDir(), "mpv.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, err := Start(ctx, Options{Binary: "mpv", SocketPath: socket}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if err := p.Play(ctx, media); err != nil {
		t.Fatalf("mpv refused the file: %v", err)
	}

	// The file has to be in before its tracks can be asked about, and those are
	// two different failures with two different causes: a file mpv never opened
	// reports no tracks forever, which looks exactly like "still loading".
	// Naming which one happened is the difference between a diagnosis and a
	// fifteen-second silence.
	waitForLoaded(t, ctx, p, media)

	// mpv parses the tracks a moment after the file opens, and "a moment" is not
	// a number anyone should guess at in a test.
	audio := waitForTracks(t, ctx, p, TrackAudio, 2)
	if len(audio) != 2 {
		t.Fatalf("mpv reported %d audio tracks, the file has two", len(audio))
	}
	if audio[0].Label() == "" {
		t.Error("a track came back with no label to show")
	}

	// Switching is the half that matters: a track that is listed but not
	// selectable is a read-only feature.
	if err := p.SelectTrack(ctx, TrackAudio, audio[1].ID); err != nil {
		t.Fatalf("switching the audio track: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool { return mpvTrackSelected(ctx, t, p, TrackAudio, audio[1].ID) })

	// The subtitle track is waited for rather than asked for once: mpv indexes
	// the streams of a file as it reads them, and the audio tracks are normally
	// there before the subtitle one is. A single query here would be a coin toss
	// that passes on the fourth run and fails on the fifth.
	subs := waitForTracks(t, ctx, p, TrackSub, 1)
	if len(subs) == 0 {
		t.Fatal("the file carries a subtitle track and mpv reported none")
	}
	if err := p.SelectTrack(ctx, TrackSub, subs[0].ID); err != nil {
		t.Fatalf("switching the subtitle track: %v", err)
	}
	// mpv applies a subtitle switch asynchronously: the setter answers "success"
	// and the stream may still be opening. Waiting is the only honest option —
	// a shorter deadline here was a guess that failed about one run in fifteen,
	// which is the worst kind of test failure because it teaches everyone to
	// re-run rather than to look.
	waitFor(t, 15*time.Second, func() bool { return mpvTrackSelected(ctx, t, p, TrackSub, subs[0].ID) })
}

// waitForLoaded blocks until mpv admits the file is in its playlist.
func waitForLoaded(t *testing.T, ctx context.Context, p *Player, media string) {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for {
		var reply struct {
			Error string `json:"error"`
			Data  int    `json:"data"`
		}
		if err := p.request(ctx, []any{"get_property", "playlist-count"}, &reply); err == nil && reply.Data >= 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("mpv never opened %s: it is not in the playlist", media)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitForTracks blocks until mpv reports at least want tracks of a kind.
func waitForTracks(t *testing.T, ctx context.Context, p *Player, kind TrackKind, want int) []Track {
	t.Helper()

	deadline := time.Now().Add(15 * time.Second)
	for {
		tracks, err := p.Tracks(ctx, kind)
		if err == nil && len(tracks) >= want {
			return tracks
		}
		if time.Now().After(deadline) {
			t.Fatalf("mpv never reported %d %s tracks (last: %v, %v)", want, kind, tracks, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// mpvTrackSelected asks mpv which track is current and compares it.
//
// `aid`/`sid` are read-only aliases of the settable `audio`/`sub`, which is
// exactly why they are used here: they answer with a plain id, while the
// current-tracks/* property answers with the whole track entry.
func mpvTrackSelected(ctx context.Context, t *testing.T, p *Player, kind TrackKind, id int) bool {
	t.Helper()

	readable := map[TrackKind]string{TrackAudio: "aid", TrackSub: "sid"}[kind]

	var reply struct {
		Error string `json:"error"`
		Data  int    `json:"data"`
	}
	if err := p.request(ctx, []any{"get_property", readable}, &reply); err != nil {
		return false
	}
	return reply.Data == id
}

// buildMultiTrackFile makes a small mkv with two audio tracks and a subtitle,
// using ffmpeg's synthetic sources — no media file has to exist first.
func buildMultiTrackFile(t *testing.T, ffmpeg string) string {
	t.Helper()

	dir := t.TempDir()
	media := filepath.Join(dir, "multitrack.mkv")

	// Two voices of different pitches, so the tracks are genuinely different
	// audio rather than one stream listed twice.
	cmd := exec.Command(ffmpeg,
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=880:duration=2",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=5:duration=2",
		"-map", "2:v", "-map", "0:a", "-map", "1:a",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
		"-metadata:s:a:0", "title=Original",
		"-metadata:s:a:1", "title=Dub",
		"-y", media,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not build a multi-track file: %v\n%s", err, output)
	}

	// A subtitle track, added the cheap way: an .srt ffmpeg can mux in.
	srt := filepath.Join(dir, "subs.srt")
	writeSubtitle(t, srt)
	withSubs := filepath.Join(dir, "multitrack-subs.mkv")
	cmd = exec.Command(ffmpeg,
		"-i", media, "-i", srt,
		"-map", "0", "-map", "1:s",
		"-c", "copy", "-y", withSubs,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not mux the subtitle: %v\n%s", err, output)
	}
	return withSubs
}

func writeSubtitle(t *testing.T, path string) {
	t.Helper()

	const srt = "1\n00:00:00,000 --> 00:00:01,000\nhello\n\n"
	if err := os.WriteFile(path, []byte(srt), 0o600); err != nil {
		t.Fatalf("write the subtitle: %v", err)
	}
}

// An event line is not a reply, but it decodes like one.
//
// mpv broadcasts async events to every connected IPC client, and a request that
// reads a single line can catch an event instead of its own answer. Decoded into
// the reply shape that is Error="" with no data — indistinguishable from "the
// file is still loading". A track query in that moment reports no tracks; worse,
// a position query in that moment reports position 0 with no error, and the
// buffer reporter hands the room "the start of the film".
func TestAnEventLineIsNotMistakenForAReply(t *testing.T) {
	fake := startFakeMPVWithEvents(t,
		[]string{`{"event":"tracks-changed"}`},
		map[string]any{
			"error": "success",
			"data": []any{
				map[string]any{"id": 1, "type": "audio", "lang": "jpn"},
			},
		})

	tracks, err := playerOn(fake.path).Tracks(context.Background(), TrackAudio)
	if err != nil {
		t.Fatalf("listing the tracks past an event line: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Lang != "jpn" {
		t.Fatalf("the event line ate the reply: got %+v", tracks)
	}
}
