// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"path/filepath"
	"strings"
)

// When a tap on a poster means "play it", the window should not answer with
// a question the person has already answered. This decides which file that
// tap selects, without asking.
//
// The rules, in order:
//
//   - one file in the torrent: it is the film, play it;
//   - several videos, but one of them outweighs the rest combined by far:
//     the release is a film plus extras, play the film;
//   - several comparable videos: it is a season, it is genuinely ambiguous,
//     and the person picks — this returns ok=false and the chips appear.
//
// Anything with no video extension at stake at all is ambiguous too —
// unless it is the only file.

var videoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".mov":  true,
	".webm": true,
	".m4v":  true,
	".ts":   true,
	".m2ts": true,
	".vob":  true,
	".wmv":  true,
}

func isVideoName(name string) bool {
	ext := filepath.Ext(name)
	return videoExtensions[strings.ToLower(ext)]
}

func obviousFile(files []fileEntry) (fileEntry, bool) {
	if len(files) == 1 {
		return files[0], true
	}
	videos := make([]fileEntry, 0, len(files))
	for _, f := range files {
		if isVideoName(f.Name) {
			videos = append(videos, f)
		}
	}
	if len(videos) == 1 {
		return videos[0], true
	}
	if len(videos) >= 2 {
		largest, rest := videos[0], int64(0)
		for _, v := range videos[1:] {
			if v.Size > largest.Size {
				rest += largest.Size
				largest = v
			} else {
				rest += v.Size
			}
		}
		// The film outweighs every other video combined: the samples and
		// trailers in the release cannot compete with it.
		if largest.Size >= 3*rest {
			return largest, true
		}
	}
	return fileEntry{}, false
}
