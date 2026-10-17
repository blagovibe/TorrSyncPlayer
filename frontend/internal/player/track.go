// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
	"fmt"
)

// TrackKind is which kind of track is being talked about. mpv carries audio,
// video and subtitle tracks in one list, and switches them through two
// differently-named properties.
type TrackKind string

const (
	// TrackAudio is a voice: the dub, the commentary, the original.
	TrackAudio TrackKind = "audio"
	// TrackSub is a subtitle track. Zero means "none", which mpv spells as no
	// track at all.
	TrackSub TrackKind = "sub"
	// TrackVideo is the picture. Nothing here switches it, so it exists for
	// one question: is mpv past loading? A video track is the proof, since a
	// file being demuxed has none yet and mpv refuses commands that need one
	// without saying why.
	TrackVideo TrackKind = "video"
)

// Track is one track of the file being played.
//
// Everything here is optional in the file: a remux carries a title and no
// language, a rip carries a language and no title, and a track with neither is
// only a number. That is why the naming lives in Label() rather than in the
// caller's format string.
//
// External marks tracks mpv loaded from outside the file — sub-add, not the
// container. Only those can have come from this application, so only those
// are ever removed by it.
type Track struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Lang     string `json:"lang"`
	Codec    string `json:"codec"`
	Selected bool   `json:"selected"`
	Default  bool   `json:"default"`
	External bool   `json:"external"`
}

// Label is how this track is shown to a person.
//
// It falls back in this order: title and language, title alone, language alone,
// and finally the number mpv gave it. The last one is not a defeat: a track with
// neither is a real thing in real files, and "Track 3" is a choice a person can
// act on while " ( )" is not.
func (t Track) Label() string {
	switch {
	case t.Title != "" && t.Lang != "":
		return fmt.Sprintf("%s (%s)", t.Title, t.Lang)
	case t.Title != "":
		return t.Title
	case t.Lang != "":
		return t.Lang
	default:
		return fmt.Sprintf("Track %d", t.ID)
	}
}

// Tracks lists the tracks of one kind in the file being played.
//
// It asks mpv once and filters, because mpv's track-list is heterogeneous — video
// tracks, chapter markers and audio all in one array — and a chooser showing all
// of them would offer a video track as a language.
func (p *Player) Tracks(ctx context.Context, kind TrackKind) ([]Track, error) {
	var reply struct {
		Error string `json:"error"`
		Data  []struct {
			ID       int    `json:"id"`
			Type     string `json:"type"`
			Title    string `json:"title"`
			Lang     string `json:"lang"`
			Codec    string `json:"codec"`
			Selected bool   `json:"selected"`
			Default  bool   `json:"default"`
			External bool   `json:"external"`
		} `json:"data"`
	}
	if err := p.request(ctx, []any{"get_property", "track-list"}, &reply); err != nil {
		return nil, err
	}
	// "success" is what mpv answers when there is nothing to report, and an
	// empty string means the same. Anything else is a real answer to a question
	// that had no answer — most often "property not found" before a file is
	// loaded.
	if reply.Error != "" && reply.Error != "success" {
		return nil, fmt.Errorf("player: %s", reply.Error)
	}

	tracks := make([]Track, 0, len(reply.Data))
	for _, t := range reply.Data {
		if t.Type != string(kind) {
			continue
		}
		tracks = append(tracks, Track{
			ID:       t.ID,
			Type:     t.Type,
			Title:    t.Title,
			Lang:     t.Lang,
			Codec:    t.Codec,
			Selected: t.Selected,
			Default:  t.Default,
			External: t.External,
		})
	}
	return tracks, nil
}

// SelectTrack switches to a track by the id mpv gave it.
//
// The property is `audio` or `sub`, not `aid`/`sid`: those are readable aliases
// that look settable and silently do nothing, which is the worst kind of
// non-feature — nothing fails and nothing changes.
//
// The reply is read rather than ignored, because this is the one command whose
// failure a person can only discover by watching the film not change.
func (p *Player) SelectTrack(ctx context.Context, kind TrackKind, id int) error {
	var reply struct {
		Error string `json:"error"`
	}
	if err := p.request(ctx, []any{"set_property", string(kind), id}, &reply); err != nil {
		return err
	}
	if reply.Error != "" && reply.Error != "success" {
		return fmt.Errorf("player: %s", reply.Error)
	}
	return nil
}
