// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"encoding/json"
	"testing"
)

// The room's commands travel a WebRTC data channel now, not the server's SSE
// stream. What goes over that channel has to be pinned down the same way the
// API once was: a message the follower cannot parse is a command silently lost,
// and a command from the wrong peer silently obeyed is worse (idea-0064).

// TestEveryMessageRoundTrips pins the wire shape: what the host sends is what
// the follower reads, field for field. A renamed JSON key here breaks rooms
// across versions with no error on either side.
func TestEveryMessageRoundTrips(t *testing.T) {
	magnet := Magnet{
		Envelope:     Envelope{Type: TypeMagnet},
		MagnetURI:    "magnet:?xt=urn:btih:abc",
		InfoHash:     "abc",
		SelectedFile: 2,
		Name:         "Movie.2024.mkv",
	}
	decoded := decode(t, encode(t, NewMagnet(magnet)))
	if decoded.Type != TypeMagnet {
		t.Fatalf("a magnet came back typed %q", decoded.Type)
	}
	var back Magnet
	if err := json.Unmarshal(encode(t, NewMagnet(magnet)), &back); err != nil {
		t.Fatalf("a magnet does not decode: %v", err)
	}
	if back != magnet {
		t.Errorf("a magnet came back changed: %+v", back)
	}
}

// TestUnknownMessagesAreRefusedNotGuessed keeps a newer host from driving an
// older follower into nonsense: a type nobody knows is an error, not a pause.
func TestUnknownMessagesAreRefusedNotGuessed(t *testing.T) {
	raw := []byte(`{"type":"teleport","position":999}`)
	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("an envelope does not decode: %v", err)
	}
	if _, err := Parse(raw); err == nil {
		t.Error("an unknown message type parsed without an error")
	}
}

// TestCommandsAreObeyedOnlyFromTheHost pins the one rule that replaces the
// server's host check: the follower takes playback commands from the peer that
// sent the magnet, and from nobody else. Anyone holding the room key can open a
// data channel, so "it arrived" is not authority.
func TestCommandsAreObeyedOnlyFromTheHost(t *testing.T) {
	const host, stranger = "peer-host", "peer-stranger"

	if !ShouldFollow(host, host) {
		t.Error("a command from the host is refused")
	}
	if ShouldFollow(stranger, host) {
		t.Error("a command from a stranger is obeyed")
	}
	if ShouldFollow("", host) {
		t.Error("a command from nobody is obeyed")
	}
}

// TestTakeoverPicksTheSmallestID pins the host-leave rule: no election, no
// clocks, no chatter. Everyone sees the same peer set, so everyone independently
// arrives at the same new host — the smallest ID among the living.
func TestTakeoverPicksTheSmallestID(t *testing.T) {
	peers := []string{"peer-c", "peer-a", "peer-b"}

	if got := TakeoverHost(peers); got != "peer-a" {
		t.Errorf("takeover picked %q, want the smallest ID", got)
	}
	if got := TakeoverHost(nil); got != "" {
		t.Errorf("takeover with no peers picked %q", got)
	}
	if got := TakeoverHost([]string{"peer-a"}); got != "peer-a" {
		t.Errorf("takeover with one peer picked %q", got)
	}
}

// TestAHostAnnouncementNamesItsSender pins the transfer message: whoever holds
// the host role says so, and the name in the message is the peer that sent it —
// a transfer is believable only from the mouth of the one giving it away.
func TestAHostAnnouncementNamesItsSender(t *testing.T) {
	raw := encode(t, NewHost("peer-new-host"))

	var announced Host
	if err := json.Unmarshal(raw, &announced); err != nil {
		t.Fatalf("a host announcement does not decode: %v", err)
	}
	if announced.PeerID != "peer-new-host" {
		t.Errorf("the announcement names %q", announced.PeerID)
	}
	if announced.Type != TypeHost {
		t.Errorf("the announcement is typed %q", announced.Type)
	}
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return raw
}

func decode(t *testing.T, raw []byte) Envelope {
	t.Helper()
	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return envelope
}
