// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

//go:build windows

package player

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// dial opens mpv's JSON IPC named pipe.
//
// Windows has no unix sockets, so neither net.Dial nor mpv's own address
// spelling can be reused: mpv is handed a bare name with
// --input-ipc-server, and it is opened here as \\.\pipe\<name>.
//
// Not exercised on the machine this was written on. It is here so the
// three-platform promise is not quietly broken at the last step, and it is
// marked so nobody mistakes it for something already proven.
func dial(address string) (net.Conn, error) {
	name, err := syscall.UTF16PtrFromString(`\\.\pipe\` + address)
	if err != nil {
		return nil, fmt.Errorf("player: pipe name %q: %w", address, err)
	}

	const (
		readWrite    = syscall.GENERIC_READ | syscall.GENERIC_WRITE
		noSharing    = 0
		openExisting = 3
	)

	handle, err := syscall.CreateFile(name, readWrite, noSharing, nil, openExisting, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("player: open pipe %s: %w", address, err)
	}

	file := os.NewFile(uintptr(handle), `\\.\pipe\`+address)
	return newPipeConn(file, address), nil
}

// pipeConn adapts a connected pipe handle to net.Conn.
//
// os.File already does the blocking read and write; what is missing is the
// deadline handling net.Conn promises, and a player that cannot time out is a
// player that hangs the UI.
type pipeConn struct {
	file    *os.File
	address string
}

func newPipeConn(file *os.File, address string) net.Conn {
	return &pipeConn{file: file, address: address}
}

func (c *pipeConn) Read(b []byte) (int, error)  { return c.file.Read(b) }
func (c *pipeConn) Write(b []byte) (int, error) { return c.file.Write(b) }

func (c *pipeConn) Close() error {
	return c.file.Close()
}

func (c *pipeConn) LocalAddr() net.Addr  { return pipeAddr(c.address) }
func (c *pipeConn) RemoteAddr() net.Addr { return pipeAddr(c.address) }

func (c *pipeConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline is not implementable on a synchronous pipe handle: there is
// no overlapped I/O to cancel. It reports the request rather than pretending,
// so a caller that depends on it fails loudly instead of hanging.
func (c *pipeConn) SetReadDeadline(time.Time) error {
	return fmt.Errorf("player: read deadlines are not supported on a named pipe (%s)", c.address)
}

func (c *pipeConn) SetWriteDeadline(time.Time) error {
	return fmt.Errorf("player: write deadlines are not supported on a named pipe (%s)", c.address)
}

type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }
