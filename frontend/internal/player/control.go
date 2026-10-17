// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
	"fmt"
)

// maxVolume is what mpv accepts: 0–100, louder is a different property
// (volume-max) that the window does not offer.
const maxVolume = 100

// Volume reports the playback loudness in mpv's own units, 0–100.
func (p *Player) Volume(ctx context.Context) (float64, error) {
	var reply struct {
		Error string  `json:"error"`
		Data  float64 `json:"data"`
	}
	if err := p.request(ctx, []any{"get_property", "volume"}, &reply); err != nil {
		return 0, err
	}
	if reply.Error != "" && reply.Error != "success" {
		return 0, fmt.Errorf("player: %s", reply.Error)
	}
	return reply.Data, nil
}

// SetVolume sets the playback loudness. Out-of-range values are clamped,
// not refused: a slider that overshoots by a pixel is not an error.
func (p *Player) SetVolume(ctx context.Context, volume float64) error {
	if volume < 0 {
		volume = 0
	}
	if volume > maxVolume {
		volume = maxVolume
	}
	return p.command(ctx, [][]any{{"set_property", "volume", volume}})
}

// ToggleFullscreen flips the player window in and out of fullscreen.
func (p *Player) ToggleFullscreen(ctx context.Context) error {
	return p.command(ctx, [][]any{{"cycle", "fullscreen"}})
}
