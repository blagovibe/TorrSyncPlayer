// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

//go:build windows

package backend

import "os"

// worldWritable reports whether a file may be written by everyone.
//
// It always says no here, and that is not carelessness: on Windows Go reports
// 0666 for a file created in a temp directory regardless of who can actually
// write it, so the Unix check refused *every* backend binary on this platform —
// a "Windows works" green run that actually meant the backend never started
// (the first three-platform CI run, 2026-10-15).
//
// Doing it properly means reading the file's ACL, and Go's standard library does
// not expose one. What is lost here is a check, not a false assurance: the
// search already refuses directories and missing files, and the binary is looked
// for next to this application rather than downloaded.
//
// If ACLs are ever needed, the place to add them is here, and only here.
func worldWritable(_ os.FileInfo) bool {
	return false
}
