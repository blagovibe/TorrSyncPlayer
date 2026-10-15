// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package player drives an external mpv process.
//
// mpv is run as a separate process rather than linked as libmpv. That is what
// Multiplex does, and it buys three things:
//
//   - No libmpv to link against, so no C ABI in the build and no header
//     dependency per platform.
//   - A crash in the decoder takes the player window with it, not the
//     application.
//   - mpv accepts custom HTTP headers on the command line
//     (--http-header-fields), so the media endpoint is authenticated with the
//     ordinary access token instead of a bearer value in the URL.
//
// Четвёртое — теперь главное: оно то, что позволило убрать билеты из /stream
// целиком (idea-0066, шаг 3: удалили билетную механику, закрыт deviation-0029).
package player

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// dialer opens whatever transport mpv listens on: a unix socket on Unix, a
// named pipe on Windows. It is a field rather than a call so a test can supply
// an in-memory pipe — net.Pipe works on every platform, and a unix socket does
// not, which is how the fake-mpv tests came to fail the first Windows run.
type dialer func(address string) (net.Conn, error)

// Player is a running mpv, controlled over its JSON IPC socket.
type Player struct {
	cmd  *exec.Cmd
	sock string
	log  *lockedBuffer
	dial dialer

	closeOnce sync.Once
}

// SocketName is what mpv is told to listen on, and therefore what dial opens.
func (p *Player) SocketName() string { return p.sock }

// Log is what mpv said about itself. Kept so a failure can quote it instead of
// leaving a reader to guess what the player was doing.
func (p *Player) Log() string { return p.log.String() }

// lockedBuffer is a concurrency-safe io.Writer for a process's output.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Options configures a Player.
type Options struct {
	// Binary is the mpv executable; "mpv" resolves it on PATH.
	Binary string
	// SocketPath is the JSON IPC socket to listen on.
	SocketPath string
	// ExtraArgs are appended last, so they win over the defaults.
	ExtraArgs []string
}

// Start launches mpv with JSON IPC enabled and waits until the socket answers.
//
// Headers is passed through --http-header-fields, one "Name: value" per entry.
// mpv sends them on every request it makes for the stream, which is what lets
// the media endpoint be a protected route instead of a public one with a
// held-over credential in the URL.
func Start(ctx context.Context, opts Options, headers []string) (*Player, error) {
	binary := opts.Binary
	if binary == "" {
		binary = "mpv"
	}
	if opts.SocketPath == "" {
		return nil, errors.New("player: no IPC socket path given")
	}

	// #nosec G204 -- the binary comes from the player settings, chosen by the
	// person running the app; the socket path and headers are this process's
	// own values, and the extra args are the documented way to pass a custom
	// mpv (opts.Binary itself is that setting, not an argument).
	cmd := exec.Command(binary, Args(opts.SocketPath, headers, opts.ExtraArgs...)...)

	// mpv's own output is kept, not discarded. It was thrown away, and a player
	// that fails to open a file or a track then fails silently — which is how a
	// track test spent fifteen seconds guessing at the reason on a CI runner.
	output := &lockedBuffer{}
	cmd.Stdout = output
	cmd.Stderr = output

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("player: start %s: %w", binary, err)
	}

	p := &Player{cmd: cmd, sock: opts.SocketPath, log: output, dial: dial}
	if err := p.waitReady(ctx); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return nil, err
	}
	return p, nil
}

// waitReady polls the IPC socket. mpv creates it a moment after launch, and
// there is no readiness signal to wait on that is worth the complexity.
func (p *Player) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := p.dial(p.sock)
		if err == nil {
			return conn.Close()
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("player: mpv did not open %s in time", p.sock)
}

// Play loads a URL and starts playback.
func (p *Player) Play(ctx context.Context, url string) error {
	return p.command(ctx, [][]any{
		{"loadfile", url, "replace"},
		{"set_property", "pause", false},
	})
}

// Pause stops playback without unloading.
func (p *Player) Pause(ctx context.Context) error {
	return p.command(ctx, [][]any{{"set_property", "pause", true}})
}

// Resume continues playback.
func (p *Player) Resume(ctx context.Context) error {
	return p.command(ctx, [][]any{{"set_property", "pause", false}})
}

// Seek moves to an absolute position in seconds.
func (p *Player) Seek(ctx context.Context, seconds float64) error {
	return p.command(ctx, [][]any{{"seek", seconds, "absolute"}})
}

// Position reports the current playback position in seconds.
//
// The buffer report the room needs travels on every tick, so this is on a hot
// path and stays a single round trip.
func (p *Player) Position(ctx context.Context) (float64, error) {
	var reply struct {
		Error string  `json:"error"`
		Data  float64 `json:"data"`
	}
	if err := p.request(ctx, []any{"get_property", "time-pos"}, &reply); err != nil {
		return 0, err
	}
	if reply.Error != "" && reply.Error != "success" {
		return 0, fmt.Errorf("player: %s", reply.Error)
	}
	return reply.Data, nil
}

// Close stops mpv.
func (p *Player) Close() {
	p.closeOnce.Do(func() {
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
			_, _ = p.cmd.Process.Wait()
		}
	})
}

// maxSkippedEvents bounds how many async event lines one request steps over
// before it stops and says so. Event traffic is finite — a file start produces
// a handful — so a longer stream means something else is wrong, and spinning
// until the context deadline would turn that into a slow silence.
const maxSkippedEvents = 32

// command sends a batch and ignores the reply, for fire-and-forget control.
func (p *Player) command(ctx context.Context, commands [][]any) error {
	conn, err := p.dial(p.sock)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// One connection per batch: mpv answers each request in order, so a
	// persistent connection would need its own reader goroutine and a
	// demultiplexer. Playback control is infrequent; polling is not, and it
	// opens its own.
	enc := json.NewEncoder(conn)
	for _, command := range commands {
		if err := enc.Encode(map[string]any{"command": command}); err != nil {
			return fmt.Errorf("player: send: %w", err)
		}
	}
	return nil
}

// request sends one command and reads its reply.
//
// Replies are matched by shape, not by order: mpv broadcasts async events to
// every connected IPC client, and a single blind read can catch an event line
// instead of the answer. Decoded into a reply shape that event is Error="" with
// no data — indistinguishable from "the file is still loading". A track query in
// that moment reports no tracks; worse, a position query in that moment reports
// position 0 with no error, and the buffer reporter hands the room "the start of
// the film". So event lines are skipped: a command reply always carries
// request_id (zero when the command did not set one), and an async event never
// does.
func (p *Player) request(ctx context.Context, command []any, out any) error {
	conn, err := p.dial(p.sock)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if err := json.NewEncoder(conn).Encode(map[string]any{"command": command}); err != nil {
		return fmt.Errorf("player: send: %w", err)
	}

	reader := bufio.NewReader(conn)
	for skipped := 0; ; skipped++ {
		if skipped >= maxSkippedEvents {
			return fmt.Errorf("player: too many event lines before the reply")
		}
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return fmt.Errorf("player: read: %w", err)
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(line, &envelope); err != nil {
			return fmt.Errorf("player: decode: %w", err)
		}
		if _, isEvent := envelope["event"]; isEvent {
			if _, hasID := envelope["request_id"]; !hasID {
				continue
			}
		}
		if err := json.Unmarshal(line, out); err != nil {
			return fmt.Errorf("player: decode: %w", err)
		}
		return nil
	}
}

// Args builds an mpv command line. Exposed for tests: what mpv is told matters
// as much as what it is asked, and a wrong flag makes mpv exit rather than
// misbehave.
func Args(socketPath string, headers []string, extra ...string) []string {
	args := []string{
		"--no-config",
		"--idle=yes",
		"--keep-open=yes",
		"--no-input-default-bindings",
		"--input-ipc-server=" + socketPath,
	}
	for _, header := range headers {
		args = append(args, "--http-header-fields="+header)
	}
	return append(args, extra...)
}

// FormatHeader renders one "Name: value" pair the way mpv's list option wants
// it. A newline inside a value would split it into two headers, so it is
// rejected rather than silently mangled.
func FormatHeader(name, value string) (string, error) {
	if name == "" {
		return "", errors.New("player: empty header name")
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("player: header %q value contains a newline", name)
	}
	return name + ": " + value, nil
}
