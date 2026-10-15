// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
)

// The interface language is resolved from three places, in order. Each link
// exists for a different person: the --lang flag is a deliberate choice for one
// run, the saved setting is the choice that survives restarts, and the system
// locale is what a first run should look like. Getting the order wrong silently
// ignores someone.

// withSystemLocale puts the machine in a known locale for one test. The
// fallback chain has three links, and the system one cannot otherwise be
// reached without being on the right machine.
func withSystemLocale(t *testing.T, locale string) {
	t.Helper()

	previous := systemLocale
	systemLocale = func() string { return locale }
	t.Cleanup(func() { systemLocale = previous })
}

func TestTheLanguageFlagOutranksTheSavedChoice(t *testing.T) {
	withSystemLocale(t, "de")

	assert.Equal(t, catalog.English, PreferredLanguage("en", catalog.LangRussian))
	assert.Equal(t, catalog.LangRussian, PreferredLanguage("ru", catalog.English))
	assert.Equal(t, catalog.LangRussian, PreferredLanguage("RU", ""), "a flag is a locale, casing and all")
}

func TestTheSavedChoiceOutranksTheSystem(t *testing.T) {
	withSystemLocale(t, "en")

	assert.Equal(t, catalog.LangRussian, PreferredLanguage("", catalog.LangRussian))
}

func TestTheSystemLocaleIsUsedWhenNothingElseSpeaks(t *testing.T) {
	withSystemLocale(t, "ru-RU")

	assert.Equal(t, catalog.LangRussian, PreferredLanguage("", ""))
}

func TestAnUnsupportedLocaleFallsThroughInsteadOfPinning(t *testing.T) {
	withSystemLocale(t, "ru")

	// The flag named a language nobody offers; the saved choice still counts.
	assert.Equal(t, catalog.LangRussian, PreferredLanguage("de", catalog.LangRussian))

	// And when nothing in the chain is supported, the source language.
	withSystemLocale(t, "de")
	assert.Equal(t, catalog.English, PreferredLanguage("fr", "zh-CN"))
}

// Locales arrive in whatever spelling the platform prefers: the environment
// uses ru_RU.UTF-8, desktop APIs use ru-RU, and a person types ru.
func TestLocaleNamesAreParsed(t *testing.T) {
	for _, candidate := range []string{"ru", "ru-RU", "ru_RU", "ru_RU.UTF-8", "RU", " ru "} {
		assert.Equal(t, catalog.LangRussian, supportedLanguage(candidate), candidate)
	}
	for _, candidate := range []string{"en", "en-US", "en_US.UTF-8", "EN"} {
		assert.Equal(t, catalog.English, supportedLanguage(candidate), candidate)
	}
	for _, candidate := range []string{"", "C", "de", "zh-CN", "sr_RS@latin"} {
		assert.Empty(t, supportedLanguage(candidate), candidate)
	}
}
