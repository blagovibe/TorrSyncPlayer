// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

// The project's first sentence is that the file starts playing as soon as
// enough data has arrived, and the rest comes in underneath the playback.
// Everything else in the project takes it on faith: the byte-level test proves
// that a partial torrent serves its head, and the buffer's own tests prove
// that the head is fetched first. Neither of them puts a player in front of
// the film.
//
// This one does. The source serves the first third of a real film and then
// stops, exactly as a torrent with only its head downloaded stops. The player
// must play what is there — and when the rest is finally let through, it must
// go on playing past the part that had not arrived.
//
// The test asks for the rest itself, so nothing here depends on how long a
// player takes to start: the film is played on partial data, or the test fails
// on the spot.
//
// The source stands in for a torrent with only its head downloaded, and the
// real swarm is not repeated here on purpose: it is slow, and it is already
// proven byte for byte in the backend's own test.

func TestRealMpvPlaysWhileTheSourceIsIncomplete(t *testing.T) {
	if !mpvAvailable() {
		t.Skip("mpv is not installed")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH, so there is no file to play")
	}

	media := buildPlayableFile(t, ffmpeg)
	size := fileSize(t, media)
	head := int(size / 3)

	source := newStallingSource(t, media, head)

	socket := testSocketPath(t, "partial")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// The player's own arguments, with no window and no sound: this is the
	// application asking mpv to play, nothing else.
	p, err := Start(ctx, Options{
		Binary:     "mpv",
		SocketPath: socket,
		ExtraArgs:  []string{"--vo=null", "--ao=null"},
	}, nil)
	if err != nil {
		t.Fatalf("mpv did not start: %v", err)
	}
	t.Cleanup(p.Close)

	if err := p.Play(ctx, source.URL()); err != nil {
		t.Fatalf("mpv refused the stream: %v", err)
	}

	// The film starts on what has arrived. The source is holding the rest
	// and will not let it go until the test says so, so a position that
	// moves now is a position mpv reached on a third of the file.
	var startedAt float64
	waitFor(t, 60*time.Second, func() bool {
		position, err := p.Position(ctx)
		if err == nil && position > 0.5 {
			startedAt = position
			return true
		}
		return false
	})

	// The source really was holding the rest: it had sent a third of the
	// film and no more.
	if sent := source.Sent(); int64(sent) >= size {
		t.Fatalf("the source sent the whole film at once: %d of %d bytes", sent, size)
	}

	// And now the rest comes in underneath the playback: the position goes
	// on past where it stood.
	source.SendTheRest()
	waitFor(t, 60*time.Second, func() bool {
		position, err := p.Position(ctx)
		return err == nil && position > startedAt+1
	})
}

// buildPlayableFile makes a short film whose header is at the front, so a
// player can start on it without seeking to the end for the index — which is
// what a partially downloaded torrent offers.
func buildPlayableFile(t *testing.T, ffmpeg string) string {
	t.Helper()

	media := filepath.Join(t.TempDir(), "film.mp4")
	// The same ffmpeg the project's other films are built with, plus one
	// thing: the index goes to the front, so a player can start on the head
	// of the file without seeking to the end for it.
	cmd := exec.Command(ffmpeg,
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=5:duration=10",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-movflags", "+faststart",
		"-y", media,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not build a film to play: %v\n%s", err, output)
	}
	return media
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the film is not there: %v", err)
	}
	return info.Size()
}

// stallingSource serves a file in two pieces: the head at once, the rest when
// the test says so. It is a torrent with only its head downloaded, seen from
// the player's side.
type stallingSource struct {
	*httptest.Server

	mu       sync.Mutex
	sent     int
	headSize int

	rest chan struct{}
	once sync.Once
}

func newStallingSource(t *testing.T, media string, head int) *stallingSource {
	t.Helper()

	whole := int(fileSize(t, media))
	content, err := os.ReadFile(media)
	if err != nil {
		t.Fatalf("the film could not be read: %v", err)
	}

	src := &stallingSource{headSize: head, rest: make(chan struct{})}
	src.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", strconv.Itoa(whole))
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		w.Write(content[:head])
		if flusher != nil {
			flusher.Flush()
		}
		src.mu.Lock()
		src.sent = head
		src.mu.Unlock()

		// The rest arrives when the test lets it through. Until then the
		// source is a torrent with only its head: the film has to play
		// anyway. The request's own end is the way out, so a failed test
		// does not hang the server's shutdown.
		select {
		case <-src.rest:
		case <-r.Context().Done():
			return
		}
		w.Write(content[head:])
		src.mu.Lock()
		src.sent = whole
		src.mu.Unlock()
	}))
	t.Cleanup(src.Close)
	return src
}

// Sent is how much the source has sent so far.
func (s *stallingSource) Sent() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent
}

// SendTheRest lets the rest of the film through.
func (s *stallingSource) SendTheRest() {
	s.once.Do(func() { close(s.rest) })
}

func (s *stallingSource) URL() string { return s.Server.URL + "/film.mp4" }
