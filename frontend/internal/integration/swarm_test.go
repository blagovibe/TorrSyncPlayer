// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"bytes"
	"context"
	"crypto/sha1"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	wrtcsgl "github.com/pojntfx/weron/pkg/wrtcsgl"
	"github.com/stretchr/testify/require"
)

// A real swarm on loopback: a real .torrent whose pieces really hash to the
// payload, so a seeder has something verifiable to hand out and a leecher
// rejects anything else. The neighbouring builder fills "pieces" with zeroes,
// which is fine for an identity check and useless for a swarm.
const swarmName = "film.mkv"

// swarmPayload is deterministic so a mismatch is never "different random bytes".
func swarmPayload(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(i*31 + i/251)
	}
	return out
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// buildSeededTorrent makes a .torrent whose pieces really hash to the payload,
// announcing the given tracker — the only way one peer learns about the other
// on loopback. Hand-encoded on purpose: linking the torrent client into the
// frontend module for a test fixture is not worth the coupling.
func buildSeededTorrent(t *testing.T, announce string, payload []byte) []byte {
	t.Helper()

	const pieceLength = 16384
	hashes := make([]byte, 0, ((len(payload)+pieceLength-1)/pieceLength)*20)
	for off := 0; off < len(payload); off += pieceLength {
		end := off + pieceLength
		if end > len(payload) {
			end = len(payload)
		}
		sum := sha1.Sum(payload[off:end])
		hashes = append(hashes, sum[:]...)
	}

	var info bytes.Buffer
	info.WriteString("d")
	info.WriteString(encodeString("length") + encodeInt(int64(len(payload))))
	info.WriteString(encodeString("name") + encodeString(swarmName))
	info.WriteString(encodeString("piece length") + encodeInt(pieceLength))
	info.WriteString(encodeString("pieces") + encodeString(string(hashes)))
	info.WriteString("e")

	var torrent bytes.Buffer
	torrent.WriteString("d")
	torrent.WriteString(encodeString("announce") + encodeString(announce))
	torrent.WriteString(encodeString("info"))
	torrent.Write(info.Bytes())
	torrent.WriteString("e")
	return torrent.Bytes()
}

type torrentEntry struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
}

// listTorrents is one backend's view of what it holds.
func listTorrents(t *testing.T, srv *server) []torrentEntry {
	t.Helper()

	var out struct {
		Torrents []torrentEntry `json:"torrents"`
	}
	require.Equal(t, 200, srv.do(t, "GET", "/api/v1/torrents", nil, &out))
	return out.Torrents
}

// waitForProgress blocks until a torrent on srv reaches at least want.
//
// Progress rather than presence, because "the torrent is listed" and "the
// torrent is arriving" are different facts, and only the second one proves the
// swarm worked. A 429 from the poll is not a failure: the protected group
// allows one request per second, and asking faster gets told to slow down
// rather than answered.
func waitForProgress(t *testing.T, srv *server, torrentID string, want float64, within time.Duration, what string) float64 {
	t.Helper()

	deadline := time.Now().Add(within)
	for {
		for _, entry := range listTorrents(t, srv) {
			if entry.ID == torrentID && entry.Progress >= want {
				return entry.Progress
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s (torrent %s, wanted progress %.4f)", what, torrentID[:8], want)
		}
		// Just over a second: the protected group allows one request per
		// second, and a tighter loop spends its time being told 429.
		time.Sleep(1100 * time.Millisecond)
	}
}

// newTestSignaler runs a weron signaler on loopback for one test. The public
// signaler is for manual runs.
func newTestSignaler(addr string, ctx context.Context) interface {
	Close() error
} {
	signaler := wrtcsgl.NewSignaler(addr, "", "", &wrtcsgl.SignalerConfig{
		Heartbeat:            2 * time.Second,
		EphemeralCommunities: true,
	}, ctx)
	if err := signaler.Open(); err != nil {
		panic("open test signaler: " + err.Error())
	}
	return signaler
}

// waitForTCP blocks until something answers on addr.
func waitForTCP(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("nothing listening on %s", addr)
}

// startSignaler runs a weron signaler on loopback and returns its ws URL.
func startSignaler(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	signaler := newTestSignaler(addr, ctx)
	t.Cleanup(func() { _ = signaler.Close() })
	waitForTCP(t, addr)
	return "ws://" + addr
}
