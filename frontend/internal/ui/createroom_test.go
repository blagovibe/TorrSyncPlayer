// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE for full license text

package ui

import (
	"testing"
	"unicode"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/room"
)

// A room is bound to a torrent, and the server refuses a room without one. That
// left the client with no way to start a room at all: `Session.Create` existed,
// was tested, and nothing in the interface ever called it. So the person who
// installed the app could join a room but not open one — and a room id comes
// from the server, so there was nothing to join either.

// TestCreatingARoomNeedsATorrent is the first half of the guard: the binding is
// not optional, so creating without one must not even be attempted.
func TestCreatingARoomNeedsATorrent(t *testing.T) {
	if canCreateRoom("") {
		t.Error("a room was allowed to be created with no torrent bound to it")
	}
	if !canCreateRoom("abc123") {
		t.Error("a torrent was selected and the room was still refused")
	}
}

// TestARoomNameIsWhateverTheServerWillAccept is the half nobody would have
// written down.
//
// The server validates a room name as letters, digits, spaces, hyphens and
// underscores, up to 50 characters. Torrent names are none of those things:
// "Movie.2024.1080p.BluRay.x264-GROUP.mkv" and "[SubsPlease] Show - 01" are
// what people actually have, and passing one straight through is a 400 on
// creation — after the user has picked a room name's worth of everything else.
func TestARoomNameIsWhateverTheServerWillAccept(t *testing.T) {
	names := []string{
		"Movie.2024.1080p.BluRay.x264-GROUP.mkv",
		"[SubsPlease] Show - 01 [1080p]",
		"Документальный фильм (2019) [1080p]",
		"a/b\\c:d*e?f\"g<h>i|j",
		"",
		"   ",
		"....",
	}

	for _, torrentName := range names {
		got := roomNameFor(torrentName, "Room")
		if got == "" {
			t.Errorf("torrent name %q produced an empty room name", torrentName)
			continue
		}
		if !validRoomName(got) {
			t.Errorf("torrent name %q became room name %q, which the server would reject", torrentName, got)
		}
	}
}

// TestARoomNameThatIsTooLongIsShortened rather than refused: a long torrent
// name is normal, and the answer should still be a room.
func TestARoomNameThatIsTooLongIsShortened(t *testing.T) {
	long := ""
	for i := 0; i < 80; i++ {
		long += "ab "
	}

	got := roomNameFor(long, "Room")
	if !validRoomName(got) {
		t.Fatalf("a long name became %q (%d characters), which the server would reject", got, len([]rune(got)))
	}
	if got == "Room" {
		t.Error("a long but perfectly usable name fell back to the default instead of being shortened")
	}
}

// TestAnUnusableTorrentNameFallsBackToTheDefault: if stripping leaves nothing,
// something has to be sent, and it has to be a name the server accepts.
func TestAnUnusableTorrentNameFallsBackToTheDefault(t *testing.T) {
	got := roomNameFor("...", "Room")
	if got != "Room" {
		t.Errorf("got %q, want the fallback %q", got, "Room")
	}
	if !validRoomName(got) {
		t.Errorf("the fallback %q is itself invalid", got)
	}
}

// TestTheInvitationCodeOfANewRoom has the shape a host hands over: id and the
// same password they just chose, in one line.
func TestTheInvitationCodeOfANewRoom(t *testing.T) {
	code := invitationFor("0123456789abcdef0123456789abcdef", "secret")
	if code != "0123456789abcdef0123456789abcdef:secret" {
		t.Errorf("got %q", code)
	}

	parsed, err := room.ParseInviteCode(code)
	if err != nil {
		t.Fatalf("a code we just built does not parse: %v", err)
	}
	if parsed.RoomID != "0123456789abcdef0123456789abcdef" || parsed.Password != "secret" {
		t.Errorf("round trip lost something: %+v", parsed)
	}
}

// validRoomName mirrors the server's rule (validation.ValidateRoomName) without
// importing the backend module: the client is a separate module on purpose.
func validRoomName(name string) bool {
	if name == "" {
		return false
	}
	n := 0
	for _, r := range name {
		n++
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsSpace(r), r == '-', r == '_':
		default:
			return false
		}
	}
	return n <= 50
}
