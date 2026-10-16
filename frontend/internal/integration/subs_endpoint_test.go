// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
)

// Subtitles end to end: a real seeder holding a real .srt in a real swarm, a
// leecher pulling it, the subtitles endpoint serving its bytes behind the
// token, and a real mpv loading them over authenticated HTTP.
//
// The backend unit tests pin the gates with a stub backend ("payload"); what
// only this test can prove is the rest of the road: anacrolix resolving the
// file, ServeStream carrying real bytes, mpv accepting an extensionless URL,
// and the access-token header arriving on the subtitle fetch. Skipped with a
// precise reason where mpv is absent.
func TestSubtitlesTravelTheEndpoint(t *testing.T) {
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("no mpv on PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	const srt = "1\n00:00:00,000 --> 00:00:01,000\nhello\n\n"
	announce, _ := startLoopbackTracker(t)

	seederData := t.TempDir()
	// The file on disk must be named what the torrent says: the seeder
	// verifies its pieces against the name in the info dict, and the content
	// being subtitles while the name says .mkv changes nothing on this road —
	// the endpoint serves by index, mpv sniffs by content.
	writeFile(t, seederData+"/torrents/"+swarmName, []byte(srt))
	seeder := startServerWith(t, "seeder-peer", seederData, true)

	var added struct {
		ID string `json:"id"`
	}
	require.Equal(t, 201, seeder.do(t, "POST", "/api/v1/torrents",
		map[string]string{"torrentFile": encodeTorrent(t, buildSeededTorrent(t, announce, []byte(srt)))}, &added),
		"the seeder could not add its own subtitles")
	require.NotEmpty(t, added.ID)
	waitForProgress(t, seeder, added.ID, 1, 60*time.Second, "the seeder never verified its subtitles")

	leecher := startServer(t, "leecher-peer")
	leecherClient := api.New(leecher.BaseURL, leecher.Token)
	leecherClient.SetClientID("leecher-peer")

	var magnet struct {
		MagnetURI string `json:"magnetUri"`
	}
	require.Equal(t, 200, seeder.do(t, "GET", "/api/v1/torrents/"+added.ID+"/magnet", nil, &magnet))

	var own struct {
		ID string `json:"id"`
	}
	addCtx, addCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer addCancel()
	require.NoError(t, leecherClient.Post(addCtx, "/api/v1/torrents",
		map[string]string{"magnetUri": magnet.MagnetURI}, &own),
		"the leecher's own backend refused the magnet")
	require.Equal(t, added.ID, own.ID)

	// The leecher selects the film the way playback always does: selection
	// steers the piece priorities, and without it nothing asks for anything.
	require.Equal(t, 200, leecher.do(t, "POST", "/api/v1/torrents/"+added.ID+"/select",
		map[string]int{"fileIndex": 0}, nil))
	waitForProgress(t, leecher, added.ID, 0.0001, 60*time.Second,
		"the leecher pulled nothing from the swarm")

	// The trailing name is mpv's, not the server's: sub-add reads the subtitle
	// format off the URL's extension, so the index alone is not fetchable.
	subURL := leecher.BaseURL + "/api/v1/torrents/" + added.ID + "/subtitles/0/film.srt"

	// Without the token the endpoint answers 401 on the real router, not on a
	// double: the gate under test is the wired one.
	resp, err := http.Get(subURL)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// With the token, the real bytes of the real torrent arrive.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subURL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Access-Token", leecher.Token)
	req.Header.Set("X-Client-ID", "leecher-peer")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, srt, string(body), "the endpoint must serve the torrent's bytes, not a placeholder")

	// And a real mpv loads them over that same authenticated fetch: the header
	// mpv was given at startup travels to the subtitle request, so no ticket
	// in the URL is needed anywhere on this road. localSilentMedia skips with
	// a reason where ffmpeg is absent, after the endpoint assertions above.
	header, err := player.FormatHeader("X-Access-Token", leecher.Token)
	require.NoError(t, err)
	// Production mpv carries the owner's identity alongside the token
	// (bug-0089): the gate keys ownership by client id, and a fetch without
	// it is user "".
	clientHeader, err := player.FormatHeader("X-Client-ID", "leecher-peer")
	require.NoError(t, err)
	p, err := player.Start(ctx, player.Options{
		Binary:     "mpv",
		SocketPath: e2eSocketPath(t, "subs-e2e"),
		ExtraArgs:  []string{"--vo=null", "--ao=null", "--idle=yes"},
	}, []string{header, clientHeader})
	require.NoError(t, err, "mpv could not start")
	defer p.Close()

	if err := p.Play(ctx, localSilentMedia(t)); err != nil {
		t.Fatalf("mpv refused the local file: %v", err)
	}
	// Play only sends loadfile: sub-add before the file opens fails with the
	// same error a missing file gives, and the two causes are indistinguishable
	// afterwards. A positive duration is the file being there.
	loadDeadline := time.Now().Add(15 * time.Second)
	for {
		duration, err := p.Duration(ctx)
		if err == nil && duration > 0 {
			break
		}
		if time.Now().After(loadDeadline) {
			t.Fatalf("mpv never opened the local file: %s", p.Log())
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err := p.AddSubtitles(ctx, subURL); err != nil {
		t.Fatalf("mpv refused the endpoint subtitles: %v\n--- what mpv said ---\n%s", err, p.Log())
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		tracks, err := p.Tracks(ctx, player.TrackSub)
		require.NoError(t, err)
		for _, tr := range tracks {
			if tr.External {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("mpv lists %+v, none of it the external subtitles", tracks)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// e2eSocketPath is a per-process IPC socket path for the mpv under test.
func e2eSocketPath(t *testing.T, label string) string {
	t.Helper()
	path := filepath.Join(os.TempDir(), fmt.Sprintf("torrsync-%s-%d.sock", label, os.Getpid()))
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

// localSilentMedia builds a tiny local mp4 for mpv to hold open while it
// fetches the subtitles over HTTP: what is under test is the fetch, not the
// film, so the film is two seconds of generated noise.
func localSilentMedia(t *testing.T) string {
	t.Helper()

	ffmpeg, err := exec.LookPath("ffmpeg")
	require.NoError(t, err)
	media := filepath.Join(t.TempDir(), "film.mp4")
	cmd := exec.Command(ffmpeg,
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=5:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-y", media,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg could not build a file to play: %v\n%s", err, output)
	}
	return media
}
