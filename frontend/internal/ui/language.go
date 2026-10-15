// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"strings"

	"fyne.io/fyne/v2/lang"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
)

// PreferredLanguage resolves the interface language: the --lang flag, then the
// saved choice, then the system's own locale, and English when nothing in the
// chain is a language the application speaks.
//
// The flag used to reach only the tray: the window kept reading the store, so
// --lang changed the tray menu and nothing else. A flag that shows up in one
// place and not another is worse than no flag — --help promises it.
func PreferredLanguage(flagValue, setting string) string {
	for _, candidate := range []string{flagValue, setting, systemLanguage()} {
		if language := supportedLanguage(candidate); language != "" {
			return language
		}
	}
	return catalog.English
}

// supportedLanguage names the catalogue a candidate locale belongs to, or ""
// when there is none. "ru", "ru-RU" and "ru_RU.UTF-8" are all Russian, and an
// unsupported locale falls through to the next candidate rather than pinning
// the interface to a language nobody offered.
func supportedLanguage(candidate string) string {
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	candidate, _, _ = strings.Cut(candidate, "@") // modifiers: sr_RS@latin
	candidate, _, _ = strings.Cut(candidate, ".") // encoding: ru_RU.UTF-8
	candidate = strings.ReplaceAll(candidate, "_", "-")
	base, _, _ := strings.Cut(candidate, "-")
	for _, language := range catalog.Supported() {
		if language == base {
			return base
		}
	}
	return ""
}

// systemLocale is the desktop's locale, or "" when it cannot be read. Fyne
// already knows how to ask each platform; guessing from $LANG alone would be
// right on one platform out of three.
//
// A variable, not a direct call, so a test can put the machine in Russian
// without being on a Russian machine — the fallback chain has three links and
// only one of them is reachable otherwise.
var systemLocale = func() string { return string(lang.SystemLocale()) }

// systemLanguage is where the chain asks the desktop.
func systemLanguage() string {
	return systemLocale()
}
