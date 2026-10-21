// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package cicontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The series of the room's divergence is a shell script, a table in the
// repository, and a job that fills the table in. None of that is code the
// compiler sees, and a script that quietly does the wrong thing is green: it
// writes a row, nobody reads the row, and the promise it was written to keep
// goes unchecked for a year.
//
// These tests drive the script the way the job drives it — a log in, a row
// out — and they check the two things that can go wrong quietly: that a run
// which broke the promise is recorded as a broken run, and that a broken
// series opens a card numbered after the cards that already exist.
const (
	syncScript   = "../../../.github/scripts/sync-series.sh"
	syncSeries   = "../../../measurements/sync.md"
	syncSource   = "../../../frontend/internal/ui/sync_measurement_test.go"
	syncBoardDir = "../../../.project-board"
)

// goodRun is the log of a run that kept the room within the promise.
const goodRun = `=== RUN   TestTheRoomKeepsEveryoneInTheSameSecond
    sync_measurement_test.go:176: steadyDivergence: 0.20
    sync_measurement_test.go:177: landingSeconds: 0.60
    sync_measurement_test.go:179: samples: 40
    sync_measurement_test.go:180: outcome: within the promise
--- PASS: TestTheRoomKeepsEveryoneInTheSameSecond (90.54s)
`

// brokenRun is the log of a run that did not.
const brokenRun = `=== RUN   TestTheRoomKeepsEveryoneInTheSameSecond
    sync_measurement_test.go:176: steadyDivergence: 1.40
    sync_measurement_test.go:177: landingSeconds: 0.60
    sync_measurement_test.go:180: outcome: outside the promise
--- FAIL: TestTheRoomKeepsEveryoneInTheSameSecond (90.54s)
`

// runScript runs the series script the way the job runs it.
func runScript(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{syncScript}, args...)...) //nolint:gosec // G204: the script is the project's own
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestTheSeriesRecordsTheRunItSaw pins the first half: what the measurement
// printed is what the row says. A run that kept the promise is recorded as
// such, and a run that broke it is recorded as broken — because a series that
// records everything as fine is a series that says nothing.
func TestTheSeriesRecordsTheRunItSaw(t *testing.T) {
	series := filepath.Join(t.TempDir(), "sync.md")
	require.NoError(t, os.WriteFile(series, []byte("| Когда | Расхождение (с) | Перемотка встала за (с) | Итог |\n"), 0o600))

	out, err := runScript(t, "record", writeLog(t, goodRun), series, syncSource)
	require.NoError(t, err, "recording a good run: %s", out)
	assert.Contains(t, out, "within the promise", "the row must say what the run was: %s", out)
	assert.Contains(t, out, "0.20", "the row must carry the number: %s", out)

	out, err = runScript(t, "record", writeLog(t, brokenRun), series, syncSource)
	require.NoError(t, err, "recording a broken run: %s", out)
	assert.Contains(t, out, "outside the promise", "a broken run must be recorded as broken: %s", out)

	rows := readRows(t, series)
	require.Len(t, rows, 2, "one row per run: %v", rows)
}

// TestTheSeriesOpensACardWhenThePromiseIsBroken pins the second half: a
// series of broken runs opens a card on the board, and the card's number is
// the next free one.
//
// The number is the part that breaks silently. Bash reads a zero-padded
// number as octal, so 0146 became 102 and the card was numbered from a number
// nobody had used — the board would have carried two cards with one id.
func TestTheSeriesOpensACardWhenThePromiseIsBroken(t *testing.T) {
	dir := t.TempDir()
	series := filepath.Join(dir, "sync.md")
	require.NoError(t, os.WriteFile(series, []byte("| Когда | Расхождение (с) | Перемотка встала за (с) | Итог |\n"), 0o600))
	log := writeLog(t, brokenRun)

	for i := 0; i < 20; i++ {
		out, err := runScript(t, "record", log, series, syncSource)
		require.NoError(t, err, "recording a broken run: %s", out)
	}

	// The board is a directory of cards, and the journal knows the numbers
	// that were used before: the next number is one more than the highest of
	// them. The script works on a copy of the repository, so the copy gets
	// the real journal.
	root := filepath.Join(dir, "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".project-board"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte(repoFile(t, "../../../CHANGELOG.md")), 0o600))

	out, err := runScript(t, "check", series, root, syncSource)
	require.Error(t, err, "a broken series must not report itself as fine: %s", out)
	assert.Contains(t, out, "the threshold is broken", "the verdict must be said: %s", out)

	cards := readCards(t, filepath.Join(root, ".project-board"))
	require.Len(t, cards, 1, "a broken series must open exactly one card: %v", cards)

	// The number must be the next free one, and it must not be the octal
	// reading of it.
	number := cardNumber(t, cards[0])
	assert.Equal(t, nextFreeNumber(t), number,
		"the card must be numbered after the cards that already exist: %s", cards[0])
}

// TestTheSeriesReadsThePromiseFromTheMeasurement pins where the number comes
// from: the measurement, which is where the goal's number is written down. A
// script that carried its own copy would answer a question nobody asked.
func TestTheSeriesReadsThePromiseFromTheMeasurement(t *testing.T) {
	source := repoFile(t, syncSource)
	assert.Contains(t, source, "const steadyDivergenceLimit = 1.0",
		"the promise's number must be readable in one line: the series script greps it")

	out, err := runScript(t, "record", writeLog(t, goodRun), filepath.Join(t.TempDir(), "sync.md"), syncSource)
	require.NoError(t, err, "the script must find the number: %s", out)
	assert.Contains(t, out, "within the promise", "0.20 s is within the promise of one second: %s", out)
}

// TestTheSeriesFileHasTheHeaderTheRowsBelongTo pins the table the script
// writes into. A row appended to a file whose header says something else is a
// row nobody can read, and a file that does not end with a newline swallows
// the first row into the header.
func TestTheSeriesFileHasTheHeaderTheRowsBelongTo(t *testing.T) {
	body := repoFile(t, syncSeries)

	// The table's header is the first line that starts a row: the file opens
	// with a title and an explanation, and the table comes after them.
	header := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "| ") {
			header = line
			break
		}
	}
	require.NotEmpty(t, header, "the series must have a table: %s", body)
	assert.Contains(t, header, "Расхождение", "the header must name what the column is: %s", header)
	assert.Len(t, strings.Split(header, "|"), 6,
		"the header must have the four columns the row has: %s", header)
	assert.True(t, strings.HasSuffix(body, "\n"),
		"the series must end with a newline, or the first row joins the header")
}

// writeLog puts a run's output into a file and returns its path.
func writeLog(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "run.log")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// readRows returns the rows of a series file, without its header.
func readRows(t *testing.T, path string) []string {
	t.Helper()
	body := repoFile(t, path)
	var rows []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "| 20") {
			rows = append(rows, line)
		}
	}
	return rows
}

// readCards returns the files of a board directory.
func readCards(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var cards []string
	for _, entry := range entries {
		if !entry.IsDir() {
			cards = append(cards, filepath.Join(dir, entry.Name()))
		}
	}
	return cards
}

// cardNumber reads the number out of a card's name.
func cardNumber(t *testing.T, path string) string {
	t.Helper()
	name := filepath.Base(path)
	parts := strings.SplitN(name, "-", 3)
	require.Len(t, parts, 3, "a card is named type-number-slug: %s", name)
	return parts[1]
}

// nextFreeNumber is the number the board and the journal say is next: one
// more than the highest they use.
func nextFreeNumber(t *testing.T) string {
	t.Helper()
	highest := 0
	for _, entry := range readCards(t, syncBoardDir) {
		parts := strings.SplitN(filepath.Base(entry), "-", 3)
		require.Len(t, parts, 3, "a card is named type-number-slug: %s", entry)
		if n := atoiOrZero(parts[1]); n > highest {
			highest = n
		}
	}
	used := regexp.MustCompile(`\b(?:idea|bug|deviation)-([0-9]{4})\b`).FindAllStringSubmatch(
		repoFile(t, "../../../CHANGELOG.md"), -1)
	for _, match := range used {
		if n := atoiOrZero(match[1]); n > highest {
			highest = n
		}
	}
	return pad4(highest + 1)
}

// atoiOrZero reads a number, and treats anything else as zero.
func atoiOrZero(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// pad4 renders a number the way a card's name does.
func pad4(n int) string {
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}
