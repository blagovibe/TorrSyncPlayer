// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package ui draws the application window.
//
// Kept free of network and process logic on purpose: everything it needs
// arrives through the client it is handed, so a window can be exercised without
// starting a backend.
package ui

import (
	"context"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
)

// Torrent is one row of the list, as the data plane reports it.
type Torrent struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
}

// window is the top-level window.
type window struct {
	client   *api.Client
	clientID string

	list      *widget.List
	status    *widget.Label
	entry     *widget.Entry
	addButton *widget.Button

	mu       sync.RWMutex
	torrents []Torrent
}

// NewWindow builds the main window around a client.
func NewWindow(client *api.Client, clientID string) fyne.CanvasObject {
	w := &window{
		client:   client,
		clientID: clientID,
		status:   widget.NewLabelWithStyle("Starting…", fyne.TextAlignLeading, fyne.TextStyle{}),
	}

	w.entry = widget.NewEntry()
	w.entry.SetPlaceHolder("magnet:?xt=urn:btih:…")
	w.entry.OnSubmitted = func(string) { w.addTorrent() }

	w.addButton = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), w.addTorrent)
	w.addButton.Importance = widget.HighImportance

	w.list = widget.NewList(
		func() int {
			w.mu.RLock()
			defer w.mu.RUnlock()
			return len(w.torrents)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			w.mu.RLock()
			defer w.mu.RUnlock()
			if id < 0 || id >= len(w.torrents) {
				return
			}
			label := obj.(*widget.Label)
			label.SetText(w.torrents[id].Name)
		},
	)
	w.list.OnSelected = func(id widget.ListItemID) { w.play(w.item(id)) }

	toolbar := container.NewHBox(
		w.entry,
		w.addButton,
		widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), w.refresh),
	)

	content := container.NewBorder(
		toolbar, w.status, nil, nil,
		container.NewScroll(w.list),
	)

	go w.poll()

	return container.NewStack(content)
}

func (w *window) item(id widget.ListItemID) *Torrent {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if id < 0 || id >= len(w.torrents) {
		return nil
	}
	t := w.torrents[id]
	return &t
}

// play is where playback is wired in. Until the player exists the row is
// selectable and nothing more, which is deliberate: half a player that looks
// finished is worse than none.
func (w *window) play(t *Torrent) {
	if t == nil {
		return
	}
	w.setStatus("Selected: " + t.Name)
}

func (w *window) addTorrent() {
	magnet := w.entry.Text
	if magnet == "" {
		w.setStatus("Nothing to add")
		return
	}

	// The data plane: the magnet goes into this application's own backend.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		var out Torrent
		body := map[string]string{"magnetUri": magnet}
		if err := w.client.Post(ctx, api.Data, "/api/v1/torrents", body, &out); err != nil {
			w.setStatus("Could not add: " + err.Error())
			return
		}
		w.entry.SetText("")
		w.setStatus("Added: " + out.Name)
		w.refresh()
	}()
}

// poll keeps the list current.
//
// A ticker rather than an answer to each request: a torrent's progress moves on
// its own, and the backend has no event to push it.
func (w *window) poll() {
	w.refresh()
	for range time.NewTicker(2 * time.Second).C {
		w.refresh()
	}
}

func (w *window) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out struct {
		Torrents []Torrent `json:"torrents"`
	}
	if err := w.client.Get(ctx, api.Data, "/api/v1/torrents", &out); err != nil {
		w.setStatus("Backend unreachable: " + err.Error())
		return
	}

	w.mu.Lock()
	w.torrents = out.Torrents
	w.mu.Unlock()

	w.list.Refresh()
	w.setStatus(plural(len(out.Torrents), "torrent", "torrents"))
}

func (w *window) setStatus(text string) {
	w.status.SetText(text)
}

// plural avoids "1 torrents", which is the kind of small wrongness that makes
// an interface feel unfinished.
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// The icons are Fyne's own. Shipping a parallel set would be a second thing to
// translate and keep in step, for no gain.
