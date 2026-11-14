// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

// Shared fixtures for the torrent package's tests. The gateway these used to
// come with is gone (deviation-0166); the magnet they describe was never its
// own — every test in this package needs the same deterministic one.
const (
	testMagnet = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=movie"
	testHash   = "0123456789abcdef0123456789abcdef01234567"
)
