// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
)

// The application writes one log file next to its settings. Everything a
// person would want to hand over after a failure fits in it: what this
// process is, what mpv it found, what the backend reports as its version,
// and what went wrong in the window. Being a file it survives the packaged
// launchers, where stderr is lost with the window.
//
// The file is not a rotation system: it is appended to, and trimmed by the
// diagnostics reader to the tail that is worth pasting.

const diagnosticsTailBytes = 16 << 10

// mpvLookup resolves an mpv the way mpvExecutable does. A package variable
// so the diagnostics tests can pin a box with no mpv on it — the shape of
// the failure a person phones in about — without rewriting PATH or moving
// real files.
var mpvLookup = mpvExecutable

// DiagnosticsLogPath is the log's home: beside settings.json, which the
// first-launch question and the settings dialog already know how to find.
// An empty settings path means the application keeps nothing, and neither
// does the log.
func DiagnosticsLogPath(settingsPath string) string {
	if settingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(settingsPath), "torrsyncplayer.log")
}

// WriteDiagnostic appends one timestamped line to the log. A failure is not
// worth failing anything over: the log is a courtesy to the future bug
// report, not a correctness requirement.
func WriteDiagnostic(settingsPath, line string) error {
	path := DiagnosticsLogPath(settingsPath)
	if path == "" {
		return nil
	}
	if strings.TrimSpace(line) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 G306 -- path derived from the settings path the application already owns
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), line)
	return err
}

// ReadDiagnosticTail returns the last bytes of the log, or what it can read.
func ReadDiagnosticTail(settingsPath string, maxBytes int) string {
	path := DiagnosticsLogPath(settingsPath)
	if path == "" {
		return ""
	}
	f, err := os.Open(path) // #nosec G304 -- path derived from the settings path the application already owns
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	stat, err := f.Stat()
	if err != nil {
		return ""
	}
	size := stat.Size()
	if size > int64(maxBytes) {
		if _, err := f.Seek(size-int64(maxBytes), io.SeekStart); err != nil {
			return ""
		}
	}
	body, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return string(body)
}

// Diagnostics builds the text a person hands over when something is wrong.
// Every line names where its value came from, because "mpv: missing" is the
// answer a support reader needs, and "mpv: " is not.
func Diagnostics(client *api.Client, settingsPath string, settings Settings) string {
	var b strings.Builder
	b.WriteString("TorrSyncPlayer diagnostics\n")

	// The backend answers /api/v1/version with what it was stamped with at
	// build time; the frontend carries nothing of its own.
	version := "unreachable"
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		var out struct {
			Version   string `json:"version"`
			Commit    string `json:"commit"`
			BuildTime string `json:"buildTime"`
		}
		if err := client.Get(ctx, "/api/v1/version", &out); err == nil {
			version = fmt.Sprintf("%s (commit %s, built %s)", out.Version, out.Commit, out.BuildTime)
		} else {
			version = "unreachable: " + err.Error()
		}
		cancel()
	}
	b.WriteString("backend version: " + version + "\n")
	b.WriteString("os/arch: " + runtime.GOOS + "/" + runtime.GOARCH + "\n")

	mpv, found := mpvLookup(settings.PlayerBinary)
	if found {
		b.WriteString("mpv: " + mpv + "\n")
	} else {
		b.WriteString("mpv: NOT FOUND (playback needs it)\n")
	}

	logPath := DiagnosticsLogPath(settingsPath)
	if logPath == "" {
		b.WriteString("log file: none (no settings path)\n")
	} else {
		b.WriteString("log file: " + logPath + "\n")
		if tail := ReadDiagnosticTail(settingsPath, diagnosticsTailBytes); tail != "" {
			b.WriteString("\nlast log lines:\n")
			b.WriteString(tail)
			if !strings.HasSuffix(tail, "\n") {
				b.WriteString("\n")
			}
		} else {
			b.WriteString("\nlog file is empty or unreadable\n")
		}
	}
	return b.String()
}
