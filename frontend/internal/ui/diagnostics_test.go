// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
)

// The log lives beside the settings because that is the one directory the
// application already owns and the person already has a way to open.
func TestDiagnosticsLogPath(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "TorrSyncPlayer", "settings.json")
	got := DiagnosticsLogPath(settings)
	assert.Equal(t, filepath.Join(filepath.Dir(settings), "torrsyncplayer.log"), got)

	assert.Equal(t, "", DiagnosticsLogPath(""), "a store with no file keeps no log either")
}

func TestWriteDiagnosticAppendsAndReadsBack(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "TorrSyncPlayer", "settings.json")

	require.NoError(t, WriteDiagnostic(settings, "first line"))
	require.NoError(t, WriteDiagnostic(settings, "second line"))

	tail := ReadDiagnosticTail(settings, 4096)
	assert.Contains(t, tail, "first line")
	assert.Contains(t, tail, "second line")

	// A blank line is not something worth a timestamp.
	assert.NoError(t, WriteDiagnostic(settings, "   "))
	assert.NotContains(t, ReadDiagnosticTail(settings, 4096), "   \n")
}

func TestReadDiagnosticTailCaps(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "TorrSyncPlayer", "settings.json")
	big := strings.Repeat("x", 4000)
	require.NoError(t, WriteDiagnostic(settings, big))
	require.NoError(t, WriteDiagnostic(settings, "the last line"))

	tail := ReadDiagnosticTail(settings, 64)
	assert.LessOrEqual(t, len(tail), 128, "the tail is capped, not a reprint")
	assert.True(t, strings.HasSuffix(strings.TrimRight(tail, "\n"), "the last line"))
}

// The diagnostics text answers the three questions a support reader asks:
// what build, what machine, and why no playback — plus the log itself.
func TestDiagnosticsMentionsWhatMatters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":"0.4.2","commit":"abc123","buildTime":"2026-11-14"}`)
	}))
	defer srv.Close()

	settingsPath := filepath.Join(t.TempDir(), "TorrSyncPlayer", "settings.json")
	require.NoError(t, WriteDiagnostic(settingsPath, "mpv start failed"))

	client := api.New(srv.URL, "token")
	text := Diagnostics(client, settingsPath, Settings{PlayerBinary: "/definitely/not-mpv-here"})

	assert.Contains(t, text, "0.4.2")
	assert.Contains(t, text, "abc123")
	assert.Contains(t, text, "os/arch: "+runtime.GOOS+"/"+runtime.GOARCH)
	assert.Contains(t, text, "mpv: NOT FOUND")
	assert.Contains(t, text, "mpv start failed")
}

func TestDiagnosticsWhenBackendIsUnreachable(t *testing.T) {
	client := api.New("http://127.0.0.1:1", "token")
	text := Diagnostics(client, "", Settings{})

	assert.Contains(t, text, "unreachable")
	assert.Contains(t, text, "log file: none (no settings path)")
}
