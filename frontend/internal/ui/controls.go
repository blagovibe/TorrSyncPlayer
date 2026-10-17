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
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
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

// mediaInfoText renders the line under the seeker: what is playing, how
// large, how long. Empty until all three are known — a half line that fills
// in piece by piece looks like the window guessing.
func mediaInfoText(tr func(string) string, name string, size int64, duration float64) string {
	if name == "" || size <= 0 || duration <= 0 {
		return ""
	}
	return fmt.Sprintf(tr("media.info"), name, humanSize(size), formatClock(duration))
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

// buildControls assembles the playback row: play/pause, elapsed, seeker,
// remaining, fullscreen — and beneath it volume with the media line. The
// window's remote, after the player window's screen: every one of these
// commands the same mpv the room drives.
func (w *window) buildControls() fyne.CanvasObject {
	w.playBtn = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), w.onPlayPressed)
	w.elapsedLbl = widget.NewLabel(formatClock(0))
	w.seekSlider = widget.NewSlider(0, 1)
	w.seekSlider.Step = 1
	w.seekSlider.OnChanged = w.onSeekMoved
	w.remainingLbl = widget.NewLabel(formatClock(0))
	w.fsBtn = widget.NewButtonWithIcon("", theme.ViewFullScreenIcon(), w.onFullscreenPressed)

	seekRow := container.NewBorder(nil, nil,
		container.NewHBox(w.playBtn, w.elapsedLbl),
		container.NewHBox(w.remainingLbl, w.fsBtn),
		w.seekSlider)

	w.volSlider = widget.NewSlider(0, 100)
	w.volSlider.Step = 1
	w.volSlider.OnChanged = w.onVolumeMoved
	w.mediaLbl = widget.NewLabel("")
	volRow := container.NewHBox(
		widget.NewIcon(theme.VolumeUpIcon()),
		w.volSlider,
		w.mediaLbl,
	)

	w.setControlsEnabled(false)
	return container.NewVBox(seekRow, volRow)
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
	if w.current != nil {
		name, size = w.current.Name, w.current.Size
	}
	userHolding := hasSeek
	w.mu.RUnlock()

	onMain(func() {
		if pausedErr == nil {
			w.mu.Lock()
			w.ctlPaused = paused
			w.mu.Unlock()
			if paused {
				w.playBtn.SetIcon(theme.MediaPlayIcon())
			} else {
				w.playBtn.SetIcon(theme.MediaPauseIcon())
			}
		}
		if positionErr == nil && durationErr == nil && duration > 0 {
			if w.seekSlider.Max != duration {
				w.seekSlider.Max = duration
			}
			if !userHolding {
				setSliderValue(w.seekSlider, w.onSeekMoved, clamp(position, 0, duration))
			}
			w.elapsedLbl.SetText(formatClock(position))
			w.remainingLbl.SetText(formatClock(duration - position))
		}
		if volumeErr == nil {
			setSliderValue(w.volSlider, w.onVolumeMoved, clamp(volume, 0, 100))
		}
		w.mediaLbl.SetText(mediaInfoText(w.tr, name, size, duration))
	})
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
