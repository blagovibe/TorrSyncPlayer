// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package p2p

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

// These tests pin the JSON that actually reaches the SSE stream, through the
// real emit path (idea-0013). The payloads used to be map[string]interface{}
// literals at each call site; the models package pins what the new structs
// marshal to, and these pin what a client is actually sent. Together they make
// the typing change provably wire-compatible rather than merely plausible.

// awaitEvent reads from ch until an event of the given type arrives.
func awaitEvent(t *testing.T, ch chan models.P2PEvent, wantType string) models.P2PEvent {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("event channel closed while waiting for %q", wantType)
			}
			if ev.Type == wantType {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q", wantType)
		}
	}
}

func TestPeerJoinedPayloadOnTheWire(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// The real flow: the client creates the room and subscribes to SSE but
	// never calls join, so the host is NOT in room.Peers. Only guests join.
	room, err := svc.CreateRoom(ctx, "host-1", "Wire Room", "", "torrent-wire-room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	events := svc.GetEvents("host-1")
	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-wire-room"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	ev := awaitEvent(t, events, models.EventTypePeerJoined)

	got, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// The client's QJsonObject lookup keys off "peerID" exactly (see
	// RoomManager::onRoomEvent), so a silent rename here is a runtime bug.
	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("event has no object data: %s", got)
	}

	for _, key := range []string{"peerID", "userID", "isHost", "peerCount"} {
		if _, present := data[key]; !present {
			t.Errorf("peer_joined payload missing %q: %s", key, got)
		}
	}
	if data["userID"] != "guest-1" {
		t.Errorf("userID = %v, want guest-1", data["userID"])
	}
	// The host created the room and is a participant, so together with the guest
	// the count is 2. It used to read 1, because the host was outside room.Peers
	// (bug-0021).
	if data["peerCount"] != float64(2) {
		t.Errorf("peerCount = %v, want 2 (the host counts as a participant)", data["peerCount"])
	}
}

func TestPeerLeftPayloadOnTheWire(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Wire Room", "", "torrent-wire-room-2")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	// Two guests, so that one leaving does not empty the room: an empty room is
	// deleted instead, and no peer_left is emitted at all.
	for _, guest := range []string{"guest-1", "guest-2"} {
		if err := svc.JoinRoom(ctx, guest, room.ID, "", "torrent-wire-room-2"); err != nil {
			t.Fatalf("JoinRoom %s: %v", guest, err)
		}
	}

	events := svc.GetEvents("host-1")
	awaitEvent(t, events, models.EventTypePeerJoined) // drain the first join
	awaitEvent(t, events, models.EventTypePeerJoined) // and the second

	if err := svc.LeaveRoom(ctx, "guest-1"); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	ev := awaitEvent(t, events, models.EventTypePeerLeft)

	got, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := payload["data"].(map[string]any)

	if data["peerID"] == nil || data["peerID"] == "" {
		t.Errorf("peer_left payload must carry peerID: %s", got)
	}
	if data["peerCount"] != float64(2) {
		t.Errorf("peerCount = %v, want 2 (host + the one remaining guest)", data["peerCount"])
	}
	// PeerLeftEvent has no IsHost field. If a client ever needs to know, the
	// absence must be deliberate rather than an oversight in a map literal.
	if _, present := data["isHost"]; present {
		t.Errorf("peer_left must not carry isHost: %s", got)
	}
}

// A buffered room waiting on a peer that caught up is the exact scenario
// idea-0018 introduced: the payload must be able to say buffering:false, and
// must arrive over the real broadcast path.
func TestBroadcastSyncCarriesBufferingFalse(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Wire Room", "", "torrent-wire-room-3")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-wire-room-3"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	events := svc.GetEvents("guest-1")
	awaitEvent(t, events, models.EventTypePeerJoined) // drain the join notice

	svc.BroadcastSync(room.ID, models.NewSyncEvent(models.SyncActionBuffering,
		models.SyncStatus{IsPlaying: true, Duration: 3600}, "guest-1"))

	ev := awaitEvent(t, events, models.EventTypeSync)

	got, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := payload["data"].(map[string]any)

	if _, present := data["buffering"]; !present {
		t.Fatalf("buffering:false must be sent explicitly — it is the message that lifts a stalled room: %s", got)
	}
	if data["buffering"] != false {
		t.Errorf("buffering = %v, want false", data["buffering"])
	}
	if data["initiator"] != "guest-1" {
		t.Errorf("initiator = %v, want guest-1", data["initiator"])
	}
}
