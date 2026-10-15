// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
)

// The settings dialog is the one place a person can reach what used to need a
// command line: storage, the htorrent gateway, the signaler — plus the few
// choices that already lived in settings.json.
//
// The form says what applies when, because half of these are read by the
// backend child at startup: a setting that looks applied and is not is worse
// than a setting that does not exist.

// mainWindow is where dialogs attach: the window this content lives in.
//
// The value built here is a fyne.CanvasObject, not a fyne.Window — the
// application owns the window — so the driver is the only honest source.
func mainWindow() fyne.Window {
	app := fyne.CurrentApp()
	if app == nil {
		return nil
	}
	windows := app.Driver().AllWindows()
	if len(windows) == 0 {
		return nil
	}
	return windows[0]
}

// applyTheme switches the application theme now.
func applyTheme(dark bool) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	if dark {
		app.Settings().SetTheme(theme.DarkTheme())
		return
	}
	app.Settings().SetTheme(theme.LightTheme())
}

// chooseFolderButton opens a folder chooser and hands the picked path to set.
func chooseFolderButton(tr func(string) string, parent fyne.Window, set func(string)) *widget.Button {
	return widget.NewButtonWithIcon(tr("settings.choose"), theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			set(uri.Path())
		}, parent)
	})
}

// chooseFileButton opens a file chooser and hands the picked path to set.
func chooseFileButton(tr func(string) string, parent fyne.Window, set func(string)) *widget.Button {
	return widget.NewButtonWithIcon(tr("settings.choose"), theme.FolderOpenIcon(), func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer func() { _ = reader.Close() }()
			set(reader.URI().Path())
		}, parent)
	})
}

// storageChoice is the storage row: memory or disk, and where on disk.
//
// One widget bundle serves both places it is asked — the first-launch question
// and the settings dialog — because two copies of "memory or disk" would drift
// apart in exactly the way the two answers differ.
type storageChoice struct {
	group        *widget.RadioGroup
	memory, disk string
	dir          *widget.Entry
	row          *fyne.Container
}

func newStorageChoice(current Settings, tr func(string) string, parent fyne.Window) *storageChoice {
	choice := &storageChoice{
		memory: tr("settings.storageMemory"),
		disk:   tr("settings.storageDisk"),
	}
	choice.group = widget.NewRadioGroup([]string{choice.memory, choice.disk}, nil)
	choice.group.Horizontal = true
	if current.UsesDiskStorage() {
		choice.group.SetSelected(choice.disk)
	} else {
		choice.group.SetSelected(choice.memory)
	}

	choice.dir = widget.NewEntry()
	dir := strings.TrimSpace(current.DataDir)
	if dir == "" {
		dir = DefaultDataDir()
	}
	choice.dir.SetText(dir)

	choice.row = container.NewBorder(nil, nil,
		widget.NewLabel(tr("settings.dataDir")),
		chooseFolderButton(tr, parent, choice.dir.SetText),
		choice.dir)

	// The handler is attached after the initial selection: wiring it up first
	// would read the radio's own setup as a person's choice.
	choice.group.OnChanged = func(string) { choice.sync() }
	choice.sync()
	return choice
}

// sync shows the directory row only when disk is chosen — and makes sure the
// field the person is about to look at is not empty while it is on screen.
func (c *storageChoice) sync() {
	if !c.diskSelected() {
		c.row.Hide()
		return
	}
	if strings.TrimSpace(c.dir.Text) == "" {
		c.dir.SetText(DefaultDataDir())
	}
	c.row.Show()
}

func (c *storageChoice) diskSelected() bool { return c.group.Selected == c.disk }

// apply maps the choice onto settings.
//
// Switching to memory keeps the directory: it is where the pieces will go if
// the person switches back, and erasing it would make the direction of the
// choice irreversible. Either answer settles the first-launch question.
func (c *storageChoice) apply(s Settings) Settings {
	if c.diskSelected() {
		s.StorageMode = StorageDisk
		s.DataDir = ExpandPath(c.dir.Text)
	} else {
		s.StorageMode = StorageMemory
	}
	s.StorageAsked = true
	return s
}

// settingsForm is the widget bundle of one editing session of the settings.
//
// The mapping back to Settings is a function, not the dialog callback, so it
// can be tested without a display — the step where a field is added to the
// form and forgotten in the mapping is otherwise invisible.
type settingsForm struct {
	tr func(string) string

	language *widget.Select
	dark     *widget.Check
	mpv      *widget.Entry

	storage *storageChoice

	gatewayURL    *widget.Entry
	gatewayUser   *widget.Entry
	gatewayPass   *widget.Entry
	gatewayCACert *widget.Entry

	signaler *widget.Entry
}

// The language options are not catalogue entries on purpose: each language
// names itself, and a Russian interface should offer "English".
const (
	languageOptionSystem  = ""
	languageOptionEnglish = "English"
	languageOptionRussian = "Русский"
)

func newSettingsForm(current Settings, tr func(string) string, parent fyne.Window) *settingsForm {
	f := &settingsForm{
		tr:       tr,
		mpv:      widget.NewEntry(),
		signaler: widget.NewEntry(),
	}

	f.language = widget.NewSelect(
		[]string{tr("settings.languageSystem"), languageOptionEnglish, languageOptionRussian}, nil)
	switch current.Language {
	case catalog.English:
		f.language.SetSelected(languageOptionEnglish)
	case catalog.LangRussian:
		f.language.SetSelected(languageOptionRussian)
	default:
		f.language.SetSelected(tr("settings.languageSystem"))
	}

	f.dark = widget.NewCheck(tr("settings.darkTheme"), nil)
	f.dark.SetChecked(current.DarkTheme)

	f.mpv.SetPlaceHolder(tr("settings.mpvPlaceholder"))
	f.mpv.SetText(current.PlayerBinary)

	f.storage = newStorageChoice(current, tr, parent)

	f.gatewayURL = widget.NewEntry()
	f.gatewayURL.SetText(current.GatewayURL)
	f.gatewayUser = widget.NewEntry()
	f.gatewayUser.SetText(current.GatewayUser)
	f.gatewayPass = widget.NewPasswordEntry()
	f.gatewayPass.SetText(current.GatewayPass)
	f.gatewayCACert = widget.NewEntry()
	f.gatewayCACert.SetText(current.GatewayCACert)

	// The placeholder is the default the backend child falls back to, so an
	// empty field reads as "the public one", not as "no signaler".
	f.signaler.SetPlaceHolder(DefaultSignaler)
	f.signaler.SetText(current.Signaler)

	return f
}

// apply maps the widgets back onto a copy of the settings.
func (f *settingsForm) apply(current Settings) Settings {
	s := current

	switch f.language.Selected {
	case languageOptionEnglish:
		s.Language = catalog.English
	case languageOptionRussian:
		s.Language = catalog.LangRussian
	default:
		s.Language = languageOptionSystem
	}

	s.DarkTheme = f.dark.Checked
	s.PlayerBinary = ExpandPath(f.mpv.Text)
	s = f.storage.apply(s)

	s.GatewayURL = strings.TrimSpace(f.gatewayURL.Text)
	s.GatewayUser = strings.TrimSpace(f.gatewayUser.Text)
	// The password is not trimmed: a password with spaces in it is a password.
	s.GatewayPass = f.gatewayPass.Text
	s.GatewayCACert = ExpandPath(f.gatewayCACert.Text)

	s.Signaler = strings.TrimSpace(f.signaler.Text)

	return s
}

// content assembles the form. It is tall, so it scrolls: a settings dialog
// whose bottom half is off the screen hides exactly the fields nobody looks
// for twice.
func (f *settingsForm) content(parent fyne.Window) fyne.CanvasObject {
	rows := container.NewVBox(
		widget.NewLabel(f.tr("settings.note")),
		widget.NewSeparator(),
		labelRow(f.tr("settings.language"), f.language),
		f.dark,
		labelRow(f.tr("settings.mpv"), container.NewBorder(nil, nil, nil,
			chooseFileButton(f.tr, parent, f.mpv.SetText), f.mpv)),
		widget.NewSeparator(),
		widget.NewLabel(f.tr("settings.storage")),
		f.storage.group,
		f.storage.row,
		widget.NewSeparator(),
		widget.NewLabel(f.tr("settings.gateway")),
		labelRow(f.tr("settings.gatewayURL"), f.gatewayURL),
		labelRow(f.tr("settings.gatewayUser"), f.gatewayUser),
		labelRow(f.tr("settings.gatewayPass"), f.gatewayPass),
		labelRow(f.tr("settings.gatewayCACert"), container.NewBorder(nil, nil, nil,
			chooseFileButton(f.tr, parent, f.gatewayCACert.SetText), f.gatewayCACert)),
		widget.NewSeparator(),
		labelRow(f.tr("settings.signaler"), f.signaler),
	)
	return container.NewVScroll(rows)
}

// labelRow puts a label beside a control, the shape the whole form uses.
func labelRow(label string, control fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widget.NewLabel(label), nil, control)
}

// showSettings opens the settings form over the main window.
func (w *window) showSettings() {
	if w.store == nil {
		return
	}
	parent := mainWindow()
	if parent == nil {
		return
	}

	// The store is read again here: the first-launch question saves before the
	// window ever looks at the settings again, and a form showing a stale
	// answer is worse than no form.
	w.mu.Lock()
	w.settings = w.store.Load()
	current := w.settings
	w.mu.Unlock()

	f := newSettingsForm(current, w.tr, parent)
	problem := widget.NewLabel("")
	problem.Hide()

	d := dialog.NewCustomWithoutButtons(w.tr("settings.title"),
		container.NewVBox(f.content(parent), problem), parent)

	save := widget.NewButtonWithIcon(w.tr("settings.save"), theme.ConfirmIcon(), func() {
		updated := f.apply(current)
		if key := updated.Problem(); key != "" {
			problem.SetText(w.tr(key))
			problem.Show()
			return
		}
		if err := w.store.Save(updated); err != nil {
			problem.SetText(fmt.Sprintf(w.tr("settings.saveFailed"), err.Error()))
			problem.Show()
			return
		}

		// What can apply now, applies now: the theme redraws the window, and
		// the signaler is read when the next room is joined. The rest is read
		// at startup, which is what the note on the form says.
		w.mu.Lock()
		w.settings = updated
		w.mu.Unlock()
		applyTheme(updated.DarkTheme)
		w.setStatus(w.tr("settings.saved"))
		d.Hide()
	})
	save.Importance = widget.HighImportance

	cancel := widget.NewButton(w.tr("settings.cancel"), func() { d.Hide() })
	d.SetButtons([]fyne.CanvasObject{cancel, save})
	d.Resize(fyne.NewSize(640, 620))
	d.Show()
}

// AskAboutStorage shows the first-launch question about where torrent pieces
// live. After it has been answered it does nothing, so the application can
// call it on every launch.
//
// The question is asked rather than a default assumed because the two answers
// are genuinely different lives for a film: in memory nothing is left on disk
// and a restart starts over, on disk a restart resumes. The backend is already
// running by the time the window exists, so this launch keeps to memory either
// way; the question says the answer applies at the next launch (idea-0081).
func AskAboutStorage(store *SettingsStore, parent fyne.Window) {
	if store == nil || parent == nil {
		return
	}
	settings := store.Load()
	if !settings.NeedsStorageChoice() {
		return
	}
	newStorageQuestion(store, settings, parent).dialog.Show()
}

// storageQuestion is the first-launch dialog with its parts, so the save path
// can be driven the way a person drives it.
type storageQuestion struct {
	dialog *dialog.CustomDialog
	choice *storageChoice
	later  *widget.Button
	save   *widget.Button
}

func newStorageQuestion(store *SettingsStore, settings Settings, parent fyne.Window) *storageQuestion {
	language := PreferredLanguage("", settings.Language)
	tr := func(key string) string { return catalog.Lookup(language, key) }

	question := &storageQuestion{choice: newStorageChoice(settings, tr, parent)}

	problem := widget.NewLabel("")
	problem.Hide()

	body := container.NewVBox(
		widget.NewLabel(tr("storage.question.body")),
		question.choice.group,
		question.choice.row,
		problem,
	)

	question.dialog = dialog.NewCustomWithoutButtons(tr("storage.question.title"), body, parent)
	question.save = widget.NewButtonWithIcon(tr("settings.save"), theme.ConfirmIcon(), func() {
		updated := question.choice.apply(settings)
		if key := updated.Problem(); key != "" {
			problem.SetText(tr(key))
			problem.Show()
			return
		}
		if err := store.Save(updated); err != nil {
			problem.SetText(fmt.Sprintf(tr("settings.saveFailed"), err.Error()))
			problem.Show()
			return
		}
		question.dialog.Hide()
	})
	question.save.Importance = widget.HighImportance

	// "Later" is not an answer: the question returns on the next launch rather
	// than a memory default being written as if it had been chosen.
	question.later = widget.NewButton(tr("storage.later"), func() { question.dialog.Hide() })
	question.dialog.SetButtons([]fyne.CanvasObject{question.later, question.save})
	question.dialog.Resize(fyne.NewSize(560, 320))

	return question
}
