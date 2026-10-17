// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package backend

import (
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestArgsCarryNoFlagTheBackendWouldRefuse(t *testing.T) {
	// An unknown flag makes the backend print "flag provided but not defined"
	// and exit 2. The application then does not start, and nothing in CI sees
	// it: this is the class of defect that shipped --jwt-secret once.
	if arg, ok := UnknownFlag(Args(Launch{Port: 8889, Token: "abc"})); !ok {
		t.Fatalf("the backend would refuse %q", arg)
	}
}

func TestArgsPassThePortAndTheToken(t *testing.T) {
	// The secret used to travel in here as `--token <secret>` — in the process
	// list, readable by anyone who can run ps on this machine
	// (deviation-0061). The flag now only says that the token is coming; the
	// secret itself goes to the child's standard input, and it must not appear
	// among the arguments at all.
	args := Args(Launch{Port: 1234, Token: "secret"})

	// The address is loopback, not a bare port: a bare port made the backend
	// bind every interface, which put a torrent client on the local network
	// on every launch (bug-0114).
	want := []string{"--addr", "127.0.0.1:1234", "--token-stdin", "--data-dir", ""}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v, want %v", args, want)
	}
	for _, arg := range args {
		if strings.Contains(arg, "secret") {
			t.Fatalf("the secret appears on the command line as %q", arg)
		}
	}
}

func TestArgsOmitAnEmptyToken(t *testing.T) {
	// An empty token pins the backend to an empty secret rather than letting
	// it generate one, which is exactly the failure bug-0060 was about. With
	// no token there is nothing to write to standard input, so the flag that
	// asks for one is left out too.
	for _, arg := range Args(Launch{Port: 8889}) {
		if strings.Contains(arg, "token") {
			t.Fatalf("an empty token was passed as %q", arg)
		}
	}
}

func TestUnknownFlagCatchesARemovedFlag(t *testing.T) {
	if _, ok := UnknownFlag([]string{"--port", "8889", "--jwt-secret", "x"}); ok {
		t.Fatal("a flag the backend removed was accepted")
	}
}

func TestGeneratedTokenIsUsableAndFixedLength(t *testing.T) {
	first, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}

	hexOnly := regexp.MustCompile(`^[0-9a-f]{32}$`)
	if !hexOnly.MatchString(first) {
		t.Fatalf("token %q is not 32 hex characters", first)
	}

	second, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two generated tokens are equal — the token is not random")
	}
}

func TestPickedPortIsFree(t *testing.T) {
	port, err := PickFreePort()
	if err != nil {
		t.Fatal(err)
	}
	if port == 0 {
		t.Fatal("no free port was picked")
	}

	// Free means bindable right now: the same check the picker makes.
	listener, err := listenAny(port)
	if err != nil {
		t.Fatalf("picked port %d is not bindable: %v", port, err)
	}
	_ = listener.Close()
}

func TestPickedPortDiffersFromAnOccupiedOne(t *testing.T) {
	// Every instance starts its own backend. With a fixed port the second
	// instance fails to bind and the application comes up talking to somebody
	// else's server.
	occupied, err := listenAny(0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = occupied.Close() }()

	busy := occupied.Addr().(*net.TCPAddr).Port

	picked, err := PickFreePort()
	if err != nil {
		t.Fatal(err)
	}
	if picked == busy {
		t.Fatalf("picked the occupied port %d", picked)
	}
}

func TestCandidatePathsCoverTheNonWindowsBuild(t *testing.T) {
	paths := CandidatePaths("/opt/app")

	found := false
	for _, path := range paths {
		if strings.HasSuffix(path, string(filepath.Separator)+"torrsyncplayer") {
			found = true
		}
	}
	if !found {
		t.Fatal("no extension-less name: the backend will not be found on Linux or macOS")
	}
}

func TestCandidatePathsKeepTheWindowsNames(t *testing.T) {
	found := false
	for _, path := range CandidatePaths("C:/app") {
		if strings.HasSuffix(path, ".exe") {
			found = true
		}
	}
	if !found {
		t.Fatal("the Windows names were dropped")
	}
}

func TestCandidatePathsStayNearTheApplication(t *testing.T) {
	// ".." is allowed for the layout the release pipeline builds, but nothing
	// may climb above the application directory: a search that walks the whole
	// disk eventually finds somebody else's binary.
	for _, path := range CandidatePaths("/opt/app") {
		if strings.Contains(filepath.ToSlash(path), "/../..") {
			t.Fatalf("path climbs too far up: %s", path)
		}
	}
}

func TestFindBackendPrefersTheOneNextToTheApplication(t *testing.T) {
	dir := t.TempDir()
	next := filepath.Join(dir, "torrsyncplayer")
	if err := os.WriteFile(next, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A decoy one directory deeper must not win.
	if err := os.MkdirAll(filepath.Join(dir, "torrserver"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "torrserver", "torrsyncplayer"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := FindBackend(dir)
	if err != nil {
		t.Fatal(err)
	}
	if found != next {
		t.Fatalf("found %s, want the one next to the application %s", found, next)
	}
}

func TestFindBackendRefusesAWorldWritableBinary(t *testing.T) {
	// Anyone who can write the binary chooses which code runs as this user.
	dir := t.TempDir()
	target := filepath.Join(dir, "torrsyncplayer")
	if err := os.WriteFile(target, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Set explicitly: WriteFile's mode goes through the umask, so asking for
	// 0777 does not reliably produce a world-writable file.
	if err := os.Chmod(target, 0o777); err != nil {
		t.Fatal(err)
	}

	// The assertion follows what this platform's check can even see, and asking
	// both platforms the same question is how the Windows run failed: there Go
	// reports 0666 for a temp file whatever the permissions are, so the Unix
	// check refuses every backend on that platform. Asserting the *coupling*
	// (if the platform calls it world-writable, the search must refuse it) pins
	// the behaviour without pretending the two platforms agree.
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}

	found, findErr := FindBackend(dir)
	if worldWritable(info) {
		if findErr == nil {
			t.Fatal("a world-writable backend was accepted")
		}
		return
	}
	if findErr != nil {
		t.Fatalf("a backend this platform does not consider world-writable was refused: %v", findErr)
	}
	if found != target {
		t.Fatalf("found %s, want %s", found, target)
	}
}

func TestFindBackendReportsWhatItLookedAt(t *testing.T) {
	// A message naming only "not found" is the message that made bug-0066 take
	// as long as it did.
	_, err := FindBackend(t.TempDir())
	if err == nil {
		t.Fatal("expected a miss")
	}
	for _, want := range []string{"torrsyncplayer", "not found"} {
		if !strings.Contains(strings.ToLower(err.Error()), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

var _ = strconv.Itoa

// listenAny binds a port on every interface, the way the backend itself binds.
func listenAny(port int) (net.Listener, error) {
	return net.Listen("tcp", ":"+strconv.Itoa(port))
}
