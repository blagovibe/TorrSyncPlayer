// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package api talks to the backend.
//
// Two servers are involved, and sending a request to the wrong one is not a
// performance question — it decides who actually downloads the film.
//
// The control plane carries rooms, the roster and synchronisation commands. It
// is the server hosting the room, which may belong to somebody else.
//
// The data plane carries torrents, files, selection, buffer telemetry and the
// media stream itself. It is always this application's own backend, because
// every participant pulls the media from the swarm themselves.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Plane says which of the two servers a call belongs to.
type Plane int

const (
	// Control is rooms, roster, sync commands, SSE.
	Control Plane = iota
	// Data is torrents, files, selection, buffer telemetry, media.
	Data
)

// Client speaks to both planes of one backend installation.
type Client struct {
	http *http.Client

	dataBaseURL string
	dataToken   string

	controlBaseURL string
	controlToken   string

	clientID string
}

// New builds a client for the two planes.
func New(dataBaseURL, dataToken, controlBaseURL, controlToken string) *Client {
	return &Client{
		http:           &http.Client{Timeout: 30 * time.Second},
		dataBaseURL:    strings.TrimRight(dataBaseURL, "/"),
		dataToken:      dataToken,
		controlBaseURL: strings.TrimRight(controlBaseURL, "/"),
		controlToken:   controlToken,
	}
}

func (c *Client) base(plane Plane) string {
	if plane == Data {
		return c.dataBaseURL
	}
	return c.controlBaseURL
}

func (c *Client) token(plane Plane) string {
	if plane == Data {
		return c.dataToken
	}
	return c.controlToken
}

// DataURL is the base URL of this application's own backend.
func (c *Client) DataURL() string { return c.dataBaseURL }

// DataToken is the token for this application's own backend.
//
// Exposed because mpv needs it as a header: it fetches the stream itself and
// cannot ask this client to attach it (idea-0066).
func (c *Client) DataToken() string { return c.dataToken }

// ControlURL is the base URL of the server hosting the room.
func (c *Client) ControlURL() string { return c.controlBaseURL }

// StreamURL builds the media URL.
//
// It is built from the data plane and nowhere else: a guest that assembled it
// from the host's address would play the host's download instead of pulling
// its own copy, which is the exact thing the goal forbids.
func (c *Client) StreamURL(torrentID string) string {
	return fmt.Sprintf("%s/api/v1/torrents/%s/stream", c.dataBaseURL, torrentID)
}

// Get performs a GET and decodes the JSON body into out.
func (c *Client) Get(ctx context.Context, plane Plane, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base(plane)+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, plane, out)
}

// Post performs a POST with a JSON body.
func (c *Client) Post(ctx context.Context, plane Plane, path string, body, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base(plane)+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, plane, out)
}

// Delete performs a DELETE.
func (c *Client) Delete(ctx context.Context, plane Plane, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.base(plane)+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, plane, nil)
}

func (c *Client) do(req *http.Request, plane Plane, out any) error {
	req.Header.Set("Accept", "application/json")
	if token := c.token(plane); token != "" {
		req.Header.Set("X-Access-Token", token)
	}
	if c.clientID != "" {
		// Sent to both planes: ownership on the data plane is keyed by it, and
		// on the control plane it identifies the peer in the room.
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
		return &APIError{Status: resp.StatusCode, Plane: plane, Path: req.URL.Path, Body: string(raw)}
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

// SetClientID identifies this run to both planes.
func (c *Client) SetClientID(id string) { c.clientID = id }

// ClientID returns this run's identifier.
func (c *Client) ClientID() string { return c.clientID }

// APIError is a non-2xx answer, kept with the plane that produced it: "the
// request failed" is not actionable, "the control plane refused us" is.
type APIError struct {
	Status int
	Plane  Plane
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	plane := "data"
	if e.Plane == Control {
		plane = "control"
	}
	return fmt.Sprintf("%s plane: %s returned %d: %s", plane, e.Path, e.Status, e.Body)
}
