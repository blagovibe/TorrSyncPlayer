// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

//go:build windows
// +build windows

// Package main provides Windows-specific console initialization.
// Sets UTF-8 codepage for proper Cyrillic output in Windows console.
package main

import (
	"syscall"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	setConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
	setConsoleCP       = kernel32.NewProc("SetConsoleCP")
)

func init() {
	// Set console to UTF-8 mode for proper Cyrillic output on Windows
	// Windows console uses CP866/CP1251 by default, but Go outputs UTF-8
	// CP_UTF8 = 65001
	setConsoleOutputCP.Call(uintptr(65001))
	setConsoleCP.Call(uintptr(65001))
}
