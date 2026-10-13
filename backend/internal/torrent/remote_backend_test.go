// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testMagnet = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=movie"
	testHash   = "0123456789abcdef0123456789abcdef01234567"
)

// fakeGateway is a stand-in for htorrent's gateway, implementing just the three
// endpoints remoteBackend talks to, with the same basic-auth requirement.
type fakeGateway struct {
	// files is served from /info and /stream.
	files []struct {
		Path   string `json:"path"`
		Length int64  `json:"length"`
	}
	name string

	// metrics is served from /metrics as the completed byte count per file.
	metricsCompleted []int64

	// lastRange records the Range header seen on /stream.
	lastRange string
	// lastQuery records the query /stream was called with.
	lastQuery url.Values

	// requireAuth rejects requests without the expected basic auth.
	requireAuth bool

	// infoStatus, when non-zero, is returned by /info instead of 200.
	infoStatus int
}

func (g *fakeGateway) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(gatewayInfoEndpoint, func(w http.ResponseWriter, r *http.Request) {
		if !g.authorized(w, r) {
			return
		}
		if g.infoStatus != 0 {
			http.Error(w, "boom", g.infoStatus)
			return
		}
		_ = json.NewEncoder(w).Encode(struct {
			Name         string `json:"name"`
			InfoHash     string `json:"infohash"`
			Files        any    `json:"files"`
			CreationDate int64  `json:"creationDate"`
		}{g.name, testHash, g.files, 0})
	})

	mux.HandleFunc(gatewayMetricsEndpoint, func(w http.ResponseWriter, r *http.Request) {
		if !g.authorized(w, r) {
			return
		}
		out := make([]map[string]any, 0, 1)
		files := make([]map[string]any, 0, len(g.files))
		for i, f := range g.files {
			completed := int64(0)
			if i < len(g.metricsCompleted) {
				completed = g.metricsCompleted[i]
			}
			files = append(files, map[string]any{
				"path": f.Path, "length": f.Length, "completed": completed,
			})
		}
		out = append(out, map[string]any{
			"magnet": testMagnet, "infohash": testHash, "peers": 3, "files": files,
		})
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc(gatewayStreamEndpoint, func(w http.ResponseWriter, r *http.Request) {
		if !g.authorized(w, r) {
			return
		}
		g.lastQuery = r.URL.Query()
		g.lastRange = r.Header.Get("Range")

		want := ""
		if len(g.files) > 0 {
			want = g.files[0].Path
		}
		if got := r.URL.Query().Get("path"); got != want {
			http.Error(w, "no such path", http.StatusNotFound)
			return
		}

		body := "0123456789"
		// Mirror what the gateway does with http.ServeContent: a Range request
		// must come back as 206 with the requested slice.
		if rng := r.Header.Get("Range"); rng == "bytes=2-5" {
			w.Header().Set("Content-Range", "bytes 2-5/10")
			w.Header().Set("Content-Length", "4")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte(body[2:6]))
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write([]byte(body))
	})

	return mux
}

func (g *fakeGateway) authorized(w http.ResponseWriter, r *http.Request) bool {
	if !g.requireAuth {
		return true
	}
	u, p, ok := r.BasicAuth()
	if !ok || u != "user" || p != "pass" {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

// newGatewayBackendForTest wires a remoteBackend to a fresh fake gateway.
func newGatewayBackendForTest(t *testing.T, g *fakeGateway) *remoteBackend {
	t.Helper()
	srv := httptest.NewServer(g.handler())
	t.Cleanup(srv.Close)

	backend, err := NewGatewayBackend(GatewayConfig{
		URL:      srv.URL,
		Username: "user",
		Password: "pass",
	})
	require.NoError(t, err)
	return backend.(*remoteBackend)
}

func sampleFilesRemoved() {}

func TestGatewayBackend_AddMagnet(t *testing.T) {
	g := &fakeGateway{
		name:        "Big Buck Bunny",
		requireAuth: true,
		files: []struct {
			Path   string `json:"path"`
			Length int64  `json:"length"`
		}{
			{Path: "Big Buck Bunny/bbb.mp4", Length: 1000},
			{Path: "Big Buck Bunny/readme.txt", Length: 20},
		},
	}
	backend := newGatewayBackendForTest(t, g)

	handle, err := backend.AddMagnet(context.Background(), testMagnet)
	require.NoError(t, err)

	assert.Equal(t, testHash, handle.ID())
	assert.Equal(t, "Big Buck Bunny", handle.Name())
	assert.Equal(t, int64(1020), handle.TotalLength())
	assert.Equal(t, 2, handle.FileCount())
	assert.Equal(t, "Big Buck Bunny/bbb.mp4", handle.FileName(0))
	assert.Equal(t, int64(1000), handle.FileLength(0))
	assert.Equal(t, "", handle.FileName(2), "out of range index is empty, not a panic")
	assert.Equal(t, int64(0), handle.FileLength(-1))
}

// The handle must report the gateway's progress, not a hardcoded zero — the UI
// progress column reads straight from this.
func TestGatewayBackend_HandleReportsProgress(t *testing.T) {
	g := &fakeGateway{
		name:             "movie",
		metricsCompleted: []int64{400, 20},
		files: []struct {
			Path   string `json:"path"`
			Length int64  `json:"length"`
		}{
			{Path: "movie/x.mkv", Length: 1000},
			{Path: "movie/notes.txt", Length: 20},
		},
	}
	backend := newGatewayBackendForTest(t, g)

	handle, err := backend.AddMagnet(context.Background(), testMagnet)
	require.NoError(t, err)

	assert.Equal(t, int64(420), handle.BytesCompleted())
	assert.False(t, handle.Complete(), "420 of 1020 bytes is not complete")
}

// Seeking is the whole reason /stream is proxied rather than buffered: the
// player depends on 206 responses coming back through unchanged.
func TestGatewayBackend_ServeStreamForwardsRange(t *testing.T) {
	g := &fakeGateway{
		name: "movie",
		files: []struct {
			Path   string `json:"path"`
			Length int64  `json:"length"`
		}{{Path: "movie/x.mkv", Length: 1000}},
	}
	backend := newGatewayBackendForTest(t, g)

	handle, err := backend.AddMagnet(context.Background(), testMagnet)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/stream", http.NoBody)
	req.Header.Set("Range", "bytes=2-5")
	rec := httptest.NewRecorder()

	backend.ServeStream(rec, req, handle, 0)

	assert.Equal(t, http.StatusPartialContent, rec.Code)
	assert.Equal(t, "bytes=2-5", g.lastRange, "Range header must reach the gateway")
	assert.Equal(t, "2345", rec.Body.String())
	assert.Equal(t, "bytes 2-5/10", rec.Header().Get("Content-Range"),
		"gateway response headers must be mirrored to the player")
	assert.Equal(t, testMagnet, g.lastQuery.Get("magnet"))
	assert.Equal(t, "movie/x.mkv", g.lastQuery.Get("path"))
}

// The gateway matches on the exact path from /info, so an unsanitised path has
// to survive into the query string.
func TestGatewayBackend_ServeStreamRejectsBadIndex(t *testing.T) {
	g := &fakeGateway{name: "movie", files: []struct {
		Path   string `json:"path"`
		Length int64  `json:"length"`
	}{{Path: "movie/x.mkv", Length: 10}}}
	backend := newGatewayBackendForTest(t, g)

	handle, err := backend.AddMagnet(context.Background(), testMagnet)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	backend.ServeStream(rec, httptest.NewRequest(http.MethodGet, "/stream", http.NoBody), handle, 7)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// Piece priorities belong to the gateway, so the buffer must stay out of it.
func TestGatewayBackend_BufferTargetIsNil(t *testing.T) {
	g := &fakeGateway{name: "movie", files: []struct {
		Path   string `json:"path"`
		Length int64  `json:"length"`
	}{{Path: "movie/x.mkv", Length: 10}}}
	backend := newGatewayBackendForTest(t, g)

	handle, err := backend.AddMagnet(context.Background(), testMagnet)
	require.NoError(t, err)

	assert.Nil(t, handle.BufferTarget(0), "gateway owns priorities; buffer.Service must not attach")
	require.NoError(t, handle.SelectForStreaming(0), "a no-op selection is still accepted")
	assert.Error(t, handle.SelectForStreaming(9))
}

func TestGatewayBackend_AddMagnetPropagatesGatewayError(t *testing.T) {
	g := &fakeGateway{
		name:       "movie",
		infoStatus: http.StatusUnauthorized,
		files: []struct {
			Path   string `json:"path"`
			Length int64  `json:"length"`
		}{{Path: "x.mkv", Length: 1}},
	}
	backend := newGatewayBackendForTest(t, g)

	_, err := backend.AddMagnet(context.Background(), testMagnet)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

// Basic auth is the gateway's only access control, so the client must always
// present it and must surface a rejection rather than silently continuing.
func TestGatewayBackend_RejectsWhenGatewayDemandsAuth(t *testing.T) {
	g := &fakeGateway{
		name:        "movie",
		requireAuth: true,
		files: []struct {
			Path   string `json:"path"`
			Length int64  `json:"length"`
		}{{Path: "x.mkv", Length: 1}},
	}

	srv := httptest.NewServer(g.handler())
	defer srv.Close()

	backend, err := NewGatewayBackend(GatewayConfig{URL: srv.URL, Username: "wrong", Password: "creds"})
	require.NoError(t, err)

	_, err = backend.AddMagnet(context.Background(), testMagnet)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestNewGatewayBackend_Validation(t *testing.T) {
	tests := []struct {
		name string
		cfg  GatewayConfig
	}{
		{"empty url", GatewayConfig{}},
		{"missing host", GatewayConfig{URL: "https://"}},
		{"bad scheme", GatewayConfig{URL: "ftp://example.com"}},
		{"unpinned ca missing", GatewayConfig{
			URL: "https://example.com", CACertPath: filepath.Join(t.TempDir(), "nope.pem"),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewGatewayBackend(tt.cfg)
			assert.Error(t, err)
		})
	}
}

// A pinned CA must actually load, otherwise the operator's HTTPS gateway looks
// like a TLS failure with no explanation.
func TestNewGatewayBackend_AcceptsPinnedCA(t *testing.T) {
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, generateSelfSignedPEM(t), 0o600))

	backend, err := NewGatewayBackend(GatewayConfig{
		URL: "https://gateway.example.com", CACertPath: caFile,
	})
	require.NoError(t, err)
	assert.NotNil(t, backend)
}

// A garbage CA file must be reported at startup, not on the first stream.
func TestNewGatewayBackend_RejectsUnusableCA(t *testing.T) {
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, []byte("not a certificate"), 0o600))

	_, err := NewGatewayBackend(GatewayConfig{URL: "https://example.com", CACertPath: caFile})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no usable certificate")
}

// A base URL with a path prefix must not lose that prefix, and must not
// produce a double slash when the endpoint is appended.
func TestGatewayBackend_EndpointJoinsPaths(t *testing.T) {
	g := &fakeGateway{name: "movie", files: []struct {
		Path   string `json:"path"`
		Length int64  `json:"length"`
	}{{Path: "x.mkv", Length: 1}}}
	srv := httptest.NewServer(g.handler())
	defer srv.Close()

	backend, err := NewGatewayBackend(GatewayConfig{URL: srv.URL + "/gw/"})
	require.NoError(t, err)
	rb := backend.(*remoteBackend)

	joined := rb.endpoint(gatewayInfoEndpoint, url.Values{"magnet": {"m"}})
	assert.Contains(t, joined, "/gw/info?magnet=m")

	// A prefix without a trailing slash must not collapse into "/info".
	backend2, err := NewGatewayBackend(GatewayConfig{URL: srv.URL + "/gw"})
	require.NoError(t, err)
	joined2 := backend2.(*remoteBackend).endpoint(gatewayInfoEndpoint, url.Values{"magnet": {"m"}})
	assert.Contains(t, joined2, "/gw/info?magnet=m")
}

func TestGatewayBackend_DropIsLocalOnly(t *testing.T) {
	g := &fakeGateway{name: "movie", files: []struct {
		Path   string `json:"path"`
		Length int64  `json:"length"`
	}{{Path: "x.mkv", Length: 1}}}
	backend := newGatewayBackendForTest(t, g)

	// Must not panic and must not make a request the gateway cannot answer.
	backend.Drop(testHash)
	backend.Close()
}

// generateSelfSignedPEM produces a throwaway CA certificate in PEM form so the
// CA-pinning path is exercised with a real certificate rather than a stub.
func generateSelfSignedPEM(t *testing.T) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "TorrSyncPlayer test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
