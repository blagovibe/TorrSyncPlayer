// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package room

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Session drives one room on the control plane: joining it, leaving it, and
// the playback commands that flow through it.
//
// Everything here talks to the server hosting the room. The media never passes
// through that server — each participant pulls it from the swarm itself.
type Session struct {
	api *http.Client
	url string

	token  string
	client string
	roomID string
	mu     sync.RWMutex
	joined bool
}

// NewSession builds a room session against a control-plane endpoint.
func NewSession(httpClient *http.Client, baseURL, token, clientID string) *Session {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Session{
		api:    httpClient,
		url:    strings.TrimRight(baseURL, "/"),
		token:  token,
		client: clientID,
	}
}

// RoomID returns the room this session is in, or an empty string.
func (s *Session) RoomID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.roomID
}

// InRoom reports whether this client is in a room.
func (s *Session) InRoom() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.joined
}

// Create makes a room bound to a torrent.
//
// The binding is not decoration: the server refuses a room with no torrent,
// because a room with nothing to watch is a room with nothing to do.
func (s *Session) Create(ctx context.Context, name, password, torrentID string) (string, error) {
	if torrentID == "" {
		return "", fmt.Errorf("room: a room must be bound to a torrent")
	}

	var created struct {
		ID string `json:"id"`
	}
	body := map[string]string{"name": name, "password": password, "torrentId": torrentID}
	if err := s.post(ctx, "/api/v1/rooms", body, &created); err != nil {
		return "", err
	}

	s.mu.Lock()
	s.roomID, s.joined = created.ID, true
	s.mu.Unlock()

	return created.ID, nil
}

// Join enters an existing room.
func (s *Session) Join(ctx context.Context, roomID, password, torrentID string) error {
	if torrentID == "" {
		return fmt.Errorf("room: a room must be bound to a torrent")
	}

	var joined struct {
		RoomID string `json:"id"`
	}
	body := map[string]string{
		"roomId":    roomID,
		"password":  password,
		"torrentId": torrentID,
	}
	if err := s.post(ctx, "/api/v1/rooms/join", body, &joined); err != nil {
		return err
	}

	s.mu.Lock()
	s.roomID, s.joined = roomID, true
	s.mu.Unlock()

	return nil
}

// Leave exits the room.
//
// Failing to leave is not fatal: the server prunes idle peers on its own, so
// the worst case is a ghost in the roster until it does.
func (s *Session) Leave(ctx context.Context) error {
	err := s.post(ctx, "/api/v1/rooms/leave", nil, nil)

	s.mu.Lock()
	s.roomID, s.joined = "", false
	s.mu.Unlock()

	return err
}

// Play asks the room to start.
func (s *Session) Play(ctx context.Context) error {
	return s.post(ctx, "/api/v1/sync/play", map[string]any{"action": "play"}, nil)
}

// Pause asks the room to stop.
func (s *Session) Pause(ctx context.Context) error {
	return s.post(ctx, "/api/v1/sync/pause", map[string]any{"action": "pause"}, nil)
}

// Seek asks the room to move to a position in seconds.
//
// The server smooths the jump, so where the room lands is not necessarily
// here. Followers take the smoothed position from the resulting sync event.
func (s *Session) Seek(ctx context.Context, seconds float64) error {
	return s.post(ctx, "/api/v1/sync/seek", map[string]any{"position": seconds}, nil)
}

// ReportBuffering tells the room that this peer is stalled, or has caught up.
//
// Any peer may report it, not only the host: otherwise a participant whose
// stream recovered could not say so, and the room would stay paused for
// everyone.
func (s *Session) ReportBuffering(ctx context.Context, buffering bool) error {
	return s.post(ctx, "/api/v1/sync/buffering", map[string]any{"buffering": buffering}, nil)
}

// Info returns the room's torrent binding, roster and host.
func (s *Session) Info(ctx context.Context) (torrentID string, peers []Peer, seq int, hostUserID string, err error) {
	raw, err := s.get(ctx, "/api/v1/rooms/info")
	if err != nil {
		return "", nil, 0, "", err
	}
	return DecodeRoomInfo(raw)
}

func (s *Session) post(ctx context.Context, path string, body, out any) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	s.authenticate(req)

	return s.do(req, out)
}

func (s *Session) get(ctx context.Context, path string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	s.authenticate(req)

	var raw json.RawMessage
	if err := s.do(req, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (s *Session) authenticate(req *http.Request) {
	if s.token != "" {
		req.Header.Set("X-Access-Token", s.token)
	}
	if s.client != "" {
		req.Header.Set("X-Client-ID", s.client)
	}
}

func (s *Session) do(req *http.Request, out any) error {
	resp, err := s.api.Do(req)
	if err != nil {
		return fmt.Errorf("room %s: %w", req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &Error{Status: resp.StatusCode, Path: req.URL.Path}
	}

	if out == nil {
		return nil
	}
	if raw, ok := out.(*json.RawMessage); ok {
		decoder := json.NewDecoder(resp.Body)
		return decoder.Decode(raw)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Error is a refusal from the room server.
type Error struct {
	Status int
	Path   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("room server: %s returned %d", e.Path, e.Status)
}
