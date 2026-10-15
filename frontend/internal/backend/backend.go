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
		"port", "addr", "token", "token-stdin",
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
// request comes back 401. The secret travels to the child on its standard
// input (--token-stdin), so it never appears in the process list — it used to
// be an argument, readable by anyone who could run ps (deviation-0061).
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
	// 127.0.0.1, not ":0": a free port is wanted, not a listener on every
	// interface. gosec G102 is right about the original — ":0" really does
	// bind all of them, and this port travels in a URL to mpv on the same
	// machine, so it should never have been reachable from outside it.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
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
// What "world-writable" means is per platform — see worldwritable_unix.go and
// worldwritable_windows.go, because on Windows the mode bits answer a different
// question than they do here.
func FindBackend(appDir string) (string, error) {
	for _, path := range CandidatePaths(appDir) {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if worldWritable(info) {
			fmt.Fprintf(os.Stderr,
				"backend binary %s is world-writable — refusing to run it\n", path)
			continue
		}
		return path, nil
	}
	return "", fmt.Errorf("backend executable not found near %s (looked at %s)",
		appDir, strings.Join(CandidatePaths(appDir), ", "))
}

// Launch describes the backend child this application starts.
//
// Grown from (path, port, token) because the child is told more than where to
// listen: which storage to use and whether an htorrent gateway owns the
// torrents. Which channel carries what is deliberate — see Args and LaunchEnv.
type Launch struct {
	// Executable is the backend binary.
	Executable string
	// Port the backend should listen on.
	Port int
	// Token the backend must accept. It travels on the child's standard input,
	// never on the command line (deviation-0061).
	Token string

	// DiskStorage writes pieces under DataDir instead of keeping them in
	// memory. DataDir also holds the torrent registry, which is what makes a
	// restart resume rather than start over.
	DiskStorage bool
	// DataDir is where pieces and persistent data go. Empty with DiskStorage
	// unset disables persistence: memory storage leaves nothing on disk at
	// all.
	DataDir string

	// GatewayURL switches the backend into remoting mode, where an htorrent
	// gateway owns the torrents; user and password are required together, the
	// CA certificate is optional.
	GatewayURL    string
	GatewayUser   string
	GatewayPass   string
	GatewayCACert string
}

// Args builds the command line for the embedded backend.
//
// The access token is not here: it travels on the child's standard input
// (--token-stdin), so it never appears in the process list (deviation-0061).
// The gateway password is not here either — it travels in the environment.
// What is here may be read by anyone: where to listen and which storage the
// window chose.
//
// Storage travels as flags, not as the DATA_DIR/DISK_STORAGE environment the
// backend also accepts: a flag is not a default, so nothing inherited from a
// shell can stand in for the choice made in the window. The empty directory
// passed in memory mode is on purpose — it is the backend's documented way to
// keep nothing on disk.
func Args(l Launch) []string {
	args := []string{"--port", fmt.Sprint(l.Port)}
	if l.Token != "" {
		// An empty token would pin the backend to an empty secret instead of
		// letting it generate a random one, which is the failure being fixed.
		// With no token to write, the flag that asks for one is left out too.
		args = append(args, "--token-stdin")
	}
	args = append(args, "--data-dir", l.DataDir)
	if l.DiskStorage {
		args = append(args, "--disk-storage")
	}
	return args
}

// managedEnv is the child environment the launch decides.
//
// DATA_DIR and DISK_STORAGE are in the list even though storage travels as
// flags: the backend reads them as flag defaults, and a leftover in the
// environment must not stand in for the choice made in the window.
func managedEnv(entry string) bool {
	key, _, _ := strings.Cut(entry, "=")
	switch strings.ToUpper(key) {
	case "DATA_DIR", "DISK_STORAGE",
		"GATEWAY_URL", "GATEWAY_USER", "GATEWAY_PASS", "GATEWAY_CA_CERT":
		return true
	}
	return false
}

// LaunchEnv builds the environment for the backend child.
//
// The child inherits this process's environment, and the backend reads the
// gateway settings from it. The variables the launch decides are replaced
// rather than appended to: appending would win for a value the launch sets,
// but a variable the launch switches off — a gateway removed in the settings —
// would survive and keep redirecting the backend. Anything unrelated is
// passed through untouched.
func LaunchEnv(parent []string, l Launch) []string {
	env := make([]string, 0, len(parent)+4)
	for _, entry := range parent {
		if managedEnv(entry) {
			continue
		}
		env = append(env, entry)
	}

	if l.GatewayURL != "" {
		env = append(env,
			"GATEWAY_URL="+l.GatewayURL,
			"GATEWAY_USER="+l.GatewayUser,
			"GATEWAY_PASS="+l.GatewayPass,
		)
		if l.GatewayCACert != "" {
			env = append(env, "GATEWAY_CA_CERT="+l.GatewayCACert)
		}
	}
	return env
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
func Start(l Launch) (*Process, error) {
	// #nosec G204 -- l.Executable is a binary this process resolved from its
	// own installation (CandidatePaths beside the client, or a built backend),
	// not user input; the args are flags built in Args here, and the
	// environment is this process's own with the launch's variables replaced.
	cmd := exec.Command(l.Executable, Args(l)...)
	cmd.Env = LaunchEnv(os.Environ(), l)

	// The secret goes to the child's standard input: written once, with the
	// newline its reader expects, and the pipe is closed when the reader is
	// exhausted. It never appears in the process list (deviation-0061).
	if l.Token != "" {
		cmd.Stdin = strings.NewReader(l.Token + "\n")
	}

	// The backend's own output is the only place the reason for a failure will
	// ever appear — a port clash, a bad flag, a storage directory that is not
	// writable — so it goes to this process's stderr rather than to a buffer
	// nobody reads.
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start backend %s: %w", l.Executable, err)
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
