// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package cicontract pins the CI workflow against the code it is supposed to
// be checking. It holds no production code on purpose: what it guards is the
// agreement between the pipeline and the repository, and that agreement has
// rotted three times — a mutation scope naming packages deleted with the room
// broker, a coverage filter excluding a package the backend never had, and a
// C++ mutation step that echoed «placeholder» and went green. None of them
// failed a build, because a green step and a working step look identical from
// the outside.
//
// DEV.md, «Проверки должны уметь краснеть», is the rule these tests exist for:
// every check here fails on the state it was written against.
package cicontract

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	workflow   = "../../../.github/workflows/ci.yml"
	workflows  = "../../../.github/workflows"
	moduleRoot = "../.."
)

// packageExists resolves a package pattern against the backend module, from
// the module root: a pattern like `./internal/validation` means nothing from
// the directory the test happens to run in.
func packageExists(t *testing.T, pattern string) bool {
	t.Helper()
	// #nosec G204 -- the argument is a package pattern read out of the
	// project's own workflow file. Nothing a person can reach gets here: the
	// test fails if the file names a package that is not there, which is the
	// whole point.
	cmd := exec.Command("go", "list", pattern) //nolint:gosec // G204: see above
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("go list %s: %v (%s)", pattern, err, strings.TrimSpace(string(out)))
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func repoFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err, "read %s", path)
	return string(body)
}

// packageArgRe finds `./internal/...`-style package patterns where a shell
// command names them, which is where a stale package path hides.
var packageArgRe = regexp.MustCompile(`\./[A-Za-z0-9_./-]*\.\.\.`)

// packageOf turns a `./internal/validation/...` pattern into the `./…` path
// `go list` needs. The leading dot is not decoration: without it `go list`
// reads the name as a standard-library package and fails on it.
func packageOf(pattern string) string {
	return strings.TrimSuffix(pattern, "/...")
}

// TestTheMutationScopeNamesOnlyPackagesThatExist pins the scope of the
// mutation gate. It ran against ./internal/sync and ./internal/p2p for days
// after both were deleted with the room broker: the gate stayed green, the
// score it printed was not the score its own comment claimed, and nothing in
// the repository could tell. A package pattern that names nothing is the
// quietest way to shrink a gate.
func TestTheMutationScopeNamesOnlyPackagesThatExist(t *testing.T) {
	body := repoFile(t, workflow)

	scope := mutationScope(t, body)
	require.NotEmpty(t, scope, "the workflow must still run mutation testing")

	for _, pattern := range packageArgRe.FindAllString(scope, -1) {
		pkg := packageOf(pattern)
		assert.True(t, packageExists(t, pkg),
			"%s is in the mutation scope of ci.yml but the module has no such package — "+
				"the gate is measuring something other than what it claims", pkg)
	}
}

// TestTheCoverageStepMeasuresTheWholeModule pins the second rot: the backend
// coverage step filtered out a package named `integration`, which the backend
// has never had since the room moved to the frontend. The filter looked like
// it was excluding something and excluded nothing.
func TestTheCoverageStepMeasuresTheWholeModule(t *testing.T) {
	body := repoFile(t, workflow)

	require.Contains(t, body, "coverprofile",
		"the backend must still produce a coverage profile")
	assert.NotContains(t, body, "grep -v 'integration'",
		"the backend has no `integration` package; the filter excluded nothing and hid that")

	// Whatever package patterns the test command names must exist, too.
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "coverprofile") || !strings.Contains(line, "go test") {
			continue
		}
		for _, pattern := range packageArgRe.FindAllString(line, -1) {
			// `./...` is the whole module — the point of the step, not a
			// package that could have gone missing.
			if pattern == "./..." {
				continue
			}
			pkg := packageOf(pattern)
			assert.True(t, packageExists(t, pkg),
				"%s is in the coverage command of ci.yml but the module has no such package", pkg)
		}
	}
}

// TestNoCIJobIsAPlaceholder pins the third rot, and the one that survives the
// longest: a step that echoes «placeholder for future implementation» and
// reports success. A step that checks nothing is worse than a missing step,
// because it is counted as coverage of whatever it was named after.
func TestNoCIJobIsAPlaceholder(t *testing.T) {
	body := repoFile(t, workflow)
	assert.NotContains(t, strings.ToLower(body), "placeholder",
		"ci.yml still carries a step that says it is a placeholder; a green step must check something")
}

// mutationScope returns the shell block of the step that runs go-mutesting.
func mutationScope(t *testing.T, body string) string {
	t.Helper()
	for _, step := range strings.Split(body, "      - name: ") {
		if !strings.Contains(step, "go-mutesting \\") {
			continue
		}
		return step
	}
	return ""
}

// goVersionPatterns read the pinned Go version out of a workflow, in either of
// the two forms the repository uses: a GO_VERSION the workflow declares for
// itself, or a version written straight into setup-go. Both are fine; what is
// not fine is a workflow with neither, or two workflows that disagree — one is
// a job on a compiler nobody chose, the other is two green jobs about different
// compilers.
var (
	goEnvVersionPattern   = regexp.MustCompile(`GO_VERSION:\s*['"]([0-9][^'"]*)['"]`)
	goSetupVersionPattern = regexp.MustCompile(`go-version:\s*['"]([0-9][^'"]*)['"]`)
)

func TestEveryWorkflowPinsTheSameGoVersion(t *testing.T) {
	entries, err := os.ReadDir(workflows)
	require.NoError(t, err, "read the workflows directory")

	pinned := map[string][]string{}
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yml") &&
			!strings.HasSuffix(entry.Name(), ".yaml")) {
			continue
		}
		body := repoFile(t, filepath.Join(workflows, entry.Name()))
		if !strings.Contains(body, "actions/setup-go") {
			continue
		}

		versions := map[string]struct{}{}
		for _, pattern := range []*regexp.Regexp{goEnvVersionPattern, goSetupVersionPattern} {
			for _, m := range pattern.FindAllStringSubmatch(body, -1) {
				versions[m[1]] = struct{}{}
			}
		}
		require.NotEmpty(t, versions,
			"%s sets Go up without pinning a version: setup-go would take its own default, "+
				"and the job would build with a compiler nobody chose", entry.Name())
		for v := range versions {
			pinned[v] = append(pinned[v], entry.Name())
		}
	}

	require.NotEmpty(t, pinned, "no workflow sets Go up, which cannot be right")
	for version, names := range pinned {
		sort.Strings(names)
		t.Logf("Go %s: %s", version, strings.Join(names, ", "))
	}
	assert.Len(t, pinned, 1,
		"workflows pin more than one Go version: a green job about a different compiler is still a lie")
}

// TestEveryModuleIsCheckedForTidiness pins that each Go module's go.mod and
// go.sum are asked whether they are tidy.
//
// The backend had such a check and the frontend did not, so the frontend's
// sums drifted and nobody saw it: the bot that publishes the measurement's
// row committed the drift, because `go mod download all` writes the sums of
// modules the build does not need, and only `go mod tidy` takes them away.
//
// A module nobody checks is a module nobody looks at.
func TestEveryModuleIsCheckedForTidiness(t *testing.T) {
	body := repoFile(t, workflow)

	// The steps of the workflow, so a step can be looked at as a whole.
	steps := strings.Split(body, "\n      - name: ")
	for _, module := range []string{"backend", "frontend"} {
		checked := false
		for _, step := range steps {
			if !strings.Contains(step, "go mod tidy") {
				continue
			}
			if !strings.Contains(step, "git diff --exit-code go.mod go.sum") {
				continue
			}
			if strings.Contains(step, "working-directory: "+module) {
				checked = true
				break
			}
		}
		assert.True(t, checked,
			"the %s module must have a step that runs go mod tidy and fails when go.mod or go.sum changed", module)
	}
}
