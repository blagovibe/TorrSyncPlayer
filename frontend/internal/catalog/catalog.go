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
	"app.title":       "TorrSyncPlayer",
	"status.starting": "Starting…",
	"status.ready":    "Ready",
	"status.torrents": "%d torrents",
	"status.torrent":  "1 torrent",
	"status.added":    "Added: %s",
	"status.nothing":  "Nothing to add",
	"status.couldNot": "Could not: %s",
	"status.playing":  "Playing: %s",
	"status.selected": "Selected: %s",
	"status.choose":   "Choose a file to play",
	"status.pick":     "Pick a torrent first: the room is bound to one",
	"status.joined":   "Joined the room",
	"status.created":  "Room opened — the code is ready to copy",
	"status.codeRead": "Invitation code read — press Join",
	"status.notIn":    "You are not in a room yet",
	"status.unreach":  "Backend unreachable: %s",

	// The playback path. These were the last strings written to the status
	// line as plain literals, so a person reading the Russian interface read
	// every failure in English (idea-0073).
	"status.joinFail":        "Could not join: %s",
	"status.roomWatching":    "The room is watching %s — adding it to your backend",
	"status.roomReadFail":    "Could not read the room's torrent: %s",
	"status.roomFileMissing": "The file the host is playing is not in this torrent: %s",
	"status.roomJoining":     "The room is playing %s — joining in",
	"status.torrentReadFail": "Could not read the torrent: %s",
	"status.torrentNoFiles":  "The torrent has no files yet",
	"status.fileSelectFail":  "Could not select the file: %s",
	"status.mpvStartFail":    "Could not start mpv: %s",
	"status.playFail":        "Could not play: %s",
	"status.trackChosen":     "Track: %s",
	"status.trackFailed":     "Could not switch the track: %s",
	"status.preparing":       "Fetching %s — %s / %s (%d%%, %s, peers: %d)",
	"status.peerJoined":      "%s joined",
	"status.peerLeft":        "%s left",
	"status.connReady":       "DHT ready — %d peers (%d direct, %d relayed), you %s",
	"status.connWaiting":     "Finding the room — DHT not ready yet",
	"dialog.error":           "Error",

	"room.label":          "In the room: %s",
	"room.host":           "(host)",
	"room.buffering":      "buffering",
	"room.defaultName":    "Room",
	"torrent.loading":     "Loading",
	"torrent.downloading": "Downloading",
	"torrent.seeding":     "Seeding",
	"action.add":          "Add",
	"action.refresh":      "Refresh",
	"action.join":         "Join",
	"action.createRoom":   "Open a room",
	"action.paste":        "Paste code",
	"action.copy":         "Copy code",
	"action.open":         "Open",
	"action.quit":         "Quit",
	"action.ok":           "OK",
	"action.shortcuts":    "Shortcuts",
	"shortcuts.title":     "Keyboard shortcuts",
	"shortcut.focus":      "Focus the magnet field",
	"shortcut.refresh":    "Refresh the torrent list",
	"shortcut.settings":   "Open settings",
	"field.invite":        "paste the invitation code",

	// The track choosers. Their keys existed in the window but not in the
	// catalogue, so the row and its "Off" entry rendered empty.
	"track.audio":   "Audio",
	"track.sub":     "Subtitles",
	"track.subfile": "Sub file",
	"track.off":     "Off",

	// The settings dialog. The note is the honest half of every choice in it:
	// theme and rendezvous apply at once, the rest is read at the next launch.
	// The two problem keys mirror the backend's own startup checks, so the
	// person reads the refusal in the form rather than in a log after a
	// restart that did not happen.
	"action.settings":               "Settings",
	"settings.title":                "Settings",
	"settings.note":                 "Theme and rendezvous apply at once; the other choices apply at the next launch.",
	"settings.language":             "Interface language",
	"settings.languageSystem":       "System language",
	"settings.darkTheme":            "Dark theme",
	"settings.mpv":                  "mpv executable",
	"settings.mpvPlaceholder":       "mpv on PATH",
	"settings.storage":              "Torrent pieces",
	"settings.storageMemory":        "In memory",
	"settings.storageDisk":          "On disk",
	"settings.dataDir":              "Directory for pieces",
	"settings.choose":               "Choose…",
	"settings.gateway":              "htorrent gateway (remoting)",
	"settings.gatewayURL":           "Gateway address",
	"settings.gatewayUser":          "User",
	"settings.gatewayPass":          "Password",
	"settings.gatewayCACert":        "CA certificate",
	"settings.bootstrap":            "Bootstrap",
	"settings.relays":               "Fallback relays",
	"settings.relaysPlaceholder":    "empty: discover relays over the DHT",
	"settings.save":                 "Save",
	"settings.cancel":               "Cancel",
	"settings.saved":                "Settings saved",
	"settings.saveFailed":           "Could not save the settings: %s",
	"settings.problem.diskDir":      "Disk storage needs a directory for the pieces",
	"settings.problem.gatewayLogin": "The gateway needs both a user and a password",

	// The first-launch question about storage (idea-0081).
	"storage.question.title": "Where should the film pieces live?",
	"storage.question.body": "In memory nothing is left on disk, and a restart starts the download over. " +
		"On disk the download resumes. The answer applies at the next launch and can be changed in the settings.",
	"storage.later": "Later",
}

// Russian is the Russian catalogue.
//
// A missing entry is left empty on purpose: an empty translation means "fall
// back to the source language", which is what a half-finished catalogue should
// do rather than showing an empty button.
//
// #nosec G101 -- same as the source catalogue above: UI text keyed by name.
var Russian = Strings{
	"app.title":                     "TorrSyncPlayer",
	"status.starting":               "Запуск…",
	"status.ready":                  "Готово",
	"status.torrents":               "торрентов: %d",
	"status.torrent":                "1 торрент",
	"status.added":                  "Добавлено: %s",
	"status.nothing":                "Нечего добавлять",
	"status.couldNot":               "Не удалось: %s",
	"status.playing":                "Смотрим: %s",
	"status.selected":               "Выбрано: %s",
	"status.choose":                 "Выберите файл для воспроизведения",
	"status.pick":                   "Сначала выберите торрент: комната привязана к одному",
	"status.joined":                 "Вы вошли в комнату",
	"status.created":                "Комната открыта — код готов к копированию",
	"status.codeRead":               "Код приглашения прочитан — нажмите «Войти»",
	"status.notIn":                  "Вы ещё не в комнате",
	"status.unreach":                "Бэкенд недоступен: %s",
	"status.joinFail":               "Не удалось войти: %s",
	"status.roomWatching":           "Комната смотрит %s — добавляю торрент в ваш бэкенд",
	"status.roomReadFail":           "Не удалось прочитать торрент комнаты: %s",
	"status.roomFileMissing":        "Файла, который воспроизводит хост, нет в этом торренте: %s",
	"status.roomJoining":            "Комната смотрит %s — подключаюсь",
	"status.torrentReadFail":        "Не удалось прочитать торрент: %s",
	"status.torrentNoFiles":         "В торренте пока нет файлов",
	"status.fileSelectFail":         "Не удалось выбрать файл: %s",
	"status.mpvStartFail":           "Не удалось запустить mpv: %s",
	"status.playFail":               "Не удалось воспроизвести: %s",
	"status.trackChosen":            "Дорожка: %s",
	"status.trackFailed":            "Не удалось переключить дорожку: %s",
	"status.preparing":              "Докачка %s — %s / %s (%d%%, %s, пиры: %d)",
	"status.peerJoined":             "Присоединился: %s",
	"status.peerLeft":               "Ушёл: %s",
	"status.connReady":              "DHT готов — пиров: %d (напрямую %d, через реле %d), вы %s",
	"status.connWaiting":            "Ищем комнату — DHT пока не готов",
	"dialog.error":                  "Ошибка",
	"room.label":                    "В комнате: %s",
	"room.host":                     "(хост)",
	"room.buffering":                "буферизация",
	"room.defaultName":              "Комната",
	"torrent.loading":               "Загрузка",
	"torrent.downloading":           "Скачивание",
	"torrent.seeding":               "Раздача",
	"action.add":                    "Добавить",
	"action.refresh":                "Обновить",
	"action.join":                   "Войти",
	"action.createRoom":             "Открыть комнату",
	"action.paste":                  "Вставить код",
	"action.copy":                   "Скопировать код",
	"action.open":                   "Открыть",
	"action.quit":                   "Выход",
	"action.ok":                     "ОК",
	"action.shortcuts":              "Шорткаты",
	"shortcuts.title":               "Горячие клавиши",
	"shortcut.focus":                "Фокус в поле magnet-ссылки",
	"shortcut.refresh":              "Обновить список торрентов",
	"shortcut.settings":             "Открыть настройки",
	"field.invite":                  "вставьте код приглашения",
	"track.audio":                   "Аудио",
	"track.sub":                     "Субтитры",
	"track.subfile":                 "Файл субтитров",
	"track.off":                     "Выкл.",
	"action.settings":               "Настройки",
	"settings.title":                "Настройки",
	"settings.note":                 "Тема и рандеву применяются сразу, остальное — при следующем запуске.",
	"settings.language":             "Язык интерфейса",
	"settings.languageSystem":       "Системный язык",
	"settings.darkTheme":            "Тёмная тема",
	"settings.mpv":                  "Исполняемый файл mpv",
	"settings.mpvPlaceholder":       "mpv из PATH",
	"settings.storage":              "Куски торрентов",
	"settings.storageMemory":        "В памяти",
	"settings.storageDisk":          "На диске",
	"settings.dataDir":              "Каталог для кусков",
	"settings.choose":               "Выбрать…",
	"settings.gateway":              "Шлюз htorrent (ремотинг)",
	"settings.gatewayURL":           "Адрес шлюза",
	"settings.gatewayUser":          "Логин",
	"settings.gatewayPass":          "Пароль",
	"settings.gatewayCACert":        "CA-сертификат",
	"settings.bootstrap":            "Бутстрап",
	"settings.relays":               "Запасные релеи",
	"settings.relaysPlaceholder":    "пусто: искать релеи через DHT",
	"settings.save":                 "Сохранить",
	"settings.cancel":               "Отмена",
	"settings.saved":                "Настройки сохранены",
	"settings.saveFailed":           "Не удалось сохранить настройки: %s",
	"settings.problem.diskDir":      "Для хранения на диске нужен каталог",
	"settings.problem.gatewayLogin": "Шлюзу нужны и логин, и пароль",
	"storage.question.title":        "Где хранить куски фильмов?",
	"storage.question.body": "В памяти ничего не остаётся на диске, но после перезапуска загрузка начнётся заново. " +
		"На диске она продолжится. Ответ применится при следующем запуске и его можно поменять в настройках.",
	"storage.later": "Позже",
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
