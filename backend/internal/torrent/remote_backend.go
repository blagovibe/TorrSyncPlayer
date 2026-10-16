// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/errors"
	"github.com/blagovibe/TorrSyncPlayer/backend/pkg/logger"
)

// gateway endpoints, as served by htorrent's `gateway` command.
const (
	gatewayInfoEndpoint    = "/info"
	gatewayMetricsEndpoint = "/metrics"
	gatewayStreamEndpoint  = "/stream"

	// gatewayRequestTimeout bounds metadata and metrics calls. Streaming is
	// deliberately excluded — it is long-lived by nature.
	gatewayRequestTimeout = 30 * time.Second

	// gatewayMetricsTTL bounds how stale progress may be. ListTorrents is polled
	// by the UI, so one /metrics call per TTL is enough to keep the progress
	// column moving without hammering the gateway.
	gatewayMetricsTTL = 2 * time.Second
)

// GatewayConfig describes how to reach an htorrent gateway (idea-0023).
type GatewayConfig struct {
	// URL is the gateway base URL, e.g. https://gateway.example.com.
	URL string
	// Username and Password authenticate against the gateway's basic auth.
	Username string
	Password string
	// CACertPath optionally pins a private CA, for a gateway behind a
	// self-signed or internal certificate. Empty means system roots.
	CACertPath string
	// RequestTimeout overrides gatewayRequestTimeout when non-zero.
	RequestTimeout time.Duration
}

// remoteBackend proxies every torrent operation to an htorrent gateway instead
// of running a torrent client in-process.
//
// What this buys (idea-0023): peers never learn each other's addresses, a
// trusted host can cache pieces once for the whole room, and playback works
// from networks that block peer-to-peer traffic because only HTTPS is used.
//
// What it cannot do, and why that is not worked around:
//   - Piece priorities and the buffer window are the gateway's business. The
//     handle reports no buffer target, so buffer.Service is inert here.
//   - The gateway exposes no delete endpoint, so Drop is a local forget only.
type remoteBackend struct {
	baseURL    *url.URL
	username   string
	password   string
	httpClient *http.Client
	timeout    time.Duration

	// mu guards the metrics cache below.
	mu           sync.Mutex
	metrics      map[string]gatewayFileMetrics
	metricsAt    time.Time
	metricsValid bool
}

// gatewayInfo mirrors htorrent's v1.Info.
type gatewayInfo struct {
	Name         string `json:"name"`
	InfoHash     string `json:"infohash"`
	Description  string `json:"description"`
	CreationDate int64  `json:"creationDate"`
	Files        []struct {
		Path   string `json:"path"`
		Length int64  `json:"length"`
	} `json:"files"`
}

// gatewayMetrics mirrors htorrent's v1.TorrentMetrics.
type gatewayMetrics []struct {
	Magnet   string `json:"magnet"`
	InfoHash string `json:"infohash"`
	Peers    int    `json:"peers"`
	Files    []struct {
		Path      string `json:"path"`
		Length    int64  `json:"length"`
		Completed int64  `json:"completed"`
	} `json:"files"`
}

// gatewayFileMetrics is the cached per-torrent progress and swarm for one torrent.
type gatewayFileMetrics struct {
	completed int64
	peers     int
}

// NewGatewayBackend builds a TorrentBackend backed by an htorrent gateway.
// Returns an error if the URL is unusable or a pinned CA cannot be loaded.
func NewGatewayBackend(cfg GatewayConfig) (TorrentBackend, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, errors.New(errors.ErrInvalidInput, "gateway URL is required")
	}

	baseURL, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, errors.Wrap(errors.ErrInvalidInput, "invalid gateway URL", err)
	}
	switch baseURL.Scheme {
	case "http", "https":
	default:
		return nil, errors.New(errors.ErrInvalidInput, "gateway URL scheme must be http or https")
	}
	if baseURL.Host == "" {
		return nil, errors.New(errors.ErrInvalidInput, "gateway URL has no host")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.CACertPath != "" {
		pem, err := os.ReadFile(cfg.CACertPath)
		if err != nil {
			return nil, errors.Wrap(errors.ErrInvalidInput, "read gateway CA cert", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New(errors.ErrInvalidInput, "gateway CA cert contains no usable certificate")
		}
		transport.TLSClientConfig = &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		}
	}

	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = gatewayRequestTimeout
	}

	return &remoteBackend{
		baseURL:    baseURL,
		username:   cfg.Username,
		password:   cfg.Password,
		httpClient: &http.Client{Timeout: timeout, Transport: transport},
		timeout:    timeout,
		metrics:    make(map[string]gatewayFileMetrics),
	}, nil
}

// endpoint builds an absolute URL for a gateway path with the given query.
func (b *remoteBackend) endpoint(pathName string, query url.Values) string {
	u := *b.baseURL
	u.Path = strings.TrimSuffix(b.baseURL.Path, "/") + pathName
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	} else {
		u.RawQuery = ""
	}
	return u.String()
}

func (b *remoteBackend) newRequest(ctx context.Context, method, rawURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(b.username, b.password)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// getJSON performs a GET and decodes the JSON body, turning a non-2xx status
// into an error that names the endpoint.
func (b *remoteBackend) getJSON(ctx context.Context, rawURL string, out interface{}) error {
	req, err := b.newRequest(ctx, http.MethodGet, rawURL)
	if err != nil {
		return err
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(errors.ErrUnavailable, "gateway unreachable", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// The gateway panics on bad input and may write a non-JSON body, so
		// cap how much of it is read into an error message.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return errors.New(errors.ErrUnavailable,
			fmt.Sprintf("gateway returned %s for %s: %s",
				resp.Status, rawURL, strings.TrimSpace(string(snippet))))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return errors.Wrap(errors.ErrInternal, "decode gateway response", err)
	}
	return nil
}

func (b *remoteBackend) AddMagnet(ctx context.Context, magnetURI string) (TorrentHandle, error) {
	info, err := b.fetchInfo(ctx, magnetURI)
	if err != nil {
		return nil, err
	}
	return b.handleFromInfo(magnetURI, info), nil
}

// AddTorrentFile converts the bencoded metadata to a magnet locally and then
// registers that, because the gateway only accepts magnets.
func (b *remoteBackend) AddTorrentFile(ctx context.Context, r io.Reader) (TorrentHandle, error) {
	mi, err := metainfo.Load(r)
	if err != nil {
		return nil, errors.Wrap(errors.ErrInvalidInput, "parse torrent file", err)
	}
	if _, err := mi.UnmarshalInfo(); err != nil {
		return nil, errors.Wrap(errors.ErrInvalidInput, "unmarshal torrent info", err)
	}
	magnet, err := mi.MagnetV2()
	if err != nil {
		return nil, errors.Wrap(errors.ErrInvalidInput, "failed to derive magnet", err)
	}
	return b.AddMagnet(ctx, magnet.String())
}

func (b *remoteBackend) fetchInfo(ctx context.Context, magnetURI string) (*gatewayInfo, error) {
	var info gatewayInfo
	err := b.getJSON(ctx, b.endpoint(gatewayInfoEndpoint, url.Values{"magnet": {magnetURI}}), &info)
	if err != nil {
		return nil, err
	}
	if info.InfoHash == "" {
		return nil, errors.New(errors.ErrInvalidInput, "gateway returned no infohash")
	}
	return &info, nil
}

func (b *remoteBackend) handleFromInfo(magnetURI string, info *gatewayInfo) *remoteHandle {
	files := make([]gatewayFile, len(info.Files))
	var total int64
	for i, f := range info.Files {
		files[i] = gatewayFile{name: f.Path, length: f.Length}
		total += f.Length
	}

	return &remoteHandle{
		backend:  b,
		magnet:   magnetURI,
		id:       info.InfoHash,
		name:     info.Name,
		total:    total,
		files:    files,
		complete: false,
	}
}

// Drop is a local forget.
//
// The gateway has no delete endpoint and keeps every torrent it has seen for its
// own lifetime, so the pieces stay cached there. That is a feature in remoting
// mode — a removed torrent keeps serving the rest of the room from cache — but
// it means the gateway's disk use is decided by the gateway, not by us.
func (b *remoteBackend) Drop(torrentID string) {
	logger.Info("Gateway: torrent dropped locally; gateway retains its cache",
		"torrentID", torrentID)
}

func (b *remoteBackend) Close() {
	b.httpClient.CloseIdleConnections()
}

// ServeStream proxies the gateway's own streaming endpoint, forwarding Range so
// that seeking still works end to end.
//
// The gateway matches on the exact file path, so the unsanitised path from
// /info is used here; the Service only sanitises for display.
func (b *remoteBackend) ServeStream(w http.ResponseWriter, r *http.Request, handle TorrentHandle, fileIndex int) {
	h, ok := handle.(*remoteHandle)
	if !ok {
		logger.Error("Gateway: remote backend received a foreign handle")
		http.Error(w, "Invalid torrent handle", http.StatusInternalServerError)
		return
	}
	if fileIndex < 0 || fileIndex >= len(h.files) {
		http.Error(w, "Invalid file index", http.StatusBadRequest)
		return
	}

	target := b.endpoint(gatewayStreamEndpoint, url.Values{
		"magnet": {h.magnet},
		"path":   {h.files[fileIndex].name},
	})

	// Stream with the client's context so an abandoned playback tears the
	// gateway connection down instead of leaving it dangling.
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, http.NoBody)
	if err != nil {
		http.Error(w, "Bad gateway request", http.StatusInternalServerError)
		return
	}
	req.SetBasicAuth(b.username, b.password)
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		logger.Error("Gateway: stream request failed", "torrentID", h.id, "error", err)
		http.Error(w, "Gateway stream failed", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Mirror the gateway's status (notably 206 for a partial response) and
	// headers so the player's range handling sees exactly what the gateway
	// sent, then copy the body through.
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		// The player hanging up mid-file is normal and not worth a stack.
		logger.Debug("Gateway: stream ended early", "torrentID", h.id, "error", err)
	}
}

// metricsFor returns cached per-file progress, refreshing at most once per TTL.
func (b *remoteBackend) metricsFor(ctx context.Context) map[string]gatewayFileMetrics {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.metricsValid && time.Since(b.metricsAt) < gatewayMetricsTTL {
		return b.metrics
	}

	var raw gatewayMetrics
	if err := b.getJSON(ctx, b.endpoint(gatewayMetricsEndpoint, nil), &raw); err != nil {
		logger.Warn("Gateway: metrics refresh failed, serving stale progress", "error", err)
		// Keep whatever we had rather than reporting zeroes; an empty map is
		// distinguishable from "nothing downloaded yet".
		return b.metrics
	}

	refreshed := make(map[string]gatewayFileMetrics, len(raw))
	for _, t := range raw {
		if t.InfoHash == "" {
			continue
		}
		var completed int64
		for _, f := range t.Files {
			completed += f.Completed
		}
		refreshed[t.InfoHash] = gatewayFileMetrics{completed: completed, peers: t.Peers}
	}

	b.metrics = refreshed
	b.metricsAt = time.Now()
	b.metricsValid = true
	return b.metrics
}

// ── handle ────────────────────────────────────────────────────────────────

type gatewayFile struct {
	name   string
	length int64
}

type remoteHandle struct {
	backend  *remoteBackend
	magnet   string
	id       string
	name     string
	total    int64
	files    []gatewayFile
	complete bool
}

func (h *remoteHandle) ID() string         { return h.id }
func (h *remoteHandle) Name() string       { return h.name }
func (h *remoteHandle) TotalLength() int64 { return h.total }
func (h *remoteHandle) FileCount() int     { return len(h.files) }

func (h *remoteHandle) FileName(index int) string {
	if index < 0 || index >= len(h.files) {
		return ""
	}
	return h.files[index].name
}

func (h *remoteHandle) FileLength(index int) int64 {
	if index < 0 || index >= len(h.files) {
		return 0
	}
	return h.files[index].length
}

// BytesCompleted reports cached gateway progress. It is best-effort: with no
// metrics available it reports 0, which the UI shows as "starting" rather than
// as a stalled transfer.
func (h *remoteHandle) BytesCompleted() int64 {
	ctx, cancel := context.WithTimeout(context.Background(), h.backend.timeout)
	defer cancel()

	if m, ok := h.backend.metricsFor(ctx)[h.id]; ok {
		return m.completed
	}
	return 0
}

func (h *remoteHandle) Complete() bool {
	if h.total == 0 {
		return false
	}
	return h.BytesCompleted() >= h.total
}

// ActivePeers reports the gateway's swarm count for this torrent. Same
// best-effort terms as progress: no metrics, no peers.
func (h *remoteHandle) ActivePeers() int {
	ctx, cancel := context.WithTimeout(context.Background(), h.backend.timeout)
	defer cancel()

	if m, ok := h.backend.metricsFor(ctx)[h.id]; ok {
		return m.peers
	}
	return 0
}

// SelectForStreaming is accepted and ignored: the gateway decides what to fetch
// and in what order, and offers no control over it.
func (h *remoteHandle) SelectForStreaming(fileIndex int) error {
	if fileIndex < 0 || fileIndex >= len(h.files) {
		return errors.New(errors.ErrInvalidInput, "invalid file index")
	}
	return nil
}

// BufferTarget always returns nil, which is how the Service learns that piece
// priorities belong to the gateway and that the local buffer must stay inert.
func (h *remoteHandle) BufferTarget(int) any { return nil }

var _ TorrentBackend = (*remoteBackend)(nil)
