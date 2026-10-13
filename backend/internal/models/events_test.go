// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package models

import (
	"encoding/json"
	"testing"
)

// These tests pin the exact JSON that SSE clients receive. The event payloads
// used to be built as map[string]interface{} at each call site, which meant the
// wire format was only ever described by the code that happened to construct
// it. They are now structs (idea-0013); these tests are the guarantee that
// typing them did not silently change what the frontend parses.
//
// If one of these fails, the change is a PROTOCOL change, not a refactor: the
// client's QJsonObject lookups key off these exact names.

func TestPeerJoinedEventJSON(t *testing.T) {
	e := PeerJoinedEvent{
		PeerID:    "peer-1",
		UserID:    "user-1",
		IsHost:    true,
		PeerCount: 3,
	}

	got, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	const want = `{"peerID":"peer-1","userID":"user-1","isHost":true,"peerCount":3}`
	if string(got) != want {
		t.Errorf("peer_joined payload changed\n got: %s\nwant: %s", got, want)
	}
}

func TestPeerLeftEventJSON(t *testing.T) {
	e := PeerLeftEvent{
		PeerID:    "peer-2",
		UserID:    "user-2",
		PeerCount: 2,
	}

	got, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	const want = `{"peerID":"peer-2","userID":"user-2","peerCount":2}`
	if string(got) != want {
		t.Errorf("peer_left payload changed\n got: %s\nwant: %s", got, want)
	}
}

// The sync payload is the awkward one: seek carries an extra top-level
// "position" and buffering an extra top-level "buffering", on top of the same
// "status" object. That asymmetry is exactly what map literals hid. The
// frontend reads status.position first and falls back to the top level, so both
// spellings must survive.
//
// The two positions differ on purpose: the host asked for 90, smoothing moved
// the room to 30, and the payload carries both.
func TestSyncEventJSONSeekKeepsTopLevelPosition(t *testing.T) {
	e := NewSyncEvent(SyncActionSeek, SyncStatus{
		IsPlaying: true,
		Position:  30, // where the room actually is, after smoothing
		Duration:  3600,
		Timestamp: 1700000000000,
	}, "host-1").WithRequestedPosition(90)

	got, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	const want = `{"action":"seek","position":90,"status":{"isPlaying":true,"position":30,` +
		`"duration":3600,"timestamp":1700000000000,"buffering":false},"initiator":"host-1"}`
	if string(got) != want {
		t.Errorf("seek payload changed\n got: %s\nwant: %s", got, want)
	}
}

func TestSyncEventJSONBufferingKeepsTopLevelFlag(t *testing.T) {
	e := NewSyncEvent(SyncActionBuffering, SyncStatus{
		IsPlaying: true,
		Position:  10,
		Duration:  3600,
		Timestamp: 1700000000000,
		Buffering: true,
	}, "peer-9")

	got, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	const want = `{"action":"buffering","buffering":true,"status":{"isPlaying":true,"position":10,` +
		`"duration":3600,"timestamp":1700000000000,"buffering":true},"initiator":"peer-9"}`
	if string(got) != want {
		t.Errorf("buffering payload changed\n got: %s\nwant: %s", got, want)
	}
}

// play/pause carry neither extra field, so they must not invent empty ones.
func TestSyncEventJSONPlayOmitsEmptyVariants(t *testing.T) {
	e := NewSyncEvent(SyncActionPlay, SyncStatus{
		IsPlaying: true,
		Duration:  3600,
		Timestamp: 1700000000000,
	}, "host-1")

	got, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	const want = `{"action":"play","status":{"isPlaying":true,"position":0,` +
		`"duration":3600,"timestamp":1700000000000,"buffering":false},"initiator":"host-1"}`
	if string(got) != want {
		t.Errorf("play payload changed\n got: %s\nwant: %s", got, want)
	}

	// Check the TOP level only: status legitimately carries both keys of its
	// own, so a substring search would flag a correct payload.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(got, &top); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := top["position"]; ok {
		t.Error("play payload must not carry a top-level position")
	}
	if _, ok := top["buffering"]; ok {
		t.Error("play payload must not carry a top-level buffering flag")
	}
	if _, ok := top["status"]; !ok {
		t.Error("play payload must carry status")
	}
}

// A buffering report of "I caught up" is buffering:false. With omitempty on a
// plain bool it would vanish — and that message is the one that lifts a room
// stalled behind a peer that caught up (idea-0018).
func TestSyncEventJSONBufferingFalseSurvives(t *testing.T) {
	e := NewSyncEvent(SyncActionBuffering, SyncStatus{
		IsPlaying: true,
		Duration:  3600,
		Timestamp: 1700000000000,
		Buffering: false,
	}, "peer-9")

	got, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !bytesContain(got, `"buffering":false`) {
		t.Errorf("buffering:false must reach the client; got %s", got)
	}
}

func bytesContain(hay []byte, needle string) bool {
	return len(hay) > 0 && indexOf(string(hay), needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
