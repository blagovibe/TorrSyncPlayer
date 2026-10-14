// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// There is no lupdate for Fyne, so these catalogues are maintained by hand. A
// hand-maintained catalogue rots silently, and these are the checks that stop
// it: without them a renamed key ships as a missing translation nobody notices
// until a person reads that screen.

func TestEveryStringIsTranslated(t *testing.T) {
	missing := MissingFrom(LangRussian)
	assert.Empty(t, missing,
		"не переведено: %v", missing)
}

func TestNoCatalogueCarriesAKeyNothingUses(t *testing.T) {
	// An extra key is worse than a missing one: it hides the fact that a key
	// was renamed, because the translation keeps looking finished while nothing
	// displays it.
	extra := Extra(LangRussian)
	assert.Empty(t, extra,
		"в переводе есть ключи, которых нет в исходнике: %v", extra)
}

func TestAnEmptyTranslationCountsAsMissing(t *testing.T) {
	// An empty entry renders as the source language, so counting it as done
	// would report finished work that is not finished.
	var catalogue = Strings{"app.title": ""}
	assert.Contains(t, missingFrom(catalogue), "app.title")
}

func TestLookupFallsBackToTheSourceLanguage(t *testing.T) {
	// A half-translated catalogue should show English, never an empty button.
	Russian["__test_only__"] = ""
	defer delete(Russian, "__test_only__")

	assert.Equal(t, catalog["app.title"], Lookup("xx", "app.title"))
	assert.Equal(t, Russian["status.joined"], Lookup(LangRussian, "status.joined"))
}

func TestLookupPrefersTheRequestedLanguage(t *testing.T) {
	assert.Equal(t, "Войти", Lookup(LangRussian, "action.join"))
}

func TestAnUnknownLanguageIsNotAnError(t *testing.T) {
	// Language selection ends up here from a system setting, an environment
	// variable or a flag. Any of them may name something that does not exist.
	// The source language is the answer, not an empty string: a blank title
	// looks like a broken application, an English one does not.
	assert.Equal(t, catalog["app.title"], Lookup("klingon", "app.title"))
	assert.Nil(t, For("klingon"))
}

func TestSupportedLanguagesAreTheOnesWeShipped(t *testing.T) {
	assert.Equal(t, []string{English, LangRussian}, Supported())
	for _, language := range Supported() {
		if language == English {
			continue // the source language has no separate catalogue
		}
		assert.Empty(t, MissingFrom(language), "%s is incomplete", language)
	}
}

func TestKeysAreSorted(t *testing.T) {
	// Sorted so a diff of the source catalogue shows real changes rather than
	// Go's map iteration order.
	keys := Keys()
	assert.True(t, sortedAscending(keys), "keys are not sorted: %v", keys)
}

func missingFrom(catalogue Strings) []string {
	var missing []string
	for _, key := range Keys() {
		if text, ok := catalogue[key]; !ok || text == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func sortedAscending(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i-1] > values[i] {
			return false
		}
	}
	return true
}
