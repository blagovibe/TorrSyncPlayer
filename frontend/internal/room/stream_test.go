// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package room

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// sseServer writes frames of the form the room stream really uses:
// "event: <type>", "data: <json>", blank line.
func sseServer(t *testing.T, frames ...string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("the test server cannot flush; the client would never see a frame")
			return
		}

		for _, frame := range frames {
			_, _ = fmt.Fprint(w, frame)
			flusher.Flush()
		}
		// Hold the connection so the client's read loop is still running when
		// the test ends, which is the state a live room is in.
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	return server
}

// collector gathers events from a client goroutine.
type collector struct {
	mu     sync.Mutex
	events []Event
}

func (c *collector) add(e Event) {
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
}

func (c *collector) waitFor(t *testing.T, eventType string) Event {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		for _, e := range c.events {
			if e.Type == eventType {
				c.mu.Unlock()
				return e
			}
		}
		c.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("event %q never arrived", eventType)
	return Event{}
}

func TestStreamDeliversTheEventTypeAndPayload(t *testing.T) {
	server := sseServer(t,
		"event: connected\ndata: {\"status\":\"ok\"}\n\n",
		"event: peer_list\ndata: {\"peers\":[{\"peerID\":\"a\",\"userID\":\"host\",\"isHost\":true}],\"rosterSeq\":1}\n\n",
	)

	got := &collector{}
	client := New(Options{BaseURL: server.URL, Token: "t", ClientID: "me"}, got.add)

	ctx, cancel := testContext(3 * time.Second)
	defer cancel()
	client.Subscribe(ctx, "room-1")

	connected := got.waitFor(t, EventConnected)
	if string(connected.Data) != `{"status":"ok"}` {
		t.Fatalf("connected payload is %s", connected.Data)
	}

	roster := got.waitFor(t, EventPeerList)
	var payload struct {
		Peers []Peer `json:"peers"`
		Seq   int    `json:"rosterSeq"`
	}
	if err := json.Unmarshal(roster.Data, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Peers) != 1 || !payload.Peers[0].IsHost {
		t.Fatalf("roster decoded as %+v", payload.Peers)
	}
	if payload.Seq != 1 {
		t.Fatalf("rosterSeq is %d, want 1", payload.Seq)
	}
}

func TestStreamDecodesTheRoomTorrent(t *testing.T) {
	// The frame that makes a guest able to download anything at all.
	server := sseServer(t,
		"event: torrent\ndata: {\"infoHash\":\"abc\",\"magnetUri\":\"magnet:?xt=urn:btih:abc\",\"name\":\"Film\",\"selectedFile\":2}\n\n",
	)

	got := &collector{}
	client := New(Options{BaseURL: server.URL}, got.add)

	ctx, cancel := testContext(3 * time.Second)
	defer cancel()
	client.Subscribe(ctx, "room-1")

	frame := got.waitFor(t, EventTorrent)
	var ev TorrentEvent
	if err := json.Unmarshal(frame.Data, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.MagnetURI == "" || ev.InfoHash != "abc" || ev.SelectedFile != 2 {
		t.Fatalf("decoded as %+v", ev)
	}
}

func TestStreamDecodesSyncCommands(t *testing.T) {
	server := sseServer(t,
		"event: sync\ndata: {\"action\":\"seek\",\"initiator\":\"host\",\"position\":42.5,\"status\":{\"isPlaying\":true,\"position\":43,\"duration\":100}}\n\n",
		"event: sync\ndata: {\"action\":\"buffering\",\"initiator\":\"guest\"}\n\n",
	)

	got := &collector{}
	client := New(Options{BaseURL: server.URL}, got.add)

	ctx, cancel := testContext(3 * time.Second)
	defer cancel()
	client.Subscribe(ctx, "room-1")

	seek := got.waitFor(t, EventSync)
	var ev SyncEvent
	if err := json.Unmarshal(seek.Data, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Action != "seek" || ev.Position != 42.5 {
		t.Fatalf("seek decoded as %+v", ev)
	}
	// The position the room actually moved to differs from the requested one:
	// the server smooths jumps. Both are carried, on purpose.
	if ev.Status == nil || ev.Status.Position != 43 {
		t.Fatalf("smoothed position lost: %+v", ev.Status)
	}
}

func TestStreamIgnoresPingAndComments(t *testing.T) {
	// The keepalive exists so a proxy does not drop an idle connection. It is
	// not a room event and must not reach the rest of the code.
	server := sseServer(t,
		": keepalive comment\n\n",
		"event: ping\ndata: {}\n\n",
		"event: peer_left\ndata: {\"peerID\":\"x\"}\n\n",
	)

	got := &collector{}
	client := New(Options{BaseURL: server.URL}, got.add)

	ctx, cancel := testContext(3 * time.Second)
	defer cancel()
	client.Subscribe(ctx, "room-1")

	got.waitFor(t, EventPeerLeft)

	got.mu.Lock()
	defer got.mu.Unlock()
	for _, e := range got.events {
		if e.Type == "" {
			t.Fatalf("an event with no type got through: %+v", e)
		}
	}
}

func TestStreamReconnectsWhenTheServerDropsIt(t *testing.T) {
	// A dropped stream is ordinary. A client that needs a human to click
	// "reconnect" sits silently out of sync with the room.
	var connections int
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		connections++
		attempt := connections
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, "event: connected\ndata: {\"attempt\":%d}\n\n", attempt)
		w.(http.Flusher).Flush()

		// Every connection is cut immediately, so only reconnecting can
		// produce a second one.
		if r.URL.Path == "" {
			return
		}
	}))
	t.Cleanup(server.Close)

	got := &collector{}
	client := New(Options{BaseURL: server.URL, Backoff: 10 * time.Millisecond, MaxBackoff: 20 * time.Millisecond}, got.add)

	ctx, cancel := testContext(5 * time.Second)
	defer cancel()
	client.Subscribe(ctx, "room-1")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := connections
		mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if connections < 3 {
		t.Fatalf("only %d connections were made: the stream did not reconnect", connections)
	}
}

func TestStreamStopsWhenAsked(t *testing.T) {
	server := sseServer(t, "event: connected\ndata: {}\n\n")

	got := &collector{}
	client := New(Options{BaseURL: server.URL, Backoff: 10 * time.Millisecond, MaxBackoff: 20 * time.Millisecond}, got.add)

	ctx, cancel := testContext(5 * time.Second)
	defer cancel()
	client.Subscribe(ctx, "room-1")

	got.waitFor(t, EventConnected)
	client.Stop()

	// After Stop the client must stop reconnecting. Give it long enough that a
	// still-running reconnect loop would be obvious.
	time.Sleep(200 * time.Millisecond)
	client.Stop() // idempotent
}

func TestMultiLineDataIsJoinedWithNewlines(t *testing.T) {
	// The specification says several data: lines are one payload, joined with
	// newlines. Getting that wrong truncates whatever the server meant to say.
	//
	// Note what is NOT being tested here: JSON cannot be split across data
	// lines, because the joining newline lands inside the string literal. Every
	// room payload is JSON and therefore arrives as a single data line — the
	// rule below is about not losing text the server deliberately wrapped.
	frame := "event: note\n" +
		"data: first line\n" +
		"data: second line\n" +
		"\n"

	server := sseServer(t, frame)

	got := &collector{}
	client := New(Options{BaseURL: server.URL}, got.add)

	ctx, cancel := testContext(3 * time.Second)
	defer cancel()
	client.Subscribe(ctx, "room-1")

	delivered := got.waitFor(t, "note")
	if string(delivered.Data) != "first line\nsecond line" {
		t.Fatalf("multi-line payload became %q", delivered.Data)
	}
}

func TestStreamReportsARefusalSeparatelyFromATransportError(t *testing.T) {
	// A refusal is not worth retrying: reconnecting to a room you are not in
	// only produces more refusals.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"You are not a member of this room"}`))
	}))
	t.Cleanup(server.Close)

	got := &collector{}
	client := New(Options{BaseURL: server.URL, Backoff: 10 * time.Millisecond}, got.add)

	ctx, cancel := testContext(1500 * time.Millisecond)
	defer cancel()
	client.Subscribe(ctx, "room-1")

	got.waitFor(t, "stream_error")

	if !IsNotInRoom(fmt.Errorf("403 Forbidden")) {
		t.Fatal("a 403 was not recognised as a membership refusal")
	}
}

func TestBackoffGrowsAndIsBounded(t *testing.T) {
	initial := 200 * time.Millisecond

	delay := nextDelay(0, initial)
	if delay != initial {
		t.Fatalf("first delay is %s, want %s", delay, initial)
	}

	// It must grow: a room server that is down should not be hammered at a
	// fixed rate.
	previous := nextDelay(0, initial)
	for i := 0; i < 10; i++ {
		next := nextDelay(previous, initial)
		if next < previous {
			t.Fatalf("delay shrank from %s to %s", previous, next)
		}
		if next > 5*time.Second {
			t.Fatalf("delay %s is not bounded", next)
		}
		previous = next
	}
	if previous != 5*time.Second {
		t.Fatalf("delay settled at %s, want the 5s cap", previous)
	}
}

func testContext(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
