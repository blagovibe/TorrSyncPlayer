// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
)

// window is only needed for the catalogue lookups below, and it is the same
// zero value the other tests use.
var w = &window{}

// A film rarely arrives with one voice, and the second one is often the only one
// a person can watch: a rip with a Russian dub and the original, an anime with
// subtitles muxed in, a remux with a commentary track. mpv has always known all
// of them; the application asked for none of it, so a person who needed the dub
// had no way to reach it from the app (idea-0080).

// TestTrackOptionsCarryTheIdBehindTheLabel is the whole reason the chooser is a
// list of pairs rather than a list of strings.
//
// The label is what a person reads, and the id is what mpv is sent. Sorting,
// prefixing, or numbering either of them to look nicer makes them stop matching,
// and the failure is silent: a person picks "Russian" and hears Japanese.
func TestTrackOptionsCarryTheIdBehindTheLabel(t *testing.T) {
	tracks := []player.Track{
		{ID: 1, Lang: "jpn", Title: "Japanese"},
		{ID: 2, Lang: "rus", Title: "Russian"},
	}

	options := trackOptions(player.TrackAudio, tracks, "")
	if len(options) != 2 {
		t.Fatalf("got %d options, wanted one per track", len(options))
	}
	if options[0].Label != "Japanese (jpn)" || options[0].ID != 1 {
		t.Errorf("the first option is %+v", options[0])
	}
	if options[1].Label != "Russian (rus)" || options[1].ID != 2 {
		t.Errorf("the second option is %+v", options[1])
	}
}

// TestSubtitlesComeWithAnOffSwitch is the one option mpv spells as a number.
//
// Zero is "no subtitles", and a chooser without it forces a person to keep
// subtitles they did not ask for — burnt-in ones are not the case here, a
// muxed track plays over the film whether anybody wanted it.
func TestSubtitlesComeWithAnOffSwitch(t *testing.T) {
	// Selected, because that is what "a subtitle track is playing" looks like on
	// the wire, and without it the file is in the state where nothing is chosen.
	tracks := []player.Track{{ID: 4, Lang: "eng", Title: "English", Selected: true}}

	options := trackOptions(player.TrackSub, tracks, "Off")
	if len(options) != 2 {
		t.Fatalf("got %d options, wanted the track and an off switch", len(options))
	}
	if options[0].ID != 0 || options[0].Label != "Off" {
		t.Errorf("the first subtitle option is %+v, wanted the off switch", options[0])
	}
	if options[1].ID != 4 {
		t.Errorf("the subtitle itself is %+v", options[1])
	}

	// Off is not selected while a subtitle track is playing: the chooser has to
	// open on the track that is actually on.
	if options[0].Selected {
		t.Error("the off switch opened selected while a subtitle track is playing")
	}

	// Audio has no such switch: there is no "no voice" track, and offering one
	// would send mpv a zero it would refuse or misread.
	audio := trackOptions(player.TrackAudio, tracks, "")
	if len(audio) != 1 || audio[0].ID != 4 {
		t.Errorf("the audio options are %+v", audio)
	}
}

// TestAFileWithOneVoiceGetsNoChooser is what keeps the row from being noise.
//
// Most films have one audio track. A selector with one entry invites a person to
// look for a choice that is not there, and a selector with only "Off" invites
// them to turn off subtitles they never had.
func TestAFileWithOneVoiceGetsNoChooser(t *testing.T) {
	oneVoice := []player.Track{{ID: 1, Lang: "jpn"}}

	if showTrackChooser(player.TrackAudio, oneVoice) {
		t.Error("a file with one audio track was offered a chooser")
	}
	if showTrackChooser(player.TrackSub, nil) {
		t.Error("a file with no subtitle tracks was offered a chooser")
	}
	if !showTrackChooser(player.TrackAudio, append(oneVoice, player.Track{ID: 2})) {
		t.Error("a file with two audio tracks was not offered a chooser")
	}
	if !showTrackChooser(player.TrackSub, []player.Track{{ID: 1}}) {
		t.Error("a file with one subtitle track was not offered a chooser")
	}
}

// TestTheSelectedTrackIsWhereTheChooserStarts keeps the chooser from lying about
// what is playing.
//
// mpv says which track is selected, and the row has to open on that one; opening
// on the first makes the person believe the film came with the wrong dub.
func TestTheSelectedTrackIsWhereTheChooserStarts(t *testing.T) {
	tracks := []player.Track{
		{ID: 1, Lang: "jpn"},
		{ID: 2, Lang: "rus", Selected: true},
	}

	options := trackOptions(player.TrackSub, tracks, "Off")
	if got := selectedOption(options); got != 2 {
		t.Errorf("the chooser opened on track %d, mpv says the selected one is 2", got)
	}

	// With nothing selected — which is how a file with no default comes back —
	// zero is the honest answer rather than the first entry: nothing is playing,
	// and for subtitles that means the off switch.
	plain := trackOptions(player.TrackSub, []player.Track{{ID: 1}, {ID: 2}}, "Off")
	if got := selectedOption(plain); got != 0 {
		t.Errorf("the chooser opened on track %d for a file with no selection", got)
	}
	if !plain[0].Selected {
		t.Error("the off switch was not selected when no subtitle track is playing")
	}
}
