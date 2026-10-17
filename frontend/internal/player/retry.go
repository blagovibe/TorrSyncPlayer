// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import "time"

// retryWhileBusy runs open until it succeeds, and gives up after within.
//
// It exists for one caller — the Windows named pipe, which hands out a single
// instance at a time and can still be releasing the previous connection when
// the next one asks (bug-0117) — but it is a platform-independent question:
// "try again, but not forever", with the answer shaped by the caller. So it
// lives here rather than behind a build tag, where it can be asked directly:
// only Windows has the pipe, and the three platforms run the same suite.
//
// isBusy decides whether a failure is worth another try. Everything else — a
// missing pipe, a refused name, a dead player — is returned at once: retrying
// those turns a clear failure into a slow one.
func retryWhileBusy(open func() error, isBusy func(error) bool, within, step time.Duration) error {
	deadline := time.Now().Add(within)
	for {
		err := open()
		if err == nil {
			return nil
		}
		if !isBusy(err) || !time.Now().Before(deadline) {
			return err
		}
		time.Sleep(step)
	}
}
