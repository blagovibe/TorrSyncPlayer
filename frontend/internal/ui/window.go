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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
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
	fileList  *widget.List
	status    *widget.Label
	entry     *widget.Entry
	addButton *widget.Button

	mu       sync.RWMutex
	torrents []Torrent
	current  *Torrent
	files    []fileEntry
	mpv      *player.Player
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

	// The file list sits next to the torrent list: picking a torrent answers
	// "which file", and that question is asked every time.
	w.fileList = widget.NewList(
		func() int {
			w.mu.RLock()
			defer w.mu.RUnlock()
			return len(w.files)
		},
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			w.mu.RLock()
			defer w.mu.RUnlock()
			if id < 0 || id >= len(w.files) {
				return
			}
			obj.(*widget.Label).SetText(fmt.Sprintf("%d. %s (%s)",
				w.files[id].Index, w.files[id].Name, humanSize(w.files[id].Size)))
		},
	)
	w.fileList.OnSelected = func(id widget.ListItemID) {
		w.mu.RLock()
		var f fileEntry
		if id >= 0 && id < len(w.files) {
			f = w.files[id]
		}
		w.mu.RUnlock()
		w.playFile(f)
	}

	content := container.NewBorder(
		toolbar, w.status, nil, nil,
		container.NewHSplit(container.NewScroll(w.list), container.NewScroll(w.fileList)),
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

// play starts the torrent in mpv.
//
// mpv runs as its own process and gets the access token as an HTTP header, so
// the media endpoint is a protected route and no ticket travels in the URL
// (idea-0066). It gets the file list first: which file to play is the user's
// choice, and a torrent is routinely a season of episodes rather than a film.
func (w *window) play(t *Torrent) {
	if t == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var listed struct {
			Files []fileEntry `json:"files"`
		}
		if err := w.client.Get(ctx, api.Data, "/api/v1/torrents/"+t.ID+"/files", &listed); err != nil {
			w.setStatus("Could not read the torrent: " + err.Error())
			return
		}
		if len(listed.Files) == 0 {
			w.setStatus("The torrent has no files yet")
			return
		}

		w.presentFiles(t, listed.Files)
	}()
}

// presentFiles shows the file list and plays the first video once one is picked.
func (w *window) presentFiles(t *Torrent, files []fileEntry) {
	// Fyne's list draws from the length and update callbacks, so the data is
	// just the slice behind them: swap it, refresh, done.
	w.mu.Lock()
	w.current = t
	w.files = files
	w.mu.Unlock()

	w.fileList.Refresh()
	w.setStatus("Choose a file to play")
}

// playFile selects the file on the data plane and hands the stream to mpv.
func (w *window) playFile(f fileEntry) {
	w.mu.RLock()
	torrent := w.current
	w.mu.RUnlock()
	if torrent == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		var selected struct {
			Index int    `json:"index"`
			Name  string `json:"name"`
		}
		body := map[string]int{"fileIndex": f.Index}
		if err := w.client.Post(ctx, api.Data, "/api/v1/torrents/"+torrent.ID+"/select", body, &selected); err != nil {
			w.setStatus("Could not select the file: " + err.Error())
			return
		}

		p, err := w.startPlayer()
		if err != nil {
			w.setStatus("Could not start mpv: " + err.Error())
			return
		}

		if err := p.Play(ctx, w.client.StreamURL(torrent.ID)); err != nil {
			w.setStatus("Could not play: " + err.Error())
			return
		}
		w.setStatus("Playing: " + f.Name)
	}()
}

// startPlayer launches mpv once and reuses it for the rest of the run.
func (w *window) startPlayer() (*player.Player, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.mpv != nil {
		return w.mpv, nil
	}

	header, err := player.FormatHeader("X-Access-Token", w.client.DataToken())
	if err != nil {
		return nil, err
	}

	mpv, err := player.Start(context.Background(), player.Options{
		SocketPath: filepath.Join(os.TempDir(), fmt.Sprintf("torrsync-%d.sock", os.Getpid())),
	}, []string{header})
	if err != nil {
		return nil, err
	}
	w.mpv = mpv
	return mpv, nil
}

// fileEntry is one row of the file list.
type fileEntry struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
}

// humanSize formats a byte count for the file list.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit && exp < 3; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
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
