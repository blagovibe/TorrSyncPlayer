// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Storage modes a person can choose. Memory is what an unanswered question
// means: the application never moves anybody to disk on its own.
const (
	// StorageMemory keeps torrent pieces in RAM. Nothing is left on disk and a
	// restart starts the download over.
	StorageMemory = "memory"
	// StorageDisk writes pieces under DataDir, so a restart resumes.
	StorageDisk = "disk"
)

// ExampleBootstrap shows the bootstrap field's format: comma-separated
// multiaddrs with /p2p/. It is one of libp2p's public bootstrap nodes, shown
// as a placeholder — an empty field already means the public defaults.
const ExampleBootstrap = "/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN"

// Settings are the choices a person makes that should survive a restart.
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
	// DisplayName is what the room shows for this person. Empty means the room
	// falls back to the device's short id.
	DisplayName string `json:"displayName,omitempty"`

	// StorageMode is "memory" or "disk". Empty means memory.
	StorageMode string `json:"storageMode,omitempty"`
	// DataDir is where pieces go when StorageMode is disk.
	DataDir string `json:"dataDir,omitempty"`
	// StorageAsked records that the first-launch storage question was
	// answered. The question is asked once per installation.
	StorageAsked bool `json:"storageAsked,omitempty"`

	// Gateway (remoting) mode: when the URL is set, the backend proxies every
	// torrent operation to an htorrent gateway instead of using its own
	// client. The password travels to the child in its environment, never on
	// the command line.
	GatewayURL    string `json:"gatewayUrl,omitempty"`
	GatewayUser   string `json:"gatewayUser,omitempty"`
	GatewayPass   string `json:"gatewayPass,omitempty"`
	GatewayCACert string `json:"gatewayCaCert,omitempty"`

	// Bootstrap is DHT rendezvous as comma-separated multiaddrs with /p2p/.
	// Empty means the public defaults; the --bootstrap flag overrides it for
	// one launch.
	Bootstrap string `json:"bootstrap,omitempty"`

	// Relays are the owner's fallback relay candidates as comma-separated
	// multiaddrs with /p2p/. Empty means DHT discovery only: autorelay finds
	// candidates by walking the DHT, and nothing is configured. The --relays
	// flag overrides it for one launch.
	Relays string `json:"relays,omitempty"`
}

// UsesDiskStorage reports whether pieces go to disk.
func (s Settings) UsesDiskStorage() bool { return s.StorageMode == StorageDisk }

// NeedsStorageChoice reports whether the first-launch question about storage
// has not been answered yet.
func (s Settings) NeedsStorageChoice() bool { return !s.StorageAsked }

// Problem returns the catalogue key of the first reason these settings cannot
// be saved, or "" when they can.
//
// The rules mirror the ones the backend enforces at startup, so the person
// reads them in the form rather than in the backend's output after a restart
// that refused to happen.
func (s Settings) Problem() string {
	if s.UsesDiskStorage() && strings.TrimSpace(s.DataDir) == "" {
		return "settings.problem.diskDir"
	}
	if strings.TrimSpace(s.GatewayURL) != "" &&
		(strings.TrimSpace(s.GatewayUser) == "" || s.GatewayPass == "") {
		return "settings.problem.gatewayLogin"
	}
	return ""
}

// EffectiveBootstrap resolves where rooms meet: the flag, then the setting,
// then empty, which the mesh reads as the public defaults. The flag is a
// deliberate override for one launch, and a saved setting must not silently
// outrank it.
func EffectiveBootstrap(flagValue, setting string) string {
	if flagValue != "" {
		return flagValue
	}
	return setting
}

// EffectiveRelays resolves the fallback relays: the flag, then the setting,
// then empty, which the mesh reads as DHT discovery only. Same precedence
// shape as rendezvous: one launch override must outrank the saved choice.
func EffectiveRelays(flagValue, setting string) string {
	if flagValue != "" {
		return flagValue
	}
	return setting
}

// ExpandPath turns a leading ~ into this user's home directory, so a typed
// "~/Pieces" means the directory the form would have prefilled rather than a
// literal "~" folder beside the application.
func ExpandPath(path string) string {
	path = strings.TrimSpace(path)
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// Nothing to expand to. The path stays as typed; the backend's
		// startup check will be the one to explain it.
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

// DefaultDataDir is where pieces go when disk storage is chosen and no
// directory has been picked: inside the familiar Downloads folder, where a
// person can find and delete them.
func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Downloads", "TorrSyncPlayer")
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
