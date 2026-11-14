// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package cicontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rendezvous series is the mirror of the divergence series: a shell
// script, a table, and a job that fills it in. It was the one that went
// quietly wrong — dividing the number of good rows in the last
// THRESHOLD_RUNS by the size of the whole file, so the check could never
// say yes again after row twenty, and it opened one false card per run
// (bug-0157).
//
// These tests drive the script the way the job drives it.
const (
	rendezvousScript   = "../../../.github/scripts/rendezvous-series.sh"
	rendezvousSeries   = "../../../measurements/rendezvous.md"
	rendezvousBoardDir = "../../../.project-board"
)

// metRun is the log of a run in which the two participants found each other.
const metRun = `=== RUN   TestARoomAcrossNAT
    public_dht_test.go:88: outcome: the room met
    public_dht_test.go:89: waited: 2
    public_dht_test.go:90: waitedSeconds: 2
--- PASS: TestARoomAcrossNAT (2.00s)
`

// missedRun is the log of a run in which they did not.
const missedRun = `=== RUN   TestARoomAcrossNAT
    public_dht_test.go:88: outcome: the search did not converge
    public_dht_test.go:89: waited: 300
    public_dht_test.go:90: waitedSeconds: 300
--- FAIL: TestARoomAcrossNAT (301.00s)
`

// runRendezvousScript runs the rendezvous series script the way the job
// runs it.
func runRendezvousScript(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return runScriptWith(t, rendezvousScript, args...)
}

// runScriptWith is the shared runner for the scripts of both series.
func runScriptWith(t *testing.T, script string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{script}, args...)...) //nolint:gosec // G204: the script is the project's own
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestTheRendezvousSeriesRecordsTheMeeting pins the first half, like the
// divergence series: what the run printed is what the row says.
func TestTheRendezvousSeriesRecordsTheMeeting(t *testing.T) {
	series := filepath.Join(t.TempDir(), "rendezvous.md")
	require.NoError(t, os.WriteFile(series, []byte("| When | outcome | seconds | waited |\n"), 0o600))

	out, err := runRendezvousScript(t, "record", writeLog(t, metRun), series)
	require.NoError(t, err, "recording a met run: %s", out)
	assert.Contains(t, out, "met", "the row must say the room met: %s", out)
	assert.Contains(t, out, "2", "the row must carry the seconds: %s", out)

	out, err = runRendezvousScript(t, "record", writeLog(t, missedRun), series)
	require.NoError(t, err, "recording a missed run: %s", out)
	assert.Contains(t, out, "did not meet", "a missed run must be recorded as missed: %s", out)
}

// TestTheRendezvousSeriesWeighsOnlyTheWindow pins the divisor of the rate,
// which is the bug this file exists to catch: fifty-one false cards were
// opened because the script divided the good rows of the last window by
// every row the file had ever recorded.
func TestTheRendezvousSeriesWeighsOnlyTheWindow(t *testing.T) {
	series := filepath.Join(t.TempDir(), "rendezvous.md")
	require.NoError(t, os.WriteFile(series, []byte("| When | outcome | seconds | waited |\n"), 0o600))

	goodRow := "| 2026-11-13T00:00:00Z | met | 2 | 2s |\n"
	var rows strings.Builder
	rows.WriteString("| When | outcome | seconds | waited |\n")
	for i := 0; i < 25; i++ {
		rows.WriteString(goodRow)
	}
	require.NoError(t, os.WriteFile(series, []byte(rows.String()), 0o600))

	out, err := runRendezvousScript(t, "check", series, t.TempDir())
	require.NoError(t, err, "a series of 25 good meetings must pass on its merits: %s", out)
	assert.Contains(t, out, "the last 20 of 25 runs", "the verdict must name the window and the size of the series: %s", out)
}

// TestTheRendezvousSeriesOpensACardWhenTheWindowIsBad pins that the window,
// once the only thing the rate sees, is also the only thing that fails:
// fifteen good meetings followed by five misses must open a card.
func TestTheRendezvousSeriesOpensACardWhenTheWindowIsBad(t *testing.T) {
	series := filepath.Join(t.TempDir(), "rendezvous.md")
	require.NoError(t, os.WriteFile(series, []byte("| When | outcome | seconds | waited |\n"), 0o600))

	log := writeLog(t, metRun)
	badLog := writeLog(t, missedRun)

	for i := 0; i < 20; i++ {
		out, err := runRendezvousScript(t, "record", log, series)
		require.NoError(t, err, "recording a met run: %s", out)
	}
	for i := 0; i < 5; i++ {
		out, err := runRendezvousScript(t, "record", badLog, series)
		require.NoError(t, err, "recording a missed run: %s", out)
	}

	// The card script computes its number from the board and the journal:
	// the board in a temp copy of the repo gets the real journal.
	root := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".project-board"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte(repoFile(t, "../../../CHANGELOG.md")), 0o600))

	out, err := runRendezvousScript(t, "check", series, root)
	require.Error(t, err, "a broken window must not report itself as fine: %s", out)
	assert.Contains(t, out, "the threshold is broken", "the verdict must be said: %s", out)

	cards := readCards(t, filepath.Join(root, ".project-board"))
	require.Len(t, cards, 1, "a broken window must open exactly one card: %v", cards)
}

// TestTheRendezvousSeriesFileHasTheHeaderTheRowsBelongTo pins the table the
// script writes into, mirroring the divergence series' own pin.
func TestTheRendezvousSeriesFileHasTheHeaderTheRowsBelongTo(t *testing.T) {
	body := repoFile(t, rendezvousSeries)

	header := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "| ") {
			header = line
			break
		}
	}
	require.NotEmpty(t, header, "the series must have a table: %s", body)
	assert.Len(t, strings.Split(header, "|"), 6,
		"the header must have the four columns the row has: %s", header)
	assert.True(t, strings.HasSuffix(body, "\n"),
		"the series must end with a newline, or the first row joins the header")
}
