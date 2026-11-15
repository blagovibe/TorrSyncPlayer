// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package cicontract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWorkflowPathFiltersMatchTheTree pins the same class of rot that made
// the security scan never start: a `paths:` filter in a workflow names a
// directory or an extension that the repository no longer carries, and every
// job behind it is dead while looking alive.
//
// The scan reads the workflows and the tree, and fails on any pattern that
// matches nothing in it.
func TestWorkflowPathFiltersMatchTheTree(t *testing.T) {
	files := collectFiles(t, "../../..")

	matches := 0
	for _, wf := range workflowFiles(t) {
		body, err := os.ReadFile(filepath.Clean(wf))
		require.NoError(t, err)
		for _, pattern := range pathFilters(string(body)) {
			regex := pathFilterToRegex(t, pattern)
			matched := false
			for _, file := range files {
				if regex.MatchString(file) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("%s filters on %q, which matches no file — the job behind it never runs", filepath.Base(wf), pattern)
			} else {
				matches++
			}
		}
	}
	// A filter that matches nothing fails above; this guard fails when the
	// extraction itself went blind and saw no filters at all (e.g. the
	// workflow format changed under it).
	require.GreaterOrEqual(t, matches, 1,
		"no path filters were found in any workflow — the check is blind, not the tree clean")
}

// collectFiles lists every file in the repository, relative to the root.
func collectFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	require.NoError(t, err)
	return out
}

// workflowFiles lists the repository's workflows.
func workflowFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(workflows)
	require.NoError(t, err)
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yml") {
			out = append(out, filepath.Join(workflows, entry.Name()))
		}
	}
	require.NotEmpty(t, out, "no workflows found — this package cannot watch anything")
	return out
}

// pathFilters extracts the entries of every `paths:` block in a workflow
// file. It is deliberately simple: the project's filters are one level of
// list items under a key, and a parser that tried to understand YAML would
// be a bigger project than the check it serves.
func pathFilters(body string) []string {
	var patterns []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- '") && strings.HasSuffix(trimmed, "'") && len(trimmed) > 4 {
			pattern := strings.TrimPrefix(trimmed, "- '")
			pattern = strings.TrimSuffix(pattern, "'")
			// The trigger lists the paths that start the workflow; a
			// bare `- foo` without quotes is also accepted.
			if strings.Contains(pattern, "/") {
				patterns = append(patterns, pattern)
			}
		}
		if strings.HasPrefix(trimmed, "- \"") && strings.HasSuffix(trimmed, "\"") && len(trimmed) > 4 {
			pattern := strings.TrimPrefix(trimmed, "- \"")
			pattern = strings.TrimSuffix(pattern, "\"")
			if strings.Contains(pattern, "/") {
				patterns = append(patterns, pattern)
			}
		}
	}
	return patterns
}

// pathFilterToRegex compiles one filter. Supported: literals, `*`,
// `**` and `?`. That is everything the workflows use, and a filter the
// compiler rejects fails loudly rather than going dead quietly.
func pathFilterToRegex(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	var src strings.Builder
	src.WriteString("^")
	for i := 0; i < len(pattern); {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			src.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(pattern[i:], "/**"):
			src.WriteString("(?:/.*)?")
			i += 3
		case strings.HasPrefix(pattern[i:], "**"):
			src.WriteString(".*")
			i += 2
		case pattern[i] == '*':
			src.WriteString("[^/]*")
			i++
		case pattern[i] == '?':
			src.WriteString("[^/]")
			i++
		default:
			src.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}
	src.WriteString("$")
	regex, err := regexp.Compile(src.String())
	require.NoError(t, err, "filter %q compiled to an illegal expression", pattern)
	return regex
}
