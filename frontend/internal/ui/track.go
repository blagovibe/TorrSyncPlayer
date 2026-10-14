// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
)

// A film rarely arrives with one voice. mpv has always known every track in the
// file; the application asked for none of it, so a person who needed the dub or
// the subtitles had no way to reach them from here (idea-0080).

// trackOption is one row of a chooser: the label a person reads, and the id mpv
// is sent. They travel as a pair because they are two different things, and a
// chooser that derives one from the other by position breaks the moment either
// is sorted, filtered or prefixed.
type trackOption struct {
	Label string
	ID    int
	// Selected is mpv's own answer, carried through so the chooser can open on
	// it instead of guessing from the order.
	Selected bool
}

// trackOptions builds the rows for one kind of track.
//
// Subtitles get an off switch in front, because mpv spells "no subtitles" as
// track zero and a file that came with subtitles nobody asked for cannot
// otherwise be silenced. Audio gets no such row: there is no "no voice" track,
// and offering one would send mpv a zero it would refuse or misread.
//
// The off switch's words are the caller's, because a row nobody reads in their
// own language is a row nobody presses.
func trackOptions(kind player.TrackKind, tracks []player.Track, offLabel string) []trackOption {
	options := make([]trackOption, 0, len(tracks)+1)
	if kind == player.TrackSub && offLabel != "" {
		options = append(options, trackOption{Label: offLabel, ID: 0})
	}
	for _, t := range tracks {
		options = append(options, trackOption{Label: t.Label(), ID: t.ID, Selected: t.Selected})
	}
	if kind == player.TrackSub {
		// Off is selected exactly when no track is, and that is not a guess:
		// mpv reports a selected track only when one is playing.
		for _, t := range tracks {
			if t.Selected {
				options[0].Selected = false
				return options
			}
		}
		options[0].Selected = true
	}
	return options
}

// showTrackChooser says whether a kind of track is worth a chooser at all.
//
// One voice is most films, and a selector with one entry invites a person to look
// for a choice that is not there. Subtitles are the exception: one track plus the
// off switch is already a decision, so a file with a single subtitle track still
// gets the row.
func showTrackChooser(kind player.TrackKind, tracks []player.Track) bool {
	switch {
	case kind == player.TrackSub:
		return len(tracks) > 0
	default:
		return len(tracks) > 1
	}
}

// selectedOption is the id the chooser should open on: the track mpv calls
// selected, and zero when it calls none.
//
// Zero rather than the first entry, because "no default" is what a file with no
// selected track means, and opening on the first would tell the person the film
// came with the wrong dub when in fact nothing is chosen yet.
func selectedOption(options []trackOption) int {
	for _, o := range options {
		if o.Selected {
			return o.ID
		}
	}
	return 0
}

// chooseTrack sends a chooser's row to mpv by the label the person read.
//
// The label rather than the index, because the index is a property of the row
// order and the row order changes with the file; the label is what the person
// actually chose. Off is a label like any other, and it means track zero.
func (w *window) chooseTrack(kind player.TrackKind, choice string) {
	if choice == "" {
		return
	}

	w.mu.RLock()
	mpv := w.mpv
	w.mu.RUnlock()
	if mpv == nil {
		return
	}

	options := w.trackOptionsFor(kind)
	for _, o := range options {
		if o.Label != choice {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := mpv.SelectTrack(ctx, kind, o.ID); err != nil {
			// Saying which row and why: "the player refused" leaves a person
			// clicking a chooser that does nothing, which is the same as having
			// no chooser.
			w.setStatus(w.trf("status.trackFailed", choice+": "+err.Error()))
			return
		}
		w.setStatus(w.trf("status.trackChosen", choice))
		return
	}
}

// trackOptionsFor is the chooser's current content for one kind of track.
func (w *window) trackOptionsFor(kind player.TrackKind) []trackOption {
	w.mu.RLock()
	defer w.mu.RUnlock()

	switch kind {
	case player.TrackSub:
		return trackOptions(kind, w.subTracks, w.tr("track.off"))
	default:
		return trackOptions(kind, w.audioTracks, "")
	}
}

// optionLabels is what a chooser shows.
//
// `widget.Select` identifies a row by its text while everything else here
// identifies a track by mpv's number, so the pair has to be translated both ways
// and the translation has to be the same one; see labelFor.
func optionLabels(kind player.TrackKind, tracks []player.Track, offLabel string) []string {
	options := trackOptions(kind, tracks, offLabel)
	labels := make([]string, 0, len(options))
	for _, o := range options {
		labels = append(labels, o.Label)
	}
	return labels
}

// labelFor is the inverse of optionLabels: the words shown for a track id, and
// empty when that id is not on offer right now.
func labelFor(kind player.TrackKind, tracks []player.Track, offLabel string, id int) string {
	for _, o := range trackOptions(kind, tracks, offLabel) {
		if o.ID == id {
			return o.Label
		}
	}
	return ""
}

// selectedTrackLabel is the row the chooser opens on: the track mpv calls
// selected, or nothing when it calls none. Opening anywhere else tells the person
// the film came with a dub it did not come with.
func (w *window) selectedTrackLabel(kind player.TrackKind) string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	switch kind {
	case player.TrackSub:
		return labelFor(kind, w.subTracks, w.tr("track.off"), selectedOption(trackOptions(kind, w.subTracks, w.tr("track.off"))))
	default:
		return labelFor(kind, w.audioTracks, "", selectedOption(trackOptions(kind, w.audioTracks, "")))
	}
}
