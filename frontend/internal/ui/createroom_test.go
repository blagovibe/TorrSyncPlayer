// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
)

// Rooms have no server anymore, so they have no names to validate and no ids
// to look up: what remains of "creating" is the guard (nothing to watch, no
// room) and the invite triple the host hands over.

// TestCreatingARoomNeedsATorrent is the guard: the binding is not optional, so
// creating without one must not even be attempted.
func TestCreatingARoomNeedsATorrent(t *testing.T) {
	if canCreateRoom("") {
		t.Error("a room was allowed to be created with no torrent bound to it")
	}
	if !canCreateRoom("abc123") {
		t.Error("a torrent was selected and the room was still refused")
	}
}

// TestTheInvitationCodeOfANewRoom has the shape a host hands over: community,
// password and key in one line, generated rather than typed.
func TestTheInvitationCodeOfANewRoom(t *testing.T) {
	invite := rtc.Invite{Community: "room-abc", Password: "secret", Key: "key-123"}
	code := invitationFor(invite)
	if code != "room-abc:secret:key-123:" {
		t.Errorf("got %q", code)
	}

	parsed, err := rtc.ParseInviteCode(code)
	if err != nil {
		t.Fatalf("a code we just built does not parse: %v", err)
	}
	if parsed.Community != invite.Community || parsed.Password != invite.Password || parsed.Key != invite.Key {
		t.Errorf("round trip lost something: %+v", parsed)
	}
}
