// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
)

// The catalogue is maintained by hand, and these two checks are what keeps it
// honest about the window — the package where every string a person reads is
// put on a widget.
//
// The catalogue's own tests check the languages against each other: a key that
// exists is translated, and no translation is orphaned. Neither of those can
// see the two ways the window evades the catalogue:
//
//   - a status line written as a plain string literal, which renders in the
//     source language no matter what the person chose;
//   - a key that is asked for but never defined, which renders as an empty
//     label — the worst of the two, because nothing is shown at all.
//
// Both were real: the whole playback-error path bypassed the catalogue, and
// the track choosers asked for keys that did not exist.

// TestNoWindowTextBypassesTheCatalog fails on any string literal with a word
// in it passed to setStatus or setLabel outside a tr/trf call.
//
// The literal is checked as text from the source, like the catalogue's own
// tests check text: a check by value would only see the strings that are
// already translated, and the point is to catch the ones that are not.
// Punctuation-only separators (": ", " — ") pass on purpose: a colon is a
// colon in every language, and demanding a catalogue entry for it would teach
// people to write around the check.
func TestNoWindowTextBypassesTheCatalog(t *testing.T) {
	for _, src := range windowSourceFiles(t) {
		ast.Inspect(src.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch calledMethod(call) {
			case "setStatus", "setLabel":
			default:
				return true
			}

			for _, arg := range call.Args {
				for _, literal := range wordLiteralsOutsideCatalog(arg) {
					position := src.fset.Position(literal.Pos())
					text, _ := strconv.Unquote(literal.Value)
					t.Errorf("%s: %q reaches the screen without the catalogue — a person reading another language sees this in the source one",
						position, text)
				}
			}
			return true
		})
	}
}

// TestEveryCatalogKeyTheWindowAsksForExists fails on a key passed to tr or trf
// that the source catalogue does not define.
//
// The other direction of the same discipline: bypassing the catalogue shows
// the wrong language, and asking for a key nobody defined shows nothing —
// an empty label or an empty status line, which looks like the feature is
// missing rather than untranslated.
func TestEveryCatalogKeyTheWindowAsksForExists(t *testing.T) {
	known := make(map[string]bool, len(catalog.Keys()))
	for _, key := range catalog.Keys() {
		known[key] = true
	}

	for _, src := range windowSourceFiles(t) {
		ast.Inspect(src.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := calledMethod(call)
			if (name != "tr" && name != "trf") || len(call.Args) == 0 {
				return true
			}

			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			key, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			if !known[key] {
				t.Errorf("%s: the catalogue has no %q — this text would reach the screen as an empty label",
					src.fset.Position(literal.Pos()), key)
			}
			return true
		})
	}
}

// windowSourceFiles parses the package's own sources, tests excluded: the
// checks are about what ships. The tray lives in this package too, so
// internal/ui is the whole surface a person can read text from.
func windowSourceFiles(t *testing.T) []parsedSource {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}

	var sources []parsedSource
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		sources = append(sources, parsedSource{fset: fset, file: file})
	}

	if len(sources) == 0 {
		t.Fatal("no sources were found — the walk is broken, not the code")
	}
	return sources
}

// parsedSource is one file together with the position table that names its
// lines, so a failure points at the line rather than at a node.
type parsedSource struct {
	fset *token.FileSet
	file *ast.File
}

// calledMethod is the method name of a call like w.tr(...), or "" when the
// call is not a method call at all.
func calledMethod(call *ast.CallExpr) string {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return selector.Sel.Name
}

// wordLiteralsOutsideCatalog collects string literals inside an expression
// that contain letters and do not sit inside a tr/trf call.
//
// Letters, not length: a separator or a format skeleton is not a sentence in
// any language, and requiring a catalogue entry for one would make the check
// something to route around.
func wordLiteralsOutsideCatalog(expr ast.Expr) []*ast.BasicLit {
	var found []*ast.BasicLit

	ast.Inspect(expr, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if name := calledMethod(call); name == "tr" || name == "trf" {
				return false
			}
		}

		literal, ok := n.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		if strings.ContainsFunc(text, unicode.IsLetter) {
			found = append(found, literal)
		}
		return true
	})

	return found
}
