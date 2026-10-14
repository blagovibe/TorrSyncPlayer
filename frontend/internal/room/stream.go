// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package room holds the control-plane half of a viewing room: joining,
// subscribing to the room's event stream, and following the host's playback
// commands.
//
// Everything here talks to the server hosting the room, which may belong to
// somebody else. The media itself never passes through it.
package room

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Event types carried by the room stream.
const (
	EventConnected  = "connected"
	EventPing       = "ping"
	EventPeerJoined = "peer_joined"
	EventPeerLeft   = "peer_left"
	EventPeerList   = "peer_list"
	EventSync       = "sync"
	EventTorrent    = "torrent"
	EventSignal     = "signal"
	EventError      = "error"
	EventTimeout    = "timeout"
)

// Peer is one participant in the roster.
type Peer struct {
	PeerID    string `json:"peerID"`
	UserID    string `json:"userID"`
	Username  string `json:"username,omitempty"`
	IsHost    bool   `json:"isHost"`
	Buffering bool   `json:"buffering"`
}

// SyncEvent is a playback command from the host, or a peer's buffering report.
type SyncEvent struct {
	Action    string      `json:"action"`
	Initiator string      `json:"initiator"`
	Position  float64     `json:"position"`
	Status    *SyncStatus `json:"status,omitempty"`
}

// SyncStatus is where the room actually is.
type SyncStatus struct {
	IsPlaying bool    `json:"isPlaying"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
}

// TorrentEvent is the room's torrent, handed to members so a guest can add it
// to its own backend and pull the media from the swarm itself.
type TorrentEvent struct {
	InfoHash     string `json:"infoHash"`
	MagnetURI    string `json:"magnetUri"`
	Name         string `json:"name"`
	SelectedFile int    `json:"selectedFile"`
}

// Event is one frame of the room stream.
type Event struct {
	Type string
	Data json.RawMessage
}

// Client subscribes to a room's event stream.
//
// It reconnects on its own. A dropped stream is ordinary — proxies drop idle
// connections, laptops sleep — and a client that needs a human to click
// "reconnect" is a client that will sit silently out of sync with the room.
type Client struct {
	baseURL string
	token   string
	client  string
	http    *http.Client

	onEvent func(Event)

	mu      sync.Mutex
	roomID  string
	stopped bool

	// backoff is the first reconnect delay; it doubles from there.
	backoff time.Duration
}

// Options configures a Client.
type Options struct {
	BaseURL  string
	Token    string
	ClientID string
	HTTP     *http.Client
	// Backoff is the delay before the first reconnect attempt; it doubles up
	// to MaxBackoff. Zero means a sensible default.
	Backoff    time.Duration
	MaxBackoff time.Duration
}

// New builds a room client.
func New(opts Options, onEvent func(Event)) *Client {
	backoff := opts.Backoff
	if backoff == 0 {
		backoff = 200 * time.Millisecond
	}
	maxBackoff := opts.MaxBackoff
	if maxBackoff == 0 {
		maxBackoff = 5 * time.Second
	}

	httpClient := opts.HTTP
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	return &Client{
		baseURL: strings.TrimRight(opts.BaseURL, "/"),
		token:   opts.Token,
		client:  opts.ClientID,
		http:    httpClient,
		onEvent: onEvent,
		roomID:  "",
		stopped: false,
	}
}

// Subscribe starts following the room, reconnecting until ctx ends or Stop is
// called. It returns once the first connection attempt has been made.
func (c *Client) Subscribe(ctx context.Context, roomID string) {
	c.mu.Lock()
	c.roomID = roomID
	c.stopped = false
	c.mu.Unlock()

	go c.follow(ctx, roomID, c.backoff)
}

// Stop ends following. Safe to call more than once.
func (c *Client) Stop() {
	c.mu.Lock()
	c.stopped = true
	c.mu.Unlock()
}

func (c *Client) shouldStop() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopped
}

// follow keeps a stream open, reopening it when it drops.
//
// The first attempt is immediate; afterwards the wait grows, because a room
// server that is down should not be hammered once a frame.
func (c *Client) follow(ctx context.Context, roomID string, initial time.Duration) {
	delay := time.Duration(0)

	for {
		if c.shouldStop() {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}

		if delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}

		err := c.streamOnce(ctx, roomID)
		if ctx.Err() != nil || c.shouldStop() {
			return
		}

		if err != nil {
			c.emit(Event{Type: "stream_error", Data: json.RawMessage(quote(err.Error()))})

			// A refusal is final: reconnecting to a room this client is not in
			// only produces more refusals, and the loop would run forever.
			if IsNotInRoom(err) {
				return
			}
		}

		delay = nextDelay(delay, initial)
	}
}

// nextDelay is the wait before the next reconnection attempt: the initial delay
// first, doubling after each failure up to a ceiling.
func nextDelay(current, initial time.Duration) time.Duration {
	if current <= 0 {
		return initial
	}
	next := current * 2
	if next > 5*time.Second {
		next = 5 * time.Second
	}
	return next
}

// streamOnce holds one connection open until it ends.
func (c *Client) streamOnce(ctx context.Context, roomID string) error {
	url := fmt.Sprintf("%s/api/v1/rooms/%s/events", c.baseURL, roomID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		req.Header.Set("X-Access-Token", c.token)
	}
	if c.client != "" {
		req.Header.Set("X-Client-ID", c.client)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("room stream %s: status %d", roomID, resp.StatusCode)
	}

	c.emit(Event{Type: EventConnected, Data: json.RawMessage(`{"status":"ok"}`)})
	return c.readFrames(resp.Body)
}

// readFrames parses Server-Sent Events until the body ends.
//
// The wire format is `event: <type>` then `data: <json>` then a blank line.
// Only data lines are collected; a multi-line payload is joined with newlines
// as the specification says.
func (c *Client) readFrames(body io.Reader) error {
	scanner := bufio.NewScanner(body)
	// Payloads here are small — a roster, a sync command, a magnet. The limit
	// is a guard against a runaway server filling memory, not a real budget.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	var (
		eventType string
		data      strings.Builder
		hasData   bool
	)

	flush := func() {
		if !hasData {
			eventType = ""
			data.Reset()
			return
		}
		c.emit(Event{Type: eventType, Data: json.RawMessage(data.String())})
		eventType = ""
		data.Reset()
		hasData = false
	}

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // comment / keepalive
		}

		field, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimPrefix(value, " ")

		switch field {
		case "event":
			eventType = value
		case "data":
			if hasData {
				data.WriteByte('\n')
			}
			data.WriteString(value)
			hasData = true
		}
	}
	flush()

	return scanner.Err()
}

func (c *Client) emit(e Event) {
	if c.onEvent != nil {
		c.onEvent(e)
	}
}

// ErrNotInRoom is returned when the server refuses the stream because this
// client has not joined.
var ErrNotInRoom = errors.New("not a member of the room")

// IsNotInRoom reports whether an error means the server rejected the
// subscription for membership reasons, as opposed to a transport problem.
//
// The distinction matters: a transport error is worth retrying, a refusal is
// not — reconnecting to a room you are not in only produces more refusals.
func IsNotInRoom(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotInRoom) {
		return true
	}
	return strings.Contains(err.Error(), "status 403") || strings.Contains(err.Error(), "403 Forbidden")
}

// quote makes a string safe to hand to a JSON decoder that expects RawMessage.
func quote(s string) []byte {
	encoded, err := json.Marshal(s)
	if err != nil {
		return []byte(`""`)
	}
	return encoded
}
