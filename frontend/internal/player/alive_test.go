// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A player that died is not a player that is slow, and the difference is the
// difference between a failure that says why and a failure that waits out
// its whole budget saying nothing.
//
// The player knows: it watches the process, and it says how it ended. The
// log is right there — it was always right there — and nobody had a way to
// ask for it.

// TestThePlayerSaysWhenItIsGone pins the difference.
func TestThePlayerSaysWhenItIsGone(t *testing.T) {
	if !mpvAvailable() {
		t.Skip("mpv is not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p, err := Start(ctx, Options{
		Binary:     "mpv",
		SocketPath: testSocketPath(t, "alive"),
		ExtraArgs:  []string{"--vo=null", "--ao=null"},
	}, nil)
	require.NoError(t, err, "start mpv")
	defer p.Close()

	require.True(t, p.Alive(), "a player that has just started is alive")
	require.NoError(t, p.ExitError(), "a living player has no exit error")

	// And now it is gone.
	require.NoError(t, p.cmd.Process.Kill(), "kill mpv")

	require.Eventually(t, func() bool { return !p.Alive() },
		10*time.Second, 50*time.Millisecond,
		"the player must notice that its process is gone")
	require.Error(t, p.ExitError(),
		"a player that is gone must say how it ended, or the failure says nothing")
}
