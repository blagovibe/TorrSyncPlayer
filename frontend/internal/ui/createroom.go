// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE for full license text

package ui

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/room"
)

// Creating a room.
//
// Session.Create existed and was covered by tests, and nothing in the interface
// ever called it: the window could join a room but not open one. Since a room id
// comes from the server, that left no way to start a watch party from the app at
// all — the whole second half of the product was unreachable.

// maxRoomNameRunes is the server's limit (validation.MaxRoomNameLength). It is
// repeated here rather than imported because the client is a separate module,
// and a client that cannot validate a name offline makes a typo a round trip.
const maxRoomNameRunes = 50

// onCreatePressed opens a room for the torrent that is selected.
//
// The room takes its name from the torrent because there is nothing to type:
// the person is watching something, and its name is already on screen. It takes
// its password from the field the join form already has, so one password field
// serves both directions instead of the window collecting a credential twice.
func (w *window) onCreatePressed() {
	w.mu.RLock()
	torrent := w.current
	w.mu.RUnlock()

	if !canCreateRoom(torrentIDOf(torrent)) {
		w.setStatus(w.tr("status.pick"))
		return
	}

	password := w.roomPassword.Text
	name := roomNameFor(torrent.Name, w.tr("room.defaultName"))
	torrentID := torrent.ID

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		session := room.NewSession(nil, w.client.ControlURL(), w.client.ControlToken(), w.clientID)
		roomID, err := session.Create(ctx, name, password, torrentID)
		if err != nil {
			w.setStatus(w.trf("status.couldNot", w.tr("action.createRoom")) + ": " + err.Error())
			return
		}

		w.mu.Lock()
		w.room = roomID
		w.session = session
		w.mu.Unlock()

		w.subscribe(roomID)

		// The code lands in the field the copy button reads, so the next thing
		// a host does — send it to the people they are watching with — is one
		// click away instead of a trip through a menu.
		code := invitationFor(roomID, password)
		onMain(func() { w.inviteEntry.SetText(code) })

		w.setStatus(w.tr("status.created"))
	}()
}

// syncCreateRoom enables the control only when there is something to watch.
//
// The guard in onCreatePressed already refuses; this is the same rule with a
// visible form, so the button is not a promise the window will not keep. It has
// to be re-evaluated whenever the selection changes, which is why this is a
// method called from there rather than a line in the constructor: a window is
// built before anything is selected, and a button frozen in that state would
// stay grey forever.
func (w *window) syncCreateRoom() {
	ready := canCreateRoom(torrentIDOf(w.currentTorrent()))

	onMain(func() {
		if ready {
			w.createRoom.Enable()
			return
		}
		w.createRoom.Disable()
	})
}

func (w *window) currentTorrent() *Torrent {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.current
}

// canCreateRoom refuses a room with nothing to watch, the same way joining
// does: the server refuses it too, and a refusal from the server is a worse
// answer than one before the request.
func canCreateRoom(torrentID string) bool {
	return torrentID != ""
}

func torrentIDOf(t *Torrent) string {
	if t == nil {
		return ""
	}
	return t.ID
}

// invitationFor is the line a host hands over: "id:password".
func invitationFor(roomID, password string) string {
	return room.InviteCode{RoomID: roomID, Password: password}.Encode()
}

// roomNameFor turns a torrent name into a name the server will accept.
//
// The server takes letters, digits, spaces, hyphens and underscores, up to 50
// characters. Real torrent names are none of those things — dots for the
// extension, brackets for the release group, and so on — so passing one through
// unchanged is a 400 after the person has already chosen a torrent and a
// password. Anything outside the rule is dropped, runs of whitespace collapse,
// and the result is cut to the limit.
//
// When nothing usable survives, the fallback is used: the name is a label, and
// an empty one would be refused for a reason the person cannot act on.
func roomNameFor(torrentName, fallback string) string {
	var b strings.Builder
	lastSpace := false

	for _, r := range torrentName {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r):
			// Leading and repeated spaces are dropped rather than trimmed at
			// the end, so the cut below cannot leave a trailing one either.
			if !lastSpace && b.Len() > 0 {
				b.WriteRune(' ')
				lastSpace = true
			}
		default:
			// Dots, brackets, colons and the rest: not in the room name rule.
		}
	}

	name := strings.TrimRight(b.String(), " ")
	if name == "" {
		return fallback
	}

	if runes := []rune(name); len(runes) > maxRoomNameRunes {
		name = strings.TrimRight(string(runes[:maxRoomNameRunes]), " ")
	}
	return name
}
