// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
	"encoding/json"
	"fmt"
)

// AddSubtitles loads an external subtitle file into the running mpv.
//
// sub-add fits the reused player: the process starts once per run, while the
// film — and the subtitle file next to it — changes per playback. The URL
// points at the subtitles endpoint, and mpv fetches it with the same
// --http-header-fields the stream uses, so no credential travels in it.
//
// The reply is read rather than ignored, like SelectTrack: a file mpv cannot
// read fails here, not three minutes later when nobody is watching the log.
func (p *Player) AddSubtitles(ctx context.Context, url string) error {
	if url == "" {
		return fmt.Errorf("player: no subtitles URL given")
	}
	var reply struct {
		Error string `json:"error"`
	}
	if err := p.request(ctx, []any{"sub-add", url}, &reply); err != nil {
		return err
	}
	if reply.Error != "" && reply.Error != "success" {
		// mpv says "error running command" for two unrelated things, and the
		// reply cannot tell them apart. Proved against the real binary: a
		// sub-add sent before the file is loaded is refused with that and
		// nothing else, while a URL mpv cannot open gets the same refusal
		// plus a "Failed to open" line in its log. One sentence reaches the
		// person watching; the other does not, so ask mpv which case this is
		// rather than repeat what it already said.
		if loaded, err := p.fileLoaded(ctx); err == nil && !loaded {
			return fmt.Errorf("player: %s — no file loaded to attach subtitles to", reply.Error)
		}
		return fmt.Errorf("player: %s", reply.Error)
	}
	return nil
}

// fileLoaded reports whether mpv has a file open — a track exists, which is
// the state sub-add attaches to. An empty track-list before the first file is
// loaded is not an error, so the reply's error field is deliberately ignored.
func (p *Player) fileLoaded(ctx context.Context) (bool, error) {
	var reply struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := p.request(ctx, []any{"get_property", "track-list"}, &reply); err != nil {
		return false, err
	}
	return len(reply.Data) > 0, nil
}

// RemoveExternalSubtitles unloads every subtitle track mpv loaded from
// outside the file. Only external tracks can have come from AddSubtitles —
// the container's own stay untouched, and so does the voice. With nothing
// external loaded it sends nothing: a no-op, not an error.
func (p *Player) RemoveExternalSubtitles(ctx context.Context) error {
	tracks, err := p.Tracks(ctx, TrackSub)
	if err != nil {
		return err
	}
	for _, t := range tracks {
		if !t.External {
			continue
		}
		var reply struct {
			Error string `json:"error"`
		}
		if err := p.request(ctx, []any{"sub-remove", t.ID}, &reply); err != nil {
			return err
		}
		if reply.Error != "" && reply.Error != "success" {
			return fmt.Errorf("player: %s", reply.Error)
		}
	}
	return nil
}
