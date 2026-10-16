// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
)

// The settings dialog is where storage, the gateway and rendezvous became
// reachable without a command line. The mapping from widgets back onto Settings
// is where a field quietly stops working: the dialog shows a value, apply drops
// it, and nothing about the file looks wrong — the exact shape of the mpv bug
// the settings file's own tests exist for.

// testWindow gives the form the parent a dialog attaches to.
func testWindow(t *testing.T) fyne.Window {
	t.Helper()

	app := test.NewApp()
	window := app.NewWindow("test")
	t.Cleanup(window.Close)
	return window
}

// testTr is the catalogue reduced to identity: this file is about the mapping,
// not about the words.
func testTr(key string) string { return key }

func TestTheFormMapsEveryFieldBackOntoTheSettings(t *testing.T) {
	parent := testWindow(t)
	current := Settings{
		Language:    catalog.LangRussian,
		StorageMode: StorageMemory,
	}

	form := newSettingsForm(current, testTr, parent)
	// Driven the way a person drives it: pick, type, type spaces around a
	// value, and expect the form to know what is space and what is a value.
	form.language.SetSelected(languageOptionEnglish)
	form.dark.SetChecked(true)
	form.mpv.SetText("~/bin/mpv")
	form.storage.group.SetSelected(form.storage.disk)
	form.storage.dir.SetText("~/Pieces")
	form.gatewayURL.SetText("  https://gateway.example.com  ")
	form.gatewayUser.SetText("  friend  ")
	form.gatewayPass.SetText(" a secret with spaces ")
	form.gatewayCACert.SetText("~/ca.pem")
	form.bootstrap.SetText("  /dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN  ")
	form.relays.SetText("  /ip4/203.0.113.7/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN  ")

	updated := form.apply(current)

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.Equal(t, catalog.English, updated.Language)
	assert.True(t, updated.DarkTheme)
	assert.Equal(t, filepath.Join(home, "bin", "mpv"), updated.PlayerBinary)
	assert.Equal(t, StorageDisk, updated.StorageMode)
	assert.Equal(t, filepath.Join(home, "Pieces"), updated.DataDir)
	assert.True(t, updated.StorageAsked, "answering in the dialog answers the first-launch question")
	assert.Equal(t, "https://gateway.example.com", updated.GatewayURL)
	assert.Equal(t, "friend", updated.GatewayUser)
	assert.Equal(t, " a secret with spaces ", updated.GatewayPass, "a password with spaces in it is a password")
	assert.Equal(t, filepath.Join(home, "ca.pem"), updated.GatewayCACert)
	assert.Equal(t, "/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN", updated.Bootstrap)
	assert.Equal(t, "/ip4/203.0.113.7/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN", updated.Relays)
}

func TestTheLanguageCanGoBackToTheSystem(t *testing.T) {
	parent := testWindow(t)
	current := Settings{Language: catalog.English}

	form := newSettingsForm(current, testTr, parent)
	form.language.SetSelected(testTr("settings.languageSystem"))

	assert.Empty(t, form.apply(current).Language,
		"the system choice should be the empty setting, not a pinned language")
}

// Switching to memory keeps the directory: it is where the pieces will go if
// the person switches back, and erasing it would make the direction of the
// choice irreversible.
func TestChoosingMemoryKeepsTheDirectoryForLater(t *testing.T) {
	parent := testWindow(t)
	current := Settings{StorageMode: StorageDisk, DataDir: "/pieces"}

	form := newSettingsForm(current, testTr, parent)
	form.storage.group.SetSelected(form.storage.memory)

	updated := form.apply(current)
	assert.Equal(t, StorageMemory, updated.StorageMode)
	assert.Equal(t, "/pieces", updated.DataDir, "switching to memory erased where the pieces would go")
}

func TestTheDirectoryFieldIsPrefilledWithTheDefault(t *testing.T) {
	parent := testWindow(t)

	form := newSettingsForm(Settings{}, testTr, parent)
	assert.Equal(t, DefaultDataDir(), form.storage.dir.Text)

	// An emptied field gets the default back when disk is chosen: saving disk
	// storage with nowhere to write would fail at the next launch, in the
	// backend's output, after the dialog said everything was fine.
	form.storage.dir.SetText("")
	form.storage.group.SetSelected(form.storage.disk)
	assert.Equal(t, DefaultDataDir(), form.storage.dir.Text)
}

func TestTheStorageQuestionSavesTheAnswerThatWasGiven(t *testing.T) {
	parent := testWindow(t)
	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))

	question := newStorageQuestion(store, DefaultSettings(), parent)
	assert.False(t, question.choice.diskSelected(), "memory is the preselected answer (idea-0081)")

	question.choice.group.SetSelected(question.choice.disk)
	question.choice.dir.SetText("/tmp/pieces")
	test.Tap(question.save)

	saved := store.Load()
	assert.True(t, saved.StorageAsked)
	assert.Equal(t, StorageDisk, saved.StorageMode)
	assert.Equal(t, "/tmp/pieces", saved.DataDir)
}

// "Later" is not an answer: the question returns on the next launch rather than
// a memory default being written as if it had been chosen.
func TestLaterIsNotAnAnswerToTheStorageQuestion(t *testing.T) {
	parent := testWindow(t)
	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))

	question := newStorageQuestion(store, DefaultSettings(), parent)
	test.Tap(question.later)

	assert.True(t, store.Load().NeedsStorageChoice(),
		"\"Later\" answered a question nobody answered")
}

func TestTheFirstLaunchAsksAboutStorage(t *testing.T) {
	parent := testWindow(t)
	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))

	AskAboutStorage(store, parent)

	assert.Len(t, parent.Canvas().Overlays().List(), 1,
		"the first launch was not asked where the pieces should live")
}

func TestAnAnsweredQuestionIsNotAskedAgain(t *testing.T) {
	parent := testWindow(t)
	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	require.NoError(t, store.Save(Settings{StorageAsked: true}))

	AskAboutStorage(store, parent)

	assert.Empty(t, parent.Canvas().Overlays().List(), "the answered question came back")
}
