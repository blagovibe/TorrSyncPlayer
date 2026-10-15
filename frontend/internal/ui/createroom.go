// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
)

// Creating a room.
//
// There is no server to ask anymore: the host generates the room's address
// itself — a community, a password and a key — and announces its own film over
// the mesh it opens. Guests learn everything from the host, never from a
// directory.

// maxRoomNameRunes is gone with the server: rooms have no names to validate,
// only the torrent's own name travelling inside the magnet announcement.

// onCreatePressed opens a room for the torrent that is selected.
//
// The room takes nothing typed: the person is watching something, and the film
// is named by the magnet announcement. It needs the magnet first, which lives
// on this client's own backend — the infohash alone does not carry one.
func (w *window) onCreatePressed() {
	w.mu.RLock()
	torrent := w.current
	w.mu.RUnlock()

	if !canCreateRoom(torrentIDOf(torrent)) {
		w.setStatus(w.tr("status.pick"))
		return
	}

	torrentID := torrent.ID

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		magnet, err := w.roomMagnet(ctx, torrentID)
		if err != nil {
			w.setStatus(w.trf("status.couldNot", w.tr("action.createRoom")) + ": " + err.Error())
			return
		}

		invite := rtc.GenerateInvite()
		room, err := w.joinMesh(invite, true)
		if err != nil {
			w.setStatus(w.trf("status.couldNot", w.tr("action.createRoom")) + ": " + err.Error())
			return
		}

		if err := room.AnnounceMagnet(ctx, magnet); err != nil {
			w.setStatus(w.trf("status.couldNot", w.tr("action.createRoom")) + ": " + err.Error())
			return
		}

		// The code lands in the field the copy button reads, so the next thing
		// a host does — send it to the people they are watching with — is one
		// click away instead of a trip through a menu.
		onMain(func() { w.inviteEntry.SetText(invite.Encode()) })

		w.setStatus(w.tr("status.created"))
	}()
}

// roomMagnet fetches the magnet URI of a torrent this client holds, from its
// own backend. The magnet travels the room so every guest adds the same
// torrent; deriving it from the infohash instead would lose the trackers and
// resolve over DHT, dropping every seeder.
func (w *window) roomMagnet(ctx context.Context, torrentID string) (rtc.Magnet, error) {
	var out struct {
		MagnetURI string `json:"magnetUri"`
		Files     []struct {
			Index int    `json:"index"`
			Name  string `json:"name"`
		} `json:"files"`
	}
	// The magnet names the torrent; the selected file is this peer's own
	// choice, remembered so the announcement names what is actually playing.
	if err := w.client.Get(ctx, api.Data, "/api/v1/torrents/"+torrentID+"/magnet", &out); err != nil {
		return rtc.Magnet{}, err
	}

	w.mu.RLock()
	selected := w.selectedFile
	name := ""
	if w.current != nil {
		name = w.current.Name
	}
	w.mu.RUnlock()

	return rtc.Magnet{
		Envelope:     rtc.Envelope{Type: rtc.TypeMagnet},
		MagnetURI:    out.MagnetURI,
		InfoHash:     torrentID,
		SelectedFile: selected,
		Name:         name,
	}, nil
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

// canCreateRoom refuses a room with nothing to watch: a room with nothing to
// watch is a room with nothing to do, and there is no server left whose refusal
// would say so later.
func canCreateRoom(torrentID string) bool {
	return torrentID != ""
}

func torrentIDOf(t *Torrent) string {
	if t == nil {
		return ""
	}
	return t.ID
}

// invitationFor is the line a host hands over: community:password:key.
func invitationFor(invite rtc.Invite) string {
	return invite.Encode()
}
