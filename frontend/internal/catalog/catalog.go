// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package catalog holds the interface strings.
//
// There is no `lupdate` for Fyne the way there is for Qt, so the catalogues are
// maintained by hand. That is only safe if something checks them, which is what
// this package is for: the strings a person can see are listed once, and a test
// fails when a catalogue drifts from that list.
package catalog

import "sort"

// Strings is a language's catalogue.
type Strings map[string]string

// Languages the application ships with. English is the source language, so an
// empty entry means "show the key" rather than "missing translation".
const (
	English     = "en"
	LangRussian = "ru"
)

// Supported lists the languages with a catalogue.
func Supported() []string {
	out := []string{English, LangRussian}
	sort.Strings(out)
	return out
}

// catalog is the source language and every string a person can see.
//
// A key missing from here is a string the person sees with a lookup key in it,
// so the test that follows fails when one is added to the UI without being
// added here.
//
// #nosec G101 -- this map is UI text, not configuration. gosec's rule matches
// any key containing "password" that holds a string value; here every value
// is a label. The alternative is a key so vague it stops describing its
// field, and a vague key is what makes a catalogue rot.
var catalog = map[string]string{
	"app.title":         "TorrSyncPlayer",
	"status.starting":   "Starting…",
	"status.ready":      "Ready",
	"status.torrents":   "%d torrents",
	"status.torrent":    "1 torrent",
	"status.added":      "Added: %s",
	"status.nothing":    "Nothing to add",
	"status.couldNot":   "Could not: %s",
	"status.playing":    "Playing: %s",
	"status.selected":   "Selected: %s",
	"status.choose":     "Choose a file to play",
	"status.pick":       "Pick a torrent first: the room is bound to one",
	"status.joined":     "Joined the room",
	"status.created":    "Room opened — the code is ready to copy",
	"status.codeRead":   "Invitation code read — press Join",
	"status.notIn":      "You are not in a room yet",
	"status.unreach":    "Backend unreachable: %s",
	"room.label":        "In the room: %s",
	"room.host":         "(host)",
	"room.buffering":    "buffering",
	"room.defaultName":  "Room",
	"action.add":        "Add",
	"action.refresh":    "Refresh",
	"action.join":       "Join",
	"action.createRoom": "Open a room",
	"action.paste":      "Paste code",
	"action.copy":       "Copy code",
	"action.open":       "Open",
	"action.quit":       "Quit",
	"field.invite":      "paste the invitation code",
}

// Russian is the Russian catalogue.
//
// A missing entry is left empty on purpose: an empty translation means "fall
// back to the source language", which is what a half-finished catalogue should
// do rather than showing an empty button.
//
// #nosec G101 -- same as the source catalogue above: UI text keyed by name.
var Russian = Strings{
	"app.title":         "TorrSyncPlayer",
	"status.starting":   "Запуск…",
	"status.ready":      "Готово",
	"status.torrents":   "торрентов: %d",
	"status.torrent":    "1 торрент",
	"status.added":      "Добавлено: %s",
	"status.nothing":    "Нечего добавлять",
	"status.couldNot":   "Не удалось: %s",
	"status.playing":    "Смотрим: %s",
	"status.selected":   "Выбрано: %s",
	"status.choose":     "Выберите файл для воспроизведения",
	"status.pick":       "Сначала выберите торрент: комната привязана к одному",
	"status.joined":     "Вы вошли в комнату",
	"status.created":    "Комната открыта — код готов к копированию",
	"status.codeRead":   "Код приглашения прочитан — нажмите «Войти»",
	"status.notIn":      "Вы ещё не в комнате",
	"status.unreach":    "Бэкенд недоступен: %s",
	"room.label":        "В комнате: %s",
	"room.host":         "(хост)",
	"room.buffering":    "буферизация",
	"room.defaultName":  "Комната",
	"action.add":        "Добавить",
	"action.refresh":    "Обновить",
	"action.join":       "Войти",
	"action.createRoom": "Открыть комнату",
	"action.paste":      "Вставить код",
	"action.copy":       "Скопировать код",
	"action.open":       "Открыть",
	"action.quit":       "Выход",
	"field.invite":      "вставьте код приглашения",
}

// For returns the catalogue for a language, falling back to the source one.
func For(language string) Strings {
	if language == LangRussian {
		return Russian
	}
	return nil
}

// Keys returns every key in the source catalogue, sorted.
func Keys() []string {
	out := make([]string, 0, len(catalog))
	for key := range catalog {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// Lookup returns the string for a key in a language, falling back to the
// source language.
func Lookup(language, key string) string {
	if catalogue := For(language); catalogue != nil {
		if text, ok := catalogue[key]; ok && text != "" {
			return text
		}
	}
	return catalog[key]
}

// MissingFrom lists keys the given language has not translated.
//
// An empty entry counts as missing on purpose: it renders as the source
// language, so shipping it as "translated" would hide the work still to do.
func MissingFrom(language string) []string {
	catalogue := For(language)
	if catalogue == nil {
		return nil
	}

	var missing []string
	for _, key := range Keys() {
		if text, ok := catalogue[key]; !ok || text == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

// Extra lists keys a language carries that the source catalogue does not.
//
// They can never be shown, and they hide the fact that a key was renamed: the
// translation keeps looking finished while nothing uses it.
func Extra(language string) []string {
	catalogue := For(language)
	if catalogue == nil {
		return nil
	}

	var extra []string
	for key := range catalogue {
		if _, ok := catalog[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	return extra
}
