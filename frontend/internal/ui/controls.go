// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"fmt"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
)

// seekDelay is the quiet time after the last slider move before the window
// commits the seek. Dragging fires dozens of changes a second, and executing
// each one would spam the room with seeks while the host is still aiming —
// the room would converge on every intermediate stop instead of the landing.
const seekDelay = 400 * time.Millisecond

// seekReady decides whether a pending slider target may be committed. No
// target, no seek; a fresh touch is still a drag in progress.
func seekReady(hasTarget bool, touched, now time.Time) bool {
	return hasTarget && !now.Before(touched.Add(seekDelay))
}

// formatClock renders seconds for the seeker: m:ss under an hour, h:mm:ss
// above it. Garbage in reads as zero, not as a crash and not as a blank —
// a missing duration is ordinary while the file header is still arriving.
func formatClock(seconds float64) string {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		seconds = 0
	}
	total := int(seconds)
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// setSliderValue moves a slider without firing its handler: the poll redraws
// positions every tick, and a redraw that fires OnChanged would seek (or
// re-set the volume) on the poll's behalf. The window already detaches
// handlers around programmatic chooser moves for the same reason.
func setSliderValue(s *widget.Slider, onChange func(float64), value float64) {
	s.OnChanged = nil
	s.SetValue(value)
	s.OnChanged = onChange
}

// setHeroIdle swaps the hero between nothing-yet and this-film: the
// invitation appears exactly when there is something to invite into, and the
// caption only ever sits above a name, never above an empty room.
func (w *window) setHeroIdle(idle bool) {
	if idle {
		w.heroCaption.Hide()
		w.heroLbl.SetText(w.tr("playback.untitled"))
		w.heroAdd.Show()
		return
	}
	w.heroCaption.Show()
	w.heroAdd.Hide()
}

// onSeekMoved records where the person dragged, not a seek: the tick commits
// it after the quiet delay, so a drag lands once instead of spraying.
func (w *window) onSeekMoved(value float64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seekPos = value
	w.hasSeek = true
	w.seekAt = time.Now()
}

// onVolumeMoved applies the loudness at once: volume is local — the room
// never hears it — so there is nothing to debounce against.
func (w *window) onVolumeMoved(value float64) {
	w.mu.RLock()
	mpv := w.mpv
	w.mu.RUnlock()
	if mpv == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mpv.SetVolume(ctx, value)
	}()
}

// onPlayPressed toggles the local player. Host or guest alike: this drives
// mpv, and the room decides what a local pause means from there — the host
// broadcasts it, the guest's heartbeat pulls it back.
func (w *window) onPlayPressed() {
	w.mu.RLock()
	mpv := w.mpv
	paused := w.ctlPaused
	w.mu.RUnlock()
	if mpv == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if paused {
			_ = mpv.Resume(ctx)
		} else {
			_ = mpv.Pause(ctx)
		}
	}()
}

// onFullscreenPressed flips the player window. Purely local chrome: the room
// syncs what is playing, never how anyone's window frames it.
func (w *window) onFullscreenPressed() {
	w.mu.RLock()
	mpv := w.mpv
	w.mu.RUnlock()
	if mpv == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mpv.ToggleFullscreen(ctx)
	}()
}

// updateControls reconciles the row with the player on every watch tick:
// the committed seek goes out, positions and loudness come in. Everything
// widget-bound funnels through onMain; every decision reads testable state.
func (w *window) updateControls(mpv *player.Player) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.mu.Lock()
	hasSeek := w.hasSeek
	seekPos := w.seekPos
	seekAt := w.seekAt
	w.mu.Unlock()

	if seekReady(hasSeek, seekAt, time.Now()) {
		_ = mpv.Seek(ctx, seekPos)
		w.mu.Lock()
		w.hasSeek = false
		w.mu.Unlock()
		hasSeek = false
	}

	paused, pausedErr := mpv.Paused(ctx)
	position, positionErr := mpv.Position(ctx)
	duration, durationErr := mpv.Duration(ctx)
	volume, volumeErr := mpv.Volume(ctx)

	w.mu.RLock()
	name, size := "", int64(0)
	peers := 0
	if w.current != nil {
		name, size = w.current.Name, w.current.Size
		for _, t := range w.torrents {
			if t.ID == w.current.ID {
				peers = t.Peers
			}
		}
	}
	userHolding := hasSeek
	w.mu.RUnlock()

	onMain(func() {
		if pausedErr == nil {
			w.mu.Lock()
			w.ctlPaused = paused
			w.mu.Unlock()
			if paused {
				w.playBtn.SetIcon(torrPlayIcon)
			} else {
				w.playBtn.SetIcon(torrPauseIcon)
			}
		}
		if positionErr == nil && durationErr == nil && duration > 0 {
			if w.seekSlider.Max != duration {
				w.seekSlider.Max = duration
			}
			if !userHolding {
				setSliderValue(w.seekSlider, w.onSeekMoved, clamp(position, 0, duration))
			}
			w.elapsedLbl.Text = formatClock(position)
			w.elapsedLbl.Refresh()
			w.durationLbl.Text = formatClock(duration)
			w.durationLbl.Refresh()
		}
		if volumeErr == nil {
			setSliderValue(w.volSlider, w.onVolumeMoved, clamp(volume, 0, 100))
		}
		if name != w.ctlHeroName {
			w.ctlHeroName = name
			w.setHeroIdle(name == "")
			if name != "" {
				w.heroLbl.SetText(name)
				w.setHeroArt(name)
			} else {
				w.setHeroArt(heroArtSeed)
			}
		}
		if duration > 0 {
			w.ctlDuration = duration
		}
		chipKey := fmt.Sprintf("%s|%d|%d|%d", name, size, int(w.ctlDuration), peers)
		if chipKey != w.ctlChipKey {
			w.ctlChipKey = chipKey
			w.rebuildHeroChips()
		}
		w.heroSubLbl.Text = heroSubtitle(w.tr, name, size, duration)
		w.heroSubLbl.Refresh()
	})
}

// heroSubtitle is the answer to "what am I watching": the name as soon as
// the window knows it, the full line once the file's own numbers arrive, or
// the idle invitation before anything is picked.
func heroSubtitle(tr func(string) string, name string, size int64, duration float64) string {
	if name == "" {
		return tr("playback.idle")
	}
	if size > 0 && duration > 0 {
		return fmt.Sprintf("%s · %s", humanSize(size), formatClock(duration))
	}
	if duration > 0 {
		return formatClock(duration)
	}
	return ""
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// setControlsOn flips the row with player presence: called every tick, but
// the widgets are touched only on change — not five hundred times a minute.
func (w *window) setControlsOn(on bool) {
	w.mu.Lock()
	changed := w.ctlOn != on
	w.ctlOn = on
	w.mu.Unlock()
	if changed {
		onMain(func() { w.setControlsEnabled(on) })
	}
}

// setControlsEnabled greys the row with no player behind it: buttons that
// can only fail are worse than disabled ones, and the row is visible from
// the first launch, before anything plays.
func (w *window) setControlsEnabled(enabled bool) {
	for _, obj := range []fyne.Disableable{w.playBtn, w.seekSlider, w.volSlider, w.fsBtn} {
		if enabled {
			obj.Enable()
		} else {
			obj.Disable()
		}
	}
}
