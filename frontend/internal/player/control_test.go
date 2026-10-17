// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// The window's remote needs three verbs mpv already speaks: read the
// loudness, set it, flip fullscreen. Each pins the exact wire shape,
// because a wrong property name is silent — mpv answers "success" to a
// set it does not understand the way the window means.

func TestVolumeReadsTheProperty(t *testing.T) {
	fake := startFakeMPV(map[string]any{"error": "success", "data": 42.0})

	volume, err := fake.play("fake").Volume(context.Background())
	if err != nil {
		t.Fatalf("reading the volume: %v", err)
	}
	if volume != 42 {
		t.Errorf("mpv said 42, the window would show %v", volume)
	}

	select {
	case request := <-fake.requests:
		command, _ := request["command"].([]any)
		if len(command) != 2 || command[0] != "get_property" || command[1] != "volume" {
			t.Fatalf("mpv was asked %v, wanted get_property volume", command)
		}
	case <-timeAfter():
		t.Fatal("the fake mpv never saw the query")
	}
}

func TestSetVolumeClampsInsteadOfRefusing(t *testing.T) {
	fake := startFakeMPV(map[string]any{"error": "success", "data": nil})

	for volume, want := range map[float64]float64{-5: 0, 30: 30, 250: 100} {
		if err := fake.play("fake").SetVolume(context.Background(), volume); err != nil {
			t.Fatalf("setting the volume to %v: %v", volume, err)
		}

		select {
		case request := <-fake.requests:
			command, _ := request["command"].([]any)
			if len(command) != 3 || command[0] != "set_property" || command[1] != "volume" {
				t.Fatalf("mpv was sent %v, wanted set_property volume", command)
			}
			if got, _ := command[2].(float64); got != want {
				t.Errorf("mpv was sent volume %v, wanted %v", got, want)
			}
		case <-timeAfter():
			t.Fatal("the fake mpv never saw the command")
		}
	}
}

func TestToggleFullscreenCyclesTheProperty(t *testing.T) {
	fake := startFakeMPV(map[string]any{"error": "success", "data": nil})

	if err := fake.play("fake").ToggleFullscreen(context.Background()); err != nil {
		t.Fatalf("toggling fullscreen: %v", err)
	}

	select {
	case request := <-fake.requests:
		command, _ := request["command"].([]any)
		if len(command) != 2 || command[0] != "cycle" || command[1] != "fullscreen" {
			t.Fatalf("mpv was sent %v, wanted cycle fullscreen", command)
		}
	case <-timeAfter():
		t.Fatal("the fake mpv never saw the command")
	}
}

// Against a real mpv, headless: set the loudness, read it back. The fake
// pins the wire shape; only the binary proves the property exists.
//
// The mismatch decides, not the machine's mood. Some builds apply the
// volume through the audio chain, and a machine with no chain has nothing
// to apply it to — the value is set and never lands (mpv 0.34 on the
// ubuntu runner, no sound card). That is the machine, not the command, so
// the reason is asked of mpv itself and travels with the skip. An output
// that does exist keeps this a failure: there the value was ours to land.
func TestRealMpvVolumeRoundTrip(t *testing.T) {
	p := startRealPlayer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := p.SetVolume(ctx, 33); err != nil {
		t.Fatalf("mpv did not take the volume: %v", err)
	}
	volume, err := p.Volume(ctx)
	if err != nil {
		t.Fatalf("mpv did not report the volume: %v", err)
	}
	if volume == 33 {
		return
	}

	var output struct {
		Error string `json:"error"`
		Data  string `json:"data"`
	}
	reason := ""
	if err := p.request(ctx, []any{"get_property", "current-ao"}, &output); err != nil {
		reason = fmt.Sprintf("mpv would not say whether it has an audio output (%v)", err)
	} else if output.Error != "" && output.Error != "success" {
		reason = fmt.Sprintf("no audio output on this machine (current-ao: %q), so there "+
			"is no chain to apply the volume to", output.Error)
	}
	if reason != "" {
		t.Skipf("mpv reports %v instead of 33: %s; the wire shape is pinned above",
			volume, reason)
	}
	t.Errorf("mpv reports %v after being set to 33", volume)
}
