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

// The dialog edits more than a language and a colour scheme now: storage, the
// gateway and the signaler all live here. A field that is written and read
// back wrongly is a setting that looks like it works, which is the exact shape
// of the mpv bug the file's other tests exist for.
func TestSettingsRememberEverythingTheDialogEdits(t *testing.T) {
	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))

	written := Settings{
		Language:      "ru",
		DarkTheme:     true,
		PlayerBinary:  "/opt/mpv",
		StorageMode:   StorageDisk,
		DataDir:       "/tmp/pieces",
		StorageAsked:  true,
		GatewayURL:    "https://gateway.example.com",
		GatewayUser:   "friend",
		GatewayPass:   "secret",
		GatewayCACert: "/etc/ca.pem",
		Signaler:      "ws://127.0.0.1:1337/",
	}
	require.NoError(t, store.Save(written))

	assert.Equal(t, written, store.Load())
}

// The storage question belongs to the first launch and to nothing else: asked
// on every start it would be a nag, and asked never it would leave a person
// with a film in RAM and no explanation.
func TestTheStorageQuestionIsAskedOnlyUntilAnswered(t *testing.T) {
	assert.True(t, DefaultSettings().NeedsStorageChoice())

	answered := DefaultSettings()
	answered.StorageAsked = true
	assert.False(t, answered.NeedsStorageChoice())
}

// The backend exits on startup when disk storage has nowhere to write, and the
// person reads that in a log. Catching it in the form costs nothing.
func TestDiskStorageNeedsADirectoryToWriteTo(t *testing.T) {
	assert.Empty(t, Settings{}.Problem(), "memory needs no directory")

	disk := Settings{StorageMode: StorageDisk}
	assert.NotEmpty(t, disk.Problem())

	disk.DataDir = "   "
	assert.NotEmpty(t, disk.Problem(), "whitespace is not a directory")

	disk.DataDir = "/tmp/pieces"
	assert.Empty(t, disk.Problem())
}

// The backend refuses a gateway whose login is half-given: the server would
// start and every playback would fail at the first request instead.
func TestTheGatewayNeedsBothHalvesOfItsLogin(t *testing.T) {
	half := Settings{GatewayURL: "https://gateway.example.com"}
	assert.NotEmpty(t, half.Problem())

	half.GatewayUser = "friend"
	assert.NotEmpty(t, half.Problem())

	half.GatewayPass = "secret"
	assert.Empty(t, half.Problem())

	// A login without a URL is not a problem: the gateway is simply off.
	assert.Empty(t, Settings{GatewayUser: "friend"}.Problem())
}

// Where rooms meet: the flag is a deliberate override for one launch, so it
// wins; then the setting; then the public address. Getting the order wrong
// silently ignores the person who typed the flag.
func TestTheSignalerPrecedence(t *testing.T) {
	assert.Equal(t, "ws://flag/", EffectiveSignaler("ws://flag/", "ws://setting/"))
	assert.Equal(t, "ws://setting/", EffectiveSignaler("", "ws://setting/"))
	assert.Equal(t, DefaultSignaler, EffectiveSignaler("", ""))
}

// A typed ~ has to mean the same directory the form would have prefilled, or
// the backend creates a literal "~" folder beside itself.
func TestATypedTildeMeansTheHomeDirectory(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(home, "Pieces"), ExpandPath("~/Pieces"))
	assert.Equal(t, home, ExpandPath("~"))
	assert.Equal(t, "/already/absolute", ExpandPath("/already/absolute"))
	assert.Equal(t, "", ExpandPath("   "))
}

// Pieces are large and meant to be deleted by hand, so the default place is
// one a person already knows how to find.
func TestTheDefaultDirectoryForPiecesIsInDownloads(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(home, "Downloads", "TorrSyncPlayer"), DefaultDataDir())
}
