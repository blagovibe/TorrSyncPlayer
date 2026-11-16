// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import "testing"

func TestOneFileIsTheObviousFile(t *testing.T) {
	files := []fileEntry{{Index: 0, Name: "The Odyssey (2026).mkv", Size: 8e9}}
	got, ok := obviousFile(files)
	if !ok || got.Index != 0 {
		t.Fatalf("a single file must be the obvious one, got ok=%v index=%d", ok, got.Index)
	}
}

func TestOneVideoAmongExtrasIsTheObviousFile(t *testing.T) {
	files := []fileEntry{
		{Index: 0, Name: "The.Odyssey.2026.BDRip.720p.x264_New-Team.mkv", Size: 8e9},
		{Index: 1, Name: "The.Odyssey.2026.RU.srt", Size: 90_000},
		{Index: 2, Name: "cover.jpg", Size: 500_000},
	}
	got, ok := obviousFile(files)
	if !ok || got.Index != 0 {
		t.Fatalf("one video among extras must be the film, got ok=%v index=%d", ok, got.Index)
	}
}

func TestTheGiantVideoBeatsTheSamples(t *testing.T) {
	files := []fileEntry{
		{Index: 0, Name: "sample.mkv", Size: 60e6},
		{Index: 1, Name: "trailer.mkv", Size: 120e6},
		{Index: 2, Name: "The.Odyssey.2026.mkv", Size: 9e9},
		{Index: 3, Name: "interview.mkv", Size: 200e6},
	}
	got, ok := obviousFile(files)
	if !ok || got.Index != 2 {
		t.Fatalf("the giant video among samples must win, got ok=%v index=%d", ok, got.Index)
	}
}

func TestComparableVideosNeedAHuman(t *testing.T) {
	files := []fileEntry{
		{Index: 0, Name: "Show.S01E01.mkv", Size: 800e6},
		{Index: 1, Name: "Show.S01E02.mkv", Size: 790e6},
		{Index: 2, Name: "Show.S01E03.mkv", Size: 810e6},
	}
	if _, ok := obviousFile(files); ok {
		t.Fatal("a season of comparable episodes must not be picked for the person")
	}
}

func TestATwoThirdsVideoDoesNotAutoPlay(t *testing.T) {
	files := []fileEntry{
		{Index: 0, Name: "Part.One.mkv", Size: 6e9},
		{Index: 1, Name: "Part.Two.mkv", Size: 8e9},
	}
	if _, ok := obviousFile(files); ok {
		t.Fatal("two comparable parts need a human")
	}
}

func TestNoVideoAmongSeveralNeedsAHuman(t *testing.T) {
	files := []fileEntry{
		{Index: 0, Name: "covers.iso", Size: 700e6},
		{Index: 1, Name: "cover.jpg", Size: 120_000},
	}
	if _, ok := obviousFile(files); ok {
		t.Fatal("nothing playable here — the chips must stay")
	}
}
