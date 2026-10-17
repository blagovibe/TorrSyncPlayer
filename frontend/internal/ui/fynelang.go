// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
)

// Fyne's own widgets speak through its translation engine, and the bundle
// it ships does not carry our languages. Every `widget.NewSelect` asks it
// for "(Select one)" in its constructor — before we can set a placeholder
// of our own — and a missing answer is logged as an error every single
// time. A person running the Russian interface got a log full of somebody
// else's failure (bug-0110).
//
// Registering the string once at startup is quieter than fighting the
// constructor, and it also fixes every other Fyne string we may reach by
// accident later. Each catalogue key carries its own translations, so this
// table is only what Fyne itself asks for by default.
func InstallFyneTranslations() {
	for locale, entries := range map[string]map[string]string{
		"ru": {"(Select one)": "Выберите"},
		"en": {"(Select one)": "Select one"},
	} {
		body := "{"
		first := true
		for key, value := range entries {
			if !first {
				body += ","
			}
			first = false
			body += fmt.Sprintf("\n  %q: %q", key, value)
		}
		body += "\n}"
		if err := lang.AddTranslationsForLocale([]byte(body), fyne.Locale(locale)); err != nil {
			// Nothing to do about it: the string falls back to English and
			// Fyne logs on its own. Failing to start the application over a
			// placeholder would be absurd.
			fyne.LogError("register Fyne translations", err)
		}
	}
}
