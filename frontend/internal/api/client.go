// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api talks to this application's own backend: torrents, files,
// selection, buffer telemetry and the media stream itself.
//
// There used to be two planes — a room server's control plane next to the data
// plane — and sending a request to the wrong one decided who downloaded the
// film. Rooms meet over WebRTC now, so the second server is gone along with the
// plane abstraction (idea-0064). What remains is one client for one backend.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client speaks to this application's own backend.
type Client struct {
	http *http.Client

	baseURL string
	token   string

	clientID string
}

// requestTimeout is the patience of one API call.
//
// It has to outlive the slowest answer the backend can give, and that answer is
// not a slow database: adding a magnet blocks until the swarm has answered with
// the metadata, which the backend allows 60 seconds for
// (backend/internal/constants/constants.go, MagnetMetadataTimeout). The window
// already asks for two minutes on that call; a shorter blanket cap here overrode
// the caller's context and turned a slow swarm into a client-side failure that
// blamed the clock (bug-0079).
//
// The number is checked in client_test.go against that constant, because no test
// can span a module boundary and the coupling is real: lower this below the
// backend's wait and the guest's first step after joining a room starts failing.
const requestTimeout = 2 * time.Minute

// New builds a client for one backend.
func New(baseURL, token string) *Client {
	return &Client{
		http:    &http.Client{Timeout: requestTimeout},
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
	}
}

// Token is the token for this application's own backend.
//
// Exposed because mpv needs it as a header: it fetches the stream itself and
// cannot ask this client to attach it (idea-0066).
func (c *Client) Token() string { return c.token }

// StreamURL builds the media URL.
//
// It is built from this backend and nowhere else: a guest that assembled it
// from the host's address would play the host's download instead of pulling
// its own copy, which is the exact thing the goal forbids.
func (c *Client) StreamURL(torrentID string) string {
	return fmt.Sprintf("%s/api/v1/torrents/%s/stream", c.baseURL, torrentID)
}

// SubtitlesURL builds the subtitle-file URL for mpv's --sub-file.
//
// Same backend as the stream — a guest that assembled it from the host's
// address would fetch another backend's files, which the ownership gate
// would refuse anyway, but the address must still never be borrowed.
//
// The file name rides along for mpv's format detection: sub-add reads the
// extension off the URL and refuses an extensionless one, so the index alone
// is not a URL mpv accepts. The server ignores the name — the index is
// authoritative.
func (c *Client) SubtitlesURL(torrentID string, fileIndex int, fileName string) string {
	return fmt.Sprintf("%s/api/v1/torrents/%s/subtitles/%d/%s",
		c.baseURL, torrentID, fileIndex, url.PathEscape(fileName))
}

// Get performs a GET and decodes the JSON body into out.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

// Post performs a POST with a JSON body.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("X-Access-Token", c.token)
	}
	if c.clientID != "" {
		// Ownership on the backend is keyed by it.
		req.Header.Set("X-Client-ID", c.clientID)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return fmt.Errorf("%s %s: read body: %w", req.Method, req.URL.Path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &APIError{Status: resp.StatusCode, Path: req.URL.Path, Body: string(raw)}
	}

	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s %s: decode: %w", req.Method, req.URL.Path, err)
	}
	return nil
}

// maxResponseSize caps what a server can make this client read into memory.
const maxResponseSize = 16 << 20

// SetClientID identifies this run to the backend.
func (c *Client) SetClientID(id string) { c.clientID = id }

// APIError is a non-2xx answer with where it came from: "the request failed"
// is not actionable, the path and status are.
type APIError struct {
	Status int
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s returned %d: %s", e.Path, e.Status, e.Body)
}
