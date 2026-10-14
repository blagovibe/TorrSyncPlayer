// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Command TorrSyncPlayer is the desktop client.
//
// It starts its own Go backend as a child process and talks to it over HTTP.
// Nothing here holds torrent logic: the backend is the torrent client, and this
// is a window onto it.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/backend"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/ui"
)

func main() {
	var (
		serverURL   = flag.String("s", "server-url", "Server URL (e.g., http://localhost:8889)")
		accessToken = flag.String("a", "access-token", "Access token for API authentication")
		clientID    = flag.String("c", "client-id", "Client identifier for P2P room (auto-generated if not provided)")
		lang        = flag.String("lang", "", "Interface language: ru, en")
		noTray      = flag.Bool("no-tray", false, "Don't use system tray")
		darkTheme   = flag.Bool("dark-theme", false, "Use dark theme")
	)
	flag.Parse()

	if err := run(runOptions{
		ServerURL:   *serverURL,
		AccessToken: *accessToken,
		ClientID:    *clientID,
		Language:    *lang,
		NoTray:      *noTray,
		DarkTheme:   *darkTheme,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "TorrSyncPlayer:", err)
		os.Exit(1)
	}
}

type runOptions struct {
	ServerURL   string
	AccessToken string
	ClientID    string
	Language    string
	NoTray      bool
	DarkTheme   bool
}

func run(opts runOptions) error {
	clientID := opts.ClientID
	if clientID == "" {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return fmt.Errorf("generate client id: %w", err)
		}
		clientID = hex.EncodeToString(raw)
	}

	// The backend is started unconditionally, even with --server-url. Every
	// participant runs its own torrent client and pulls the media from the
	// swarm themselves; the room's server is a rendezvous for sync commands
	// and the roster only. A client with no backend of its own could not
	// download anything, so --server-url configures the control plane and no
	// longer replaces the local one.
	port, err := backend.PickFreePort()
	if err != nil {
		return err
	}
	token, err := backend.GenerateToken()
	if err != nil {
		return err
	}

	appDir, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate application directory: %w", err)
	}
	executable, err := backend.FindBackend(filepath.Dir(appDir))
	if err != nil {
		return err
	}

	child, err := backend.Start(executable, port, token)
	if err != nil {
		return err
	}
	defer child.Stop()

	dataURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Without --server-url the client is the host, and its own backend serves
	// the room it creates.
	controlURL, controlToken := dataURL, token
	if opts.ServerURL != "" {
		controlURL, controlToken = opts.ServerURL, opts.AccessToken
	}

	client := api.New(dataURL, token, controlURL, controlToken)
	client.SetClientID(clientID)

	if err := waitForBackend(client, child, 10*time.Second); err != nil {
		return err
	}

	a := app.NewWithID("com.blagovibe.torrsyncplayer")
	a.Settings().SetTheme(theme.LightTheme())

	// Settings are read before the window is built: the language is applied to
	// the texts as they are created, and setting it afterwards would leave half
	// the interface in the previous language.
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.TempDir()
	}
	store := ui.NewSettingsStore(ui.DefaultPath(configDir))
	settings := store.Load()
	if opts.Language != "" {
		settings.Language = opts.Language
	}
	if opts.DarkTheme || settings.DarkTheme {
		a.Settings().SetTheme(theme.DarkTheme())
	}

	window := a.NewWindow("TorrSyncPlayer")
	window.SetContent(ui.NewWindow(client, clientID, store))
	window.Resize(fyne.NewSize(1100, 700))

	// Closing the window hides it while the tray is there: the backend is a
	// child process, so downloads continue whether or not anyone is looking.
	// Without a tray there would be no way back to the window, so closing it
	// really does end the application.
	//
	// --no-tray is honoured rather than merely accepted: a flag that does
	// nothing is worse than a missing one, because the person reading --help
	// believes it did something.
	var tray *ui.Tray
	if !opts.NoTray {
		tray = ui.NewTray(settings.Language,
			func() { window.Show(); window.RequestFocus() },
			a.Quit)
		defer tray.Shutdown()
	}
	ui.CloseToTray(window, tray)

	go func() {
		// Quit when the backend dies on its own: a window whose backend is
		// gone shows a list that will never update and reports errors nobody
		// can act on. The reason goes out with it, because the window closing
		// on its own explains nothing.
		<-child.Exited()
		fmt.Fprintln(os.Stderr, backend.DescribeExit(child.ExitError()))
		a.Quit()
	}()

	window.ShowAndRun()
	return nil
}

// waitForBackend blocks until the backend answers, or the deadline passes.
//
// The port is free at the moment it is picked but the child still has to bind
// it; polling is simpler and no worse than a readiness pipe here.
// waitForBackend polls until the backend answers.
//
// It watches the process as well as the clock, because a backend that died on
// startup cannot become ready and waiting out the full timeout turns a precise
// failure into ten seconds of silence followed by the wrong explanation. That is
// not hypothetical: a port already in use produced exactly that, and the only
// clue to the real cause was a line of output nobody was reading yet.
func waitForBackend(client *api.Client, child *backend.Process, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var lastErr error
	for {
		var out map[string]any
		if err := client.Get(ctx, api.Data, "/api/v1/version", &out); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("backend did not become ready in %s: %w", timeout, lastErr)
		case <-child.Exited():
			return errors.New(backend.DescribeExit(child.ExitError()))
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func init() {
	// Ctrl-C and SIGTERM must not leave the backend running as an orphan.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		os.Exit(0)
	}()
}
