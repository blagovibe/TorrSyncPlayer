// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE for full license text

package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// Removing a torrent that does not free its disk is the worst kind of bug
// here: the API answers 200, the torrent disappears from the list, and
// gigabytes stay where the user believes they deleted them.
//
// It happened because the deletion path was written from a plausible reading of
// the storage layout rather than from the layout itself. anacrolix writes
// <baseDir>/<Info.BestName()>/<File.BestPath()...> — flat, with no infohash
// directory — and this removed baseDir/<infohash>, a directory that is never
// created. os.RemoveAll on a path that does not exist returns nil, so the
// failure was silent and total.

// TestRemovingATorrentDeletesWhatAnacrolixWrote is the layout itself, asserted
// rather than assumed: the directory that goes is the one that was filled.
func TestRemovingATorrentDeletesWhatAnacrolixWrote(t *testing.T) {
	base := t.TempDir()

	// How anacrolix lays a torrent down: <base>/<info name>/<file path>.
	filmDir := filepath.Join(base, "film.mkv")
	if err := os.MkdirAll(filepath.Join(filmDir, "extras"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range []string{"film.mkv", "extras/subtitles.srt"} {
		if err := os.WriteFile(filepath.Join(filmDir, filepath.FromSlash(name)), []byte("data"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// A second torrent that must survive: removing one torrent is not a licence
	// to empty the storage.
	other := filepath.Join(base, "other.release")
	if err := os.MkdirAll(other, 0o750); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}
	if err := os.WriteFile(filepath.Join(other, "other.mkv"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write other: %v", err)
	}

	fs := NewFileStorage(base)
	if err := fs.RemoveTorrent("film.mkv"); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := os.Stat(filmDir); !os.IsNotExist(err) {
		t.Errorf("the torrent's data is still on disk: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("removing one torrent took another's data with it: %v", err)
	}
}

// TestRemovingATorrentThatIsNotThereSucceeds: the torrent was in memory, or was
// never written. There is nothing to delete and nothing to complain about.
func TestRemovingATorrentThatIsNotThereSucceeds(t *testing.T) {
	fs := NewFileStorage(t.TempDir())
	if err := fs.RemoveTorrent("never-written"); err != nil {
		t.Errorf("removing a torrent with no data on disk should not fail: %v", err)
	}
}

// TestRemovingATorrentWithNoNameDoesNotEmptyTheStorage is the case where the
// honest answer is "I cannot tell what belongs to this torrent": with the
// sentinel name anacrolix writes the file straight into the base directory, so
// there is no per-torrent directory to delete and deleting one would take the
// other torrents with it.
func TestRemovingATorrentWithNoNameDoesNotEmptyTheStorage(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "loose.mkv"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := NewFileStorage(base)
	if err := fs.RemoveTorrent("-"); err != nil {
		t.Errorf("remove with the sentinel name: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "loose.mkv")); err != nil {
		t.Errorf("a torrent with no directory of its own took the storage with it: %v", err)
	}
}
