// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package cicontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The vulnerability scan fetches its database over the network, and a broken
// connection to it is the weather, not a finding. The scan tells the two
// apart by its exit code — 3 for a finding, everything else for a scan that
// did not finish — and the script retries only the second.
//
// A job that goes red for someone else's reasons teaches everyone to ignore
// red, and this repository has already paid for that lesson twice: the
// modules and the tool installs both got the same treatment. The scan was
// the third, and it went red on a run where the code was fine.
const govulncheckScript = "../../../.github/scripts/govulncheck.sh"

// TestTheScanTellsAFindingFromAFailureToLook drives the script with a
// stand-in scan that exits the way the real one does.
func TestTheScanTellsAFindingFromAFailureToLook(t *testing.T) {
	t.Run("a clean scan passes", func(t *testing.T) {
		out, code := runFakeScan(t, 0)
		assert.Equal(t, 0, code, "a clean scan must pass: %s", out)
	})

	t.Run("a finding fails at once", func(t *testing.T) {
		out, code := runFakeScan(t, 3)
		assert.Equal(t, 3, code, "a finding must fail the job: %s", out)
		assert.Contains(t, out, "found vulnerabilities", "the reason must be on the screen: %s", out)
		assert.NotContains(t, out, "trying again",
			"a finding is not the weather and must not be retried: %s", out)
	})

	t.Run("a failure to look is retried", func(t *testing.T) {
		// The stand-in fails twice and then succeeds, which is what a
		// database that was briefly unreachable looks like.
		out, code := runFakeScan(t, 1, 1, 0)
		assert.Equal(t, 0, code, "a scan that finally finished must pass: %s", out)
		assert.Contains(t, out, "trying again", "the retry must be said: %s", out)
	})

	t.Run("a scan that never finishes fails", func(t *testing.T) {
		out, code := runFakeScan(t, 1, 1, 1)
		assert.NotEqual(t, 0, code, "a scan that never finished must not pass: %s", out)
		assert.Contains(t, out, "did not finish", "the reason must be on the screen: %s", out)
	})
}

// runFakeScan runs the script against a stand-in scan that exits with the
// given codes, one per attempt, and returns the output and the exit code.
func runFakeScan(t *testing.T, codes ...int) (string, int) {
	t.Helper()

	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(bin, 0o750))

	// The stand-in counts its own runs, so each attempt gets the next code.
	counter := filepath.Join(dir, "attempts")
	stand := "#!/usr/bin/env bash\n" +
		"n=0\n" +
		"if [ -f " + counter + " ]; then n=$(cat " + counter + "); fi\n" +
		"n=$((n + 1))\n" +
		"echo \"$n\" > " + counter + "\n" +
		"echo \"the stand-in scan, attempt $n\"\n" +
		"codes=("
	for i, c := range codes {
		if i > 0 {
			stand += " "
		}
		stand += itoa(c)
	}
	stand += ")\n" +
		"exit \"${codes[$((n - 1))]}\"\n"

	path := filepath.Join(bin, "govulncheck")
	// The stand-in has to be executable — it stands in for a program the
	// script runs — and gosec wants files at 0600, which would not run. It
	// is a throwaway in a temporary directory.
	//nolint:gosec // G306: an executable stand-in, in a temporary directory
	require.NoError(t, os.WriteFile(path, []byte(stand), 0o700))

	cmd := exec.Command("bash", govulncheckScript, "./...") //nolint:gosec // G204: the script is the project's own
	cmd.Env = append(os.Environ(),
		"PATH="+bin+":"+os.Getenv("PATH"),
		// The wait between attempts is a variable, so the test does not
		// spend half a minute sleeping.
		"GOVULNCHECK_RETRY_WAIT=0",
	)
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit, "the script must exit non-zero: %s", out)
		code = exit.ExitCode()
	}
	return string(out), code
}

// itoa renders a number for the stand-in script.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
