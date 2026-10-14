// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Settings are the few choices a person makes that should survive a restart.
//
// Deliberately small. Everything else either comes from the backend or belongs
// to the room, and saving those would mean storing state that is somebody
// else's to define.
type Settings struct {
	// Language is "ru" or "en". Empty means "whatever the system says".
	Language string `json:"language"`
	// DarkTheme overrides the system preference.
	DarkTheme bool `json:"darkTheme"`
	// PlayerBinary lets a person point at a different mpv. Empty means the one
	// on PATH.
	PlayerBinary string `json:"playerBinary,omitempty"`
}

// SettingsStore reads and writes settings, tolerating a file that is missing,
// unreadable or damaged.
//
// Every one of those is a normal state for a config file — a first run, a
// locked-down home directory, a half-written file after a power cut — and none
// of them is a reason to refuse to start. Defaults are returned instead.
type SettingsStore struct {
	path string

	mu sync.Mutex
}

// NewSettingsStore opens settings at a path.
//
// An empty path keeps settings in memory only, which is what a portable or
// read-only run wants.
func NewSettingsStore(path string) *SettingsStore {
	return &SettingsStore{path: path}
}

// DefaultSettings is what an absent or damaged file yields.
func DefaultSettings() Settings {
	return Settings{Language: "", DarkTheme: false}
}

// Load reads the settings, falling back to defaults on any problem.
func (s *SettingsStore) Load() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.path == "" {
		return DefaultSettings()
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		return DefaultSettings()
	}

	var settings Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		// A damaged file is overwritten on the next Save, which is the only
		// sensible thing to do with something unreadable.
		return DefaultSettings()
	}
	return settings
}

// Save writes the settings.
func (s *SettingsStore) Save(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.path == "" {
		return nil
	}

	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	// Written through a temporary file and renamed: a half-written settings
	// file is exactly the damaged state Load has to recover from, and it is
	// easy not to hit.
	temp := s.path + ".tmp"
	if err := os.WriteFile(temp, encoded, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, s.path)
}

// DefaultPath is where settings live for this application.
func DefaultPath(configDir string) string {
	return filepath.Join(configDir, "TorrSyncPlayer", "settings.json")
}
