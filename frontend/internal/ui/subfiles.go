// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"path"
	"strings"
	"time"
)

// Subtitle files live next to the film, not in it (idea-0080, variant 2).
// The container-track chooser cannot see them, so they get a chooser of their
// own: one row per subtitle file of the torrent, loaded into the running mpv
// with sub-add. Off unloads what was added, and only that — the container's
// own subtitles are the other chooser's business.

// subtitleExtensions is what counts as a subtitle file.
var subtitleExtensions = []string{".srt", ".vtt", ".ass", ".ssa", ".sub"}

// subtitleFiles picks the subtitle files out of a torrent listing, in listing
// order: the room watches one film, and its subtitles sit beside it.
func subtitleFiles(files []fileEntry) []fileEntry {
	var out []fileEntry
	for _, f := range files {
		lower := strings.ToLower(f.Name)
		for _, ext := range subtitleExtensions {
			if strings.HasSuffix(lower, ext) {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// subFileOption is one row of the subtitle-file chooser: the label a person
// reads, the file index the endpoint takes, and the full name the URL
// carries for mpv's format detection. Off is index -1: no torrent file is
// ever -1, so the switch cannot collide with a choice.
type subFileOption struct {
	Label string
	Index int
	Name  string
}

// subFileOptions builds the chooser rows: the off switch first, then one row
// per file labelled by base name. No files means no rows and no chooser — a
// selector with only "Off" in it invites a person to look for subtitles the
// torrent does not have.
func subFileOptions(files []fileEntry, offLabel string) []subFileOption {
	subs := subtitleFiles(files)
	if len(subs) == 0 {
		return nil
	}
	options := make([]subFileOption, 0, len(subs)+1)
	options = append(options, subFileOption{Label: offLabel, Index: -1})
	for _, f := range subs {
		options = append(options, subFileOption{Label: path.Base(f.Name), Index: f.Index, Name: f.Name})
	}
	return options
}

// showSubFiles puts the subtitle-file chooser on screen when the torrent has
// subtitle files. It opens on Off: a file nobody asked for must not read
// itself aloud in the wrong language.
func (w *window) showSubFiles() {
	w.mu.RLock()
	files := w.files
	w.mu.RUnlock()

	options := subFileOptions(files, w.tr("track.off"))
	if len(options) == 0 {
		onMain(func() { w.subFileSelect.Hide() })
		return
	}

	labels := make([]string, 0, len(options))
	for _, o := range options {
		labels = append(labels, o.Label)
	}
	onMain(func() {
		w.subFileSelect.OnChanged = nil
		w.subFileSelect.Options = labels
		w.subFileSelect.SetSelected(options[0].Label)
		w.subFileSelect.OnChanged = func(choice string) { w.chooseSubFile(choice) }
		w.subFileSelect.Show()
		w.subFileSelect.Refresh()
	})
}

// chooseSubFile sends a subtitle-file row to mpv by the label the person
// read. Off unloads what was added before — and only that: with nothing
// external loaded RemoveExternalSubtitles sends nothing at all.
func (w *window) chooseSubFile(choice string) {
	if choice == "" {
		return
	}

	w.mu.RLock()
	mpv := w.mpv
	torrent := w.current
	files := w.files
	w.mu.RUnlock()
	if mpv == nil || torrent == nil {
		return
	}

	for _, o := range subFileOptions(files, w.tr("track.off")) {
		if o.Label != choice {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if o.Index < 0 {
			if err := mpv.RemoveExternalSubtitles(ctx); err != nil {
				w.setStatus(w.trf("status.trackFailed", choice+": "+err.Error()))
				return
			}
			w.setStatus(w.trf("status.trackChosen", choice))
			return
		}
		if err := mpv.AddSubtitles(ctx, w.client.SubtitlesURL(torrent.ID, o.Index, o.Name)); err != nil {
			w.setStatus(w.trf("status.trackFailed", choice+": "+err.Error()))
			return
		}
		w.setStatus(w.trf("status.trackChosen", choice))
		return
	}
}
