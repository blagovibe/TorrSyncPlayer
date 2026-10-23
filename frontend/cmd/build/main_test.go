package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildPutsBothBinariesInTheRequestedDirectory pins the invariant that
// broke: both builds run with a different `cmd.Dir`, so a relative output path
// is resolved against each module's own directory and the two binaries end up
// in two different places. The client looks for the backend beside itself, so
// that is not cosmetic — the pair does not start.
//
// Running the real build here would not work anyway: the relative module paths
// ("." and "../backend") need the working directory to be frontend/, so the
// test would write into the source tree.
func TestBuildPutsBothBinariesInTheRequestedDirectory(t *testing.T) {
	dir := t.TempDir()
	defer chdir(t, dir)()

	var outputs []string
	record := func(dir, output string, pkg, ldflags []string) error {
		outputs = append(outputs, output)
		return os.WriteFile(output, []byte("binary"), 0o755)
	}

	if err := build("dist", record); err != nil {
		t.Fatalf("build: %v", err)
	}

	if len(outputs) != 2 {
		t.Fatalf("want two builds, got %d: %v", len(outputs), outputs)
	}

	want := filepath.Join(dir, "dist")
	for _, output := range outputs {
		if !filepath.IsAbs(output) {
			t.Errorf("output %q is relative: go build resolves it against its own module directory, not the directory the caller asked for", output)
		}
		// Both sides are resolved before comparing, and not as a formality: on
		// macOS /var is a symlink to /private/var, so t.TempDir() hands out
		// /var/folders/... while filepath.Abs — which is what the build script
		// uses and what os.Getwd returns — produces /private/var/folders/...
		// for the same directory. Comparing the strings made a passing build
		// look broken on a platform where it is not.
		if got, want := resolveSymlinks(t, filepath.Dir(output)), resolveSymlinks(t, want); got != want {
			t.Errorf("output %q landed in %q, want %q", output, got, want)
		}
	}
}

// resolveSymlinks returns the directory a path really names, so a comparison is
// about the directory rather than about one spelling of it.
func resolveSymlinks(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

func TestBuildFailsWhenABinaryDoesNotCompile(t *testing.T) {
	defer chdir(t, t.TempDir())()

	refusing := func(dir, output string, pkg, ldflags []string) error {
		return os.ErrPermission
	}

	err := build("dist", refusing)
	if err == nil {
		t.Fatal("build reported success although neither binary was produced")
	}
	// The client is built first, so its failure is the one that surfaces, and
	// the message has to say which half it was — otherwise a broken build
	// reads like a broken network.
	if !strings.Contains(err.Error(), "client") {
		t.Errorf("error %q does not say which half failed; want it to name the client", err)
	}
}

func TestBuildReportsAFailingBackend(t *testing.T) {
	defer chdir(t, t.TempDir())()

	backendBroken := func(dir, output string, pkg, ldflags []string) error {
		if dir == "." {
			return os.WriteFile(output, []byte("binary"), 0o755)
		}
		return os.ErrPermission
	}

	err := build("dist", backendBroken)
	if err == nil {
		t.Fatal("build reported success although the backend did not compile")
	}
	if !strings.Contains(err.Error(), "backend") {
		t.Errorf("error %q does not name the backend; a failure there reads as a client failure", err)
	}
}

// TestTheBackendIsStampedWithTheReleaseVersion pins bug-0151: the artifacts
// reported version=dev because the build passed no ldflags, and the fix has to
// reach the backend — the binary that answers /api/v1/version.
func TestTheBackendIsStampedWithTheReleaseVersion(t *testing.T) {
	t.Setenv("VERSION", "v0.5.0")
	t.Setenv("COMMIT", "deadbeef")
	defer chdir(t, t.TempDir())()

	var clientFlags, backendFlags []string
	record := func(dir, output string, pkg, ldflags []string) error {
		if dir == "." {
			clientFlags = append([]string(nil), ldflags...)
		} else {
			backendFlags = append([]string(nil), ldflags...)
		}
		return os.WriteFile(output, []byte("binary"), 0o755)
	}
	if err := build("dist", record); err != nil {
		t.Fatalf("build: %v", err)
	}

	stamped := strings.Join(backendFlags, " ")
	if !strings.Contains(stamped, versionPackage+".Version=0.5.0") {
		t.Errorf("the backend was not stamped with the release version: %q", stamped)
	}
	if !strings.Contains(stamped, versionPackage+".Commit=deadbeef") {
		t.Errorf("the backend was not stamped with the commit: %q", stamped)
	}
	if len(clientFlags) != 0 {
		t.Errorf("the client was stamped with the backend's variables: %v", clientFlags)
	}
}

// TestALocalBuildStaysDev keeps the other half honest: a binary built by hand
// must not claim to be a release.
func TestALocalBuildStaysDev(t *testing.T) {
	t.Setenv("VERSION", "")
	t.Setenv("COMMIT", "")

	stamped := strings.Join(versionLdflags(), " ")
	if !strings.Contains(stamped, versionPackage+".Version=dev") {
		t.Errorf("a local build does not say dev: %q", stamped)
	}
	if !strings.Contains(stamped, versionPackage+".Commit=unknown") {
		t.Errorf("a local build does not say unknown: %q", stamped)
	}
}

// TestBuildArgsCarryTheLdflags pins the wiring between the stamp and the
// command: a variable computed but not passed to go build is the same defect
// one step later.
func TestBuildArgsCarryTheLdflags(t *testing.T) {
	args := buildArgs("/tmp/out", []string{"./cmd/server"}, []string{"-X", "pkg.Version=1.2.3"})
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-ldflags" && args[i+1] == "-X pkg.Version=1.2.3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the ldflags never reach go build: %q", strings.Join(args, " "))
	}

	bare := buildArgs("/tmp/out", nil, nil)
	for _, arg := range bare {
		if arg == "-ldflags" {
			t.Fatalf("a build with nothing to stamp was given an empty -ldflags: %q", strings.Join(bare, " "))
		}
	}
}

// chdir moves into dir and returns a function that puts the old one back.
func chdir(t *testing.T, dir string) func() {	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	return func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}
}
