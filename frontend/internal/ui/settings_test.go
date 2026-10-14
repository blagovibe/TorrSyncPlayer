// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A config file that is missing, unreadable or damaged is a normal state, not
// a failure: it happens on a first run, in a locked-down home directory, and
// after a power cut. None of them is a reason to refuse to start.
func TestSettingsRoundTrip(t *testing.T) {
	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))

	written := Settings{Language: "ru", DarkTheme: true}
	require.NoError(t, store.Save(written))

	assert.Equal(t, written, store.Load())
}

func TestSettingsDefaultOnAMissingFile(t *testing.T) {
	store := NewSettingsStore(filepath.Join(t.TempDir(), "never-written.json"))

	assert.Equal(t, DefaultSettings(), store.Load())
}

func TestSettingsDefaultOnADamagedFile(t *testing.T) {
	// A half-written settings file must not stop the application: the choices
	// are a language and a colour scheme, and both have defaults.
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json at all"), 0o600))

	store := NewSettingsStore(path)
	assert.Equal(t, DefaultSettings(), store.Load())

	// And the next save repairs it rather than appending to the damage.
	require.NoError(t, store.Save(Settings{Language: "en"}))
	assert.Equal(t, "en", store.Load().Language)
}

func TestSettingsSurviveADirectoryThatDoesNotExistYet(t *testing.T) {
	// The usual first run: the config directory has never been created.
	path := filepath.Join(t.TempDir(), "deeply", "nested", "settings.json")
	store := NewSettingsStore(path)

	require.NoError(t, store.Save(Settings{Language: "ru"}))
	assert.Equal(t, "ru", store.Load().Language)
}

func TestSettingsAreNotWrittenWhereThereIsNoPath(t *testing.T) {
	// A portable or read-only run keeps settings in memory rather than failing
	// on every save.
	store := NewSettingsStore("")

	assert.NoError(t, store.Save(Settings{Language: "ru"}))
	assert.Equal(t, DefaultSettings(), store.Load())
}

func TestSettingsLeaveNoTemporaryFileBehind(t *testing.T) {
	// The write goes through a temporary file and a rename, so an interrupted
	// save cannot leave half a file where the real one used to be.
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	store := NewSettingsStore(path)

	require.NoError(t, store.Save(Settings{Language: "ru"}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "a temporary file was left behind: %v", entries)
	assert.Equal(t, "settings.json", entries[0].Name())
}

func TestSettingsDefaultPathIsPerApplication(t *testing.T) {
	// One directory per application: a shared flat file is how two tools end up
	// fighting over the same name.
	path := DefaultPath("/home/someone/.config")

	assert.Equal(t, filepath.Join("/home/someone/.config", "TorrSyncPlayer", "settings.json"), path)
}
