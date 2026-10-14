// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package adopt is the guest's half of a room: the room says what it is watching,
// and the guest takes that into its own backend and its own player.
//
// It exists as a package rather than as functions in the window because of how the
// defect looked: the field that remembered the host's file was written and read
// by nothing, and the whole path lived inside a struct built from Fyne widgets —
// so the only thing that ever exercised it was a running application, and the
// interface showed no symptom at all. The guest joined, watched a row appear in
// its list, and sat there while the host's play command arrived at a client that
// had never started a player (bug-0078).
//
// Nothing here knows about HTTP, about mpv, or about a widget. The parts that do
// arrive as parameters, which is what lets a test drive the real thing and lets a
// scenario drive it against real backends.
package adopt

import "context"

// File is one file of a torrent, as the backend lists them.
type File struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
}

// Offer is what a room has offered a guest, and what the guest has.
//
// Every field is a reason to answer "not yet", which is why they travel together:
// passed one by one, a decision with five inputs is how a field ends up written
// and never read.
type Offer struct {
	// RoomInfohash is the torrent the room is watching.
	RoomInfohash string
	// HaveTorrents are the infohashes the guest's own backend already holds.
	// A magnet only becomes a file list once a swarm has answered, so this
	// starts empty and fills in seconds later.
	HaveTorrents []string
	// Playing is what the guest is playing right now, empty when nothing plays.
	Playing string
	// HostFile is the file the host chose. Negative means it has not chosen.
	HostFile int
	// Adopted records that this offer has already been carried out.
	Adopted bool
}

// Torrent is the room's torrent, when there is one to play.
func (o Offer) Torrent() (string, bool) {
	if o.Adopted || o.HostFile < 0 || o.RoomInfohash == "" {
		return "", false
	}
	// A guest busy with something else is not joining a room; taking the player
	// away from it would be an interruption dressed up as a feature.
	if o.Playing != "" && o.Playing != o.RoomInfohash {
		return "", false
	}
	for _, id := range o.HaveTorrents {
		if id == o.RoomInfohash {
			return id, true
		}
	}
	return "", false
}

// Outcome is why carrying out an offer stopped where it did.
//
// It is reported instead of a bool because "nothing happened yet" and "something
// went wrong" need different things said to the person, and a bool would make
// them the same silence.
type Outcome int

const (
	// NotYet is the ordinary state: the room has named no file, the torrent has
	// not arrived, or the offer has already been carried out.
	NotYet Outcome = iota
	// Adopted means the host's file was handed to the player.
	Adopted
	// NoSuchFile means the room named a file this torrent does not have.
	NoSuchFile
	// Unreadable means the guest's own backend did not answer.
	Unreadable
)

// CarryOut performs an offer: read the guest's own file list, choose the file
// the host named, and hand both to play.
//
// files is a network call and play is the player and the widgets; neither can run
// in a test, and a path whose wiring cannot be exercised is exactly how
// pendingRoomFile came to be written and never read.
func CarryOut(
	ctx context.Context,
	offer Offer,
	files func(ctx context.Context, torrentID string) ([]File, error),
	play func(torrent string, listed []File, chosen File),
) (Outcome, error) {
	torrent, ok := offer.Torrent()
	if !ok {
		return NotYet, nil
	}

	listed, err := files(ctx, torrent)
	if err != nil {
		return Unreadable, err
	}

	chosen, ok := FileFor(listed, offer.HostFile)
	if !ok {
		return NoSuchFile, nil
	}

	play(torrent, listed, chosen)
	return Adopted, nil
}

// FileFor picks the file the room named out of the torrent's file list.
//
// Selection is per-peer on the server, so a guest that picks its own file watches
// a different part of the release from everyone else — and nothing about that
// looks like an error, the video simply plays. So the number the room sent is
// taken as it is, and a number the torrent does not have is refused rather than
// rounded to the nearest thing.
func FileFor(files []File, hostFile int) (File, bool) {
	if hostFile < 0 {
		return File{}, false
	}
	for _, f := range files {
		if f.Index == hostFile {
			return f, true
		}
	}
	return File{}, false
}
