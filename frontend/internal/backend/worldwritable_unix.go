// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

//go:build !windows

package backend

import "os"

// worldWritable reports whether a file may be written by everyone.
//
// On Unix the mode bits answer this directly, and the answer matters: anyone who
// can write the binary is choosing which code runs as this user.
func worldWritable(info os.FileInfo) bool {
	return info.Mode().Perm()&0o002 != 0
}
