// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"context"
	"encoding/json"
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
)

// The remoting mode — a backend that has no torrent client of its own and
// asks a gateway for everything — has only ever been tested against a
// stand-in: a hand-written HTTP server that speaks what our client expects.
//
// That is the same trap the room's own tests were in, and the project has
// already paid for it once: a test that feeds the code the format the code
// expects proves the two agree with each other, not that either agrees with
// the world. The real hTorrent has never been run.
//
// This runs it: a real hTorrent gateway, a real seeder behind it, and the
// real backend in remoting mode. What is pinned here is *our* side of the
// contract — the magnet, the metadata, the file list, and the progress the
// client reports — against the real thing's own answers.
//
// The one thing this does not pin is the piece transfer: the gateway does not
// pull the film in this environment, and the reason is in the gateway's own
// client, not in ours. That is a card, not a test that cannot be green.
//
// The gateway is the htorrent binary. It is not a dependency of this project
// — the client speaks to it over HTTP and knows nothing of its library — so
// the test skips when the binary is absent, and the pipeline installs it and
// checks that the test did not skip.

// TestTheBackendTalksToARealGateway is the remoting mode, end to end.
func TestTheBackendTalksToARealGateway(t *testing.T) {
	gatewayBin := gatewayBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	announce, tracker := startLoopbackTracker(t)
	payload := swarmPayload(1 << 20)

	// The seeder: a real backend of our own, holding the real film, so the
	// gateway has something to find.
	seederData := t.TempDir()
	writeFile(t, filepath.Join(seederData, "torrents", swarmName), payload)
	seeder := startServerWith(t, "seeder", seederData, true)

	var added struct {
		ID string `json:"id"`
	}
	require.Equal(t, 201, seeder.do(t, "POST", "/api/v1/torrents",
		map[string]string{"torrentFile": encodeTorrent(t, buildSeededTorrent(t, announce, payload))}, &added),
		"the seeder could not add its own torrent")
	waitForProgress(t, seeder, added.ID, 1, 60*time.Second, "the seeder never verified the film it holds")
	require.Equal(t, 200, seeder.do(t, "POST", "/api/v1/torrents/"+added.ID+"/select",
		map[string]int{"fileIndex": 0}, nil))

	magnet := hostMagnet(t, seeder, added.ID)

	// The gateway: the real thing, with its own storage and its own
	// credentials.
	gatewayPort := freePort(t)
	gatewayUser, gatewayPass := "gateway-user", "gateway-password"
	storage := t.TempDir()
	gwLog := filepath.Join(t.TempDir(), "gateway.log")
	logFile, err := os.Create(gwLog)
	require.NoError(t, err, "gateway log: %v", err)

	gw := exec.Command(gatewayBin, "gateway",
		"--storage", storage,
		"--laddr", fmt.Sprintf("127.0.0.1:%d", gatewayPort),
		"--api-username", gatewayUser,
		"--api-password", gatewayPass,
	)
	gw.Stdout = logFile
	gw.Stderr = logFile
	require.NoError(t, gw.Start(), "start the gateway")
	t.Cleanup(func() {
		if gw.Process != nil {
			_ = gw.Process.Kill()
			_, _ = gw.Process.Wait()
		}
		_ = logFile.Close()
		if t.Failed() {
			t.Logf("gateway log (%s):\n%s", gwLog, tailFile(gwLog, 40))
		}
	})
	gatewayURL := fmt.Sprintf("http://127.0.0.1:%d", gatewayPort)
	waitForGateway(t, gatewayURL, gatewayUser, gatewayPass)

	// The backend in remoting mode: no torrent client of its own, everything
	// through the gateway.
	remoting := startServerWith(t, "remoting", t.TempDir(), false,
		"--gateway-url", gatewayURL,
		"--gateway-user", gatewayUser,
		"--gateway-pass", gatewayPass,
	)
	client := api.New(remoting.BaseURL, remoting.Token)
	client.SetClientID("remoting")

	// 1. The magnet goes in. Our backend has no client of its own, so every
	// word of the metadata came through the gateway.
	var own struct {
		ID string `json:"id"`
	}
	require.Equal(t, 201, remoting.do(t, "POST", "/api/v1/torrents",
		map[string]string{"magnetUri": magnet}, &own),
		"the remoting backend refused the magnet")

	listed := listTorrents(t, remoting)
	require.Len(t, listed, 1, "the remoting backend does not see the torrent it was given")
	assert.Equal(t, own.ID, listed[0].ID)
	assert.Equal(t, swarmName, listed[0].Name,
		"the name came from the gateway's own answer, and it is not the film's")

	// 2. The files came through the gateway too, and they are the film's.
	var listedFiles struct {
		Files []struct {
			Index int    `json:"index"`
			Name  string `json:"name"`
			Size  int64  `json:"size"`
		} `json:"files"`
	}
	require.Equal(t, 200, remoting.do(t, "GET", "/api/v1/torrents/"+own.ID+"/files", nil, &listedFiles),
		"the remoting backend could not list the files")
	require.Len(t, listedFiles.Files, 1, "the gateway did not report the film's files")
	assert.Equal(t, swarmName, listedFiles.Files[0].Name, "the gateway reported a different file")
	assert.Equal(t, int64(len(payload)), listedFiles.Files[0].Size, "the gateway reported a different size")

	// 3. And the progress the client reports is the gateway's own progress.
	// This is the part a stand-in could never prove: the client parses the
	// gateway's metrics, and the numbers it shows are the numbers the gateway
	// shows.
	waitForGatewayMetrics(t, gatewayURL, gatewayUser, gatewayPass)
	gatewayProgress := gatewayCompletedBytes(t, gatewayURL, gatewayUser, gatewayPass)
	ours := listTorrents(t, remoting)
	require.Len(t, ours, 1)
	assert.Equal(t, gatewayProgress, int64(ours[0].Progress*float64(len(payload))),
		"the client reports %0.4f of the film, the gateway says %d bytes",
		ours[0].Progress, gatewayProgress)

	_ = tracker
	_ = ctx
}

// gatewayBinary finds the htorrent binary, and skips without it.
func gatewayBinary(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("htorrent")
	if err != nil {
		t.Skip("htorrent is not installed: the real gateway cannot be run without it")
	}
	return path
}

// waitForGateway waits until the gateway answers its own API.
func waitForGateway(t *testing.T, baseURL, user, pass string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if code := gatewayCode(t, baseURL, user, pass); code == http.StatusOK {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("the gateway never answered on %s", baseURL)
}

// waitForGatewayMetrics waits until the gateway reports the torrent it was
// given. The gateway learns about it when the magnet goes in, and the metrics
// follow.
func waitForGatewayMetrics(t *testing.T, baseURL, user, pass string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if gatewayCompletedBytes(t, baseURL, user, pass) >= 0 && len(gatewayTorrents(t, baseURL, user, pass)) > 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("the gateway never reported the torrent on %s", baseURL)
}

// gatewayTorrents is the gateway's own view of what it holds.
func gatewayTorrents(t *testing.T, baseURL, user, pass string) []gatewayTorrent {
	t.Helper()
	body, err := gatewayGet(t, baseURL, user, pass, "/metrics")
	require.NoError(t, err, "read the gateway's metrics")
	var out []gatewayTorrent
	require.NoError(t, json.Unmarshal(body, &out), "decode the gateway's metrics: %s", body)
	return out
}

// gatewayTorrent is one entry of the gateway's metrics.
type gatewayTorrent struct {
	Magnet   string `json:"magnet"`
	InfoHash string `json:"infohash"`
	Peers    int    `json:"peers"`
	Files    []struct {
		Path      string `json:"path"`
		Length    int64  `json:"length"`
		Completed int64  `json:"completed"`
	} `json:"files"`
}

// gatewayCompletedBytes is how many bytes the gateway says it has.
func gatewayCompletedBytes(t *testing.T, baseURL, user, pass string) int64 {
	t.Helper()
	var total int64
	for _, torrent := range gatewayTorrents(t, baseURL, user, pass) {
		for _, f := range torrent.Files {
			total += f.Completed
		}
	}
	return total
}

// gatewayGet asks the gateway something and returns the body.
func gatewayGet(t *testing.T, baseURL, user, pass, path string) ([]byte, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+path, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(user, pass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// gatewayCode asks the gateway for a status code.
func gatewayCode(t *testing.T, baseURL, user, pass string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, baseURL+"/metrics", http.NoBody)
	if err != nil {
		return 0
	}
	req.SetBasicAuth(user, pass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}
