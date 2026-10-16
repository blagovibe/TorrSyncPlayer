// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
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
		return fmt.Errorf("player: %s", reply.Error)
	}
	return nil
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
