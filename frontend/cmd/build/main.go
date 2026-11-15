// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Command build assembles a runnable TorrSyncPlayer: the client and the
// backend binary beside it.
//
// It exists because the two are not separable at run time. Every participant
// runs its own backend — that is what "each pulls from the swarm themselves"
// means — so the client looks for the backend next to its own executable
// (see internal/backend.FindBackend) and starts it. A build that produces the
// client alone has produced something that cannot start.
//
// It is a program rather than a shell script because the project ships on Linux,
// macOS and Windows, and three scripts that must agree on what to build is one
// more thing to keep in step. Go is already a build dependency on all three.
//
//	go run ./cmd/build              → ./dist
//	go run ./cmd/build -out release → ./release
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	out := flag.String("out", "dist", "directory to put the runnable pair in")
	flag.Parse()

	if err := build(*out, goBuild); err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
}

// builder compiles one binary: in dir, from pkg (nil meaning the package in
// dir), writing it to output, with ldflags when the binary has to know what it
// is.
//
// It is a parameter because the two builds run from different module
// directories, and that is exactly where a relative output path stops meaning
// what it looks like it means. Injecting it keeps that testable without
// compiling anything twice per run.
type builder func(dir, output string, pkg, ldflags []string) error

func build(out string, goBuild builder) error {
	if err := os.MkdirAll(out, 0o750); err != nil {
		return err
	}

	// Every path handed to `go build` has to be absolute. The two builds run
	// from different module directories — "." for the client, "../backend" for
	// the server — and `go build -o dist/x` resolves that against its own
	// module directory, not against the caller's. With a relative `out` that
	// silently put the backend in backend/dist while the client went to
	// frontend/dist, and the client looks for the backend beside itself.
	absOut, err := filepath.Abs(out)
	if err != nil {
		return err
	}

	// The application.
	app := filepath.Join(absOut, appName())
	if err := goBuild(".", app, nil, nil); err != nil {
		return fmt.Errorf("client: %w", err)
	}

	// The backend, under the name the client looks for. On Windows the two
	// differ by more than case — client.exe beside server.exe — which is why
	// this is a decision and not a constant. It is also the binary that answers
	// /api/v1/version, so it is the one that has to be stamped (bug-0151).
	backendPath := filepath.Join(absOut, backendName())
	if err := goBuild("../backend", backendPath, []string{"./cmd/server"}, versionLdflags()); err != nil {
		return fmt.Errorf("backend: %w", err)
	}

	// Printed the way the caller asked for them rather than as absolute paths:
	// what is on disk and what was requested are the same files, and the
	// request is what the caller recognises.
	fmt.Printf("%s\n%s\n", filepath.Join(out, appName()), filepath.Join(out, backendName()))
	return nil
}

func goBuild(dir, output string, pkg, ldflags []string) error {
	// #nosec G204 -- argv is built here from an output path this tool owns and
	// package paths fixed in the source; it shell-executes nothing.
	cmd := exec.Command("go", buildArgs(output, pkg, ldflags)...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

// buildArgs is the argument list, kept apart so the ldflags a release depends on
// can be asserted without compiling anything (bug-0151).
func buildArgs(output string, pkg, ldflags []string) []string {
	args := []string{"build"}
	if len(ldflags) > 0 {
		args = append(args, "-ldflags", strings.Join(ldflags, " "))
	}
	args = append(args, "-o", output)
	return append(args, pkg...)
}

// versionPackage is where the backend keeps the variables a build stamps.
const versionPackage = "github.com/blagovibe/TorrSyncPlayer/backend/internal/version"

// versionLdflags stamps the backend with what it is.
//
// The release artifacts reported version=dev because this script passed no
// ldflags at all, and the v1.1.x release line had already paid for that lesson
// (bug-0151): a build nobody can identify turns every report into "some build
// from somewhere". The version comes from VERSION, which the release workflow
// sets from the tag; the commit comes from COMMIT. A hands-built binary keeps
// saying dev, so it cannot be mistaken for a release.
func versionLdflags() []string {
	version := strings.TrimPrefix(strings.TrimSpace(os.Getenv("VERSION")), "v")
	if version == "" {
		version = "dev"
	}
	commit := strings.TrimSpace(os.Getenv("COMMIT"))
	if commit == "" {
		commit = "unknown"
	}
	built := time.Now().UTC().Format(time.RFC3339)
	return []string{
		"-X", versionPackage + ".Version=" + version,
		"-X", versionPackage + ".Commit=" + commit,
		"-X", versionPackage + ".BuildTime=" + built,
	}
}

// appName is the client executable: TorrSyncPlayer.exe, TorrSyncPlayer.
func appName() string {
	if runtime.GOOS == "windows" {
		return "TorrSyncPlayer.exe"
	}
	return "TorrSyncPlayer"
}

// backendName is the backend executable.
//
// The lower case on purpose. internal/backend.FindBackend looks for both
// spellings, but the server binary is named in lower case everywhere else in
// the project — backend/build/torrsyncplayer, the CI artifact — and a
// differently-cased pair on a case-insensitive filesystem is a trap.
func backendName() string {
	if runtime.GOOS == "windows" {
		return "torrsyncplayer.exe"
	}
	return "torrsyncplayer"
}
