// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package backend owns the lifecycle of the embedded Go backend.
//
// It exists as its own unit because this logic used to live in the Qt
// frontend's main.cpp, which was in no test target at all. That blind spot
// already cost a release once — a flag the backend no longer had reached a
// Windows binary through a fully green pipeline — and it cost a second one
// later, when the search for the backend executable listed only *.exe and the
// application started with no backend at all on Linux and macOS. Both were
// invisible until the thing was actually run.
//
// Everything here is deliberately free of GUI types so it can be tested.
package backend

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// KnownFlags mirrors parseFlags in backend/cmd/server/main.go.
//
// An unknown flag is not a warning to the backend, it is
// "flag provided but not defined" and os.Exit(2): the application simply does
// not start. That is why the list is asserted here rather than assumed.
func KnownFlags() []string {
	return []string{
		"port", "addr", "token",
		"data-dir", "disk-storage",
		"tls-cert", "tls-key", "tls", "auto-tls",
		"enable-profiling",
		"gateway-url", "gateway-user", "gateway-pass", "gateway-ca-cert",
	}
}

// GenerateToken mints the access token for the embedded backend.
//
// The token belongs to this process, not to the backend. Left to itself the
// backend generates a random one and prints it to stdout, and nobody reads
// that: the client then cannot talk to its own backend at all and every
// request comes back 401. Passing the secret in is the mode --token already
// documents — "тот, кто его задал, знает его и так".
func GenerateToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	// Fixed length on purpose: the backend compares with
	// crypto/subtle.ConstantTimeCompare, and a varying length would leak the
	// secret through the time the comparison takes.
	return hex.EncodeToString(raw), nil
}

// PickFreePort asks the kernel for a port nobody is listening on.
//
// A fixed port is what a backend used to take: a second instance on the same
// machine failed to bind and then quietly talked to somebody else's server.
func PickFreePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("pick a free port: %w", err)
	}
	defer func() { _ = listener.Close() }()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %T", listener.Addr())
	}
	return addr.Port, nil
}

// CandidatePaths lists where an already-built backend may live, given the
// directory this application's executable sits in.
//
// The extension-less names are the ones that matter: that is what the project
// builds on Linux and macOS, and what the release pipeline puts beside the
// application. The .exe names are kept for Windows.
func CandidatePaths(appDir string) []string {
	return []string{
		filepath.Join(appDir, "torrsyncplayer"),
		filepath.Join(appDir, "torrserver", "torrsyncplayer"),
		filepath.Join(appDir, "..", "torrserver", "torrsyncplayer"),
		filepath.Join(appDir, "..", "..", "backend", "build", "torrsyncplayer"),
		filepath.Join(appDir, "torrsyncplayer.exe"),
		filepath.Join(appDir, "torrserver", "torrsyncplayer.exe"),
		filepath.Join(appDir, "..", "torrserver", "torrsyncplayer.exe"),
		filepath.Join(appDir, "torrserver.exe"),
	}
}

// FindBackend returns the first candidate that exists and is not world-writable.
//
// A world-writable binary is refused with a warning rather than silently run:
// anyone able to write it would be choosing which code executes as this user.
func FindBackend(appDir string) (string, error) {
	for _, path := range CandidatePaths(appDir) {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode().Perm()&0o002 != 0 {
			fmt.Fprintf(os.Stderr,
				"backend binary %s is world-writable — refusing to run it\n", path)
			continue
		}
		return path, nil
	}
	return "", fmt.Errorf("backend executable not found near %s (looked at %s)",
		appDir, strings.Join(CandidatePaths(appDir), ", "))
}

// Args builds the command line for the embedded backend.
func Args(port int, token string) []string {
	args := []string{"--port", fmt.Sprint(port)}
	if token != "" {
		// An empty token would pin the backend to an empty secret instead of
		// letting it generate a random one, which is the failure being fixed.
		args = append(args, "--token", token)
	}
	return args
}

// UnknownFlag reports the first argument the backend would refuse.
//
// Kept as a function rather than a comment so a test can hold the two lists
// against each other; that is the only thing standing between a renamed flag
// and a binary that exits 2 on startup.
func UnknownFlag(args []string) (string, bool) {
	known := make(map[string]struct{}, len(KnownFlags()))
	for _, name := range KnownFlags() {
		known[name] = struct{}{}
	}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		name := strings.TrimPrefix(arg, "--")
		if _, ok := known[name]; !ok {
			return arg, false
		}
	}
	return "", true
}

// Process is the backend the application started, and the only thing allowed to
// wait on it.
//
// Three separate things want to know when the backend ends: the startup check,
// which must not sit out a full timeout waiting for a process that has already
// died; the window, which must quit rather than show a list that will never
// update; and the shutdown, which must wait for a clean exit before insisting.
// exec.Cmd.Wait may only be called once, and calling it twice is an error
// rather than a second answer, so one goroutine waits and everybody reads the
// result.
type Process struct {
	cmd *exec.Cmd

	// exited is closed once the backend has ended.
	exited chan struct{}

	// exitErr is the result of that wait. Read it only after <exited, which is
	// what makes it safe: the close happens after the write.
	exitErr error
}

// Start launches the backend and returns the running process.
func Start(path string, port int, token string) (*Process, error) {
	cmd := exec.Command(path, Args(port, token)...)

	// The backend's own output is the only place the reason for a failure will
	// ever appear — a port clash, a bad flag, a storage directory that is not
	// writable — so it goes to this process's stderr rather than to a buffer
	// nobody reads.
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start backend %s: %w", path, err)
	}

	process := &Process{cmd: cmd, exited: make(chan struct{})}
	go func() {
		process.exitErr = cmd.Wait()
		close(process.exited)
	}()
	return process, nil
}

// Exited is closed when the backend has ended, however it ended.
func (p *Process) Exited() <-chan struct{} { return p.exited }

// ExitError is how the backend ended. Only meaningful once Exited is closed.
func (p *Process) ExitError() error { return p.exitErr }

// Running reports whether the backend is still alive.
func (p *Process) Running() bool {
	if p == nil || p.cmd.Process == nil {
		return false
	}
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

// Stop asks the backend to finish, then insists.
//
// It sends SIGTERM rather than killing outright because the backend flushes
// torrent ownership on the way out, and a plain kill loses it.
func (p *Process) Stop() {
	if !p.Running() {
		return
	}

	_ = p.cmd.Process.Signal(syscall.SIGTERM)

	select {
	case <-p.exited:
	case <-time.After(5 * time.Second):
		// It did not go on its own. Insist.
		_ = p.cmd.Process.Kill()
		<-p.exited
	}
}

// DescribeExit turns a backend's death into something worth reading.
//
// The exit status alone says almost nothing, and the reason is already in the
// output above it, so the message points there instead of restating a number.
func DescribeExit(err error) string {
	if err == nil {
		return "the backend stopped"
	}

	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Sprintf("the backend stopped with %s — its own output above says why", exit)
	}
	return fmt.Sprintf("the backend stopped: %v", err)
}
