// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

//go:build !windows

package player

import "net"

// dial opens mpv's JSON IPC unix socket. On Windows mpv listens on a named
// pipe instead, and the addressing differs; see dial_windows.go.
func dial(address string) (net.Conn, error) {
	return net.Dial("unix", address)
}
