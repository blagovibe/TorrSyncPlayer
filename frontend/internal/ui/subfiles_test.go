// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"
)

// Subtitle files live next to the film, not in it (idea-0080, variant 2).
// The chooser offers them by file: the container-track chooser cannot see
// them, and mpv loads them with sub-add at runtime.

// subtitleExtensions is what counts as a subtitle file. Matched case-
// insensitively against the file name: releases spell them .SRT as often as
// .srt, and a filter that misses half the room's collection is decoration.
func TestSubtitleFilesPicksSubtitleExtensions(t *testing.T) {
	files := []fileEntry{
		{Index: 0, Name: "film.mkv"},
		{Index: 1, Name: "film.RU.srt"},
		{Index: 2, Name: "film.en.VTT"},
		{Index: 3, Name: " subs/forced.ass"},
		{Index: 4, Name: " subs/commentary.SSA"},
		{Index: 5, Name: "cover.jpg"},
		{Index: 6, Name: "notes.txt"},
		{Index: 7, Name: "noextension"},
	}

	got := subtitleFiles(files)
	var indexes []int
	for _, f := range got {
		indexes = append(indexes, f.Index)
	}
	want := []int{1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("picked %v, wanted subtitle files %v", indexes, want)
	}
	for i := range want {
		if indexes[i] != want[i] {
			t.Fatalf("picked %v, wanted subtitle files %v", indexes, want)
		}
	}
}

// subFileOptions builds the chooser rows: the off switch first, then one row
// per file, labelled by base name — the directory is the release's, not
// information. The index travels with the label because the label is what the
// person reads and the index is what the endpoint takes.
func TestSubFileOptionsStartsWithOff(t *testing.T) {
	files := []fileEntry{
		{Index: 3, Name: "subs/forced.ass"},
		{Index: 1, Name: "film.RU.srt"},
	}

	options := subFileOptions(files, "Off")
	if len(options) != 3 {
		t.Fatalf("got %d options, wanted off plus two files", len(options))
	}
	if options[0].Label != "Off" || options[0].Index != -1 {
		t.Errorf("first row is %+v, not the off switch", options[0])
	}
	if options[1].Label != "forced.ass" || options[1].Index != 3 || options[1].Name != "subs/forced.ass" {
		t.Errorf("second row is %+v, not the base name with its index and full name", options[1])
	}
	if options[2].Label != "film.RU.srt" || options[2].Index != 1 {
		t.Errorf("third row is %+v, not the base name with its index", options[2])
	}
}

func TestSubFileOptionsEmptyWithoutFiles(t *testing.T) {
	if options := subFileOptions(nil, "Off"); len(options) != 0 {
		t.Fatalf("no subtitle files must mean no chooser, got %+v", options)
	}
}

// mpvHeaders carries the backend's whole identity check to the player: the
// token that guards it and the client id that owns its torrents. One header
// used to look like enough, until the ownership gate showed it is not
// (bug-0089).
func TestMpvHeadersCarryTokenAndClientID(t *testing.T) {
	headers, err := mpvHeaders("tok", "peer-7")
	if err != nil {
		t.Fatalf("building mpv headers: %v", err)
	}
	if len(headers) != 2 || headers[0] != "X-Access-Token: tok" || headers[1] != "X-Client-ID: peer-7" {
		t.Fatalf("mpv would send %q, not the token with its owner", headers)
	}
}

func TestMpvHeadersWithoutClientIDStayAnonymous(t *testing.T) {
	headers, err := mpvHeaders("tok", "")
	if err != nil {
		t.Fatalf("building mpv headers: %v", err)
	}
	if len(headers) != 1 || headers[0] != "X-Access-Token: tok" {
		t.Fatalf("mpv would send %q, not the lone token", headers)
	}
}
