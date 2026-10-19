// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The NAT scenario runs on a privileged runner and nothing else. That is a
// good reason for it to be the *only* place the participant machinery is
// exercised: machinery nobody runs between CI runs rots, and this is the
// machinery that decides whether "the room works behind NAT" is a claim or a
// wish.
//
// So the same two participants — two real processes, two real meshes, a real
// DHT rendezvous, the host's film crossing to the guest — are run here on one
// host, where peers connect directly and therefore no relayed path is
// required. What this proves is that the mechanism is alive: the re-executed
// binary, the participant environment, the concurrent start, the handshake and
// the magnet. What it deliberately does not claim is NAT: that is
// TestARoomAcrossNAT's job, and only a separated pair can do it.
func TestTheNATParticipantMachineryWorksOnOneHost(t *testing.T) {
	// A room on one host meets in half a second, or does not meet at all: a
	// minute is already generous, and three minutes of it would eat the whole
	// budget and leave the retries nothing to retry with. Against a NAT the
	// window is the other way round, and that scenario asks for its own.
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	// No relay: one would have to listen on the namespace bridges, which do not
	// exist on a single host. The participants do not need one — they can reach
	// each other directly, which is exactly the difference this run is about.
	//
	// Retried with a fresh testbed per attempt, like the room's own mesh tests:
	// rendezvous is weather, and a scenario that goes red on weather teaches
	// everyone to ignore it.
	for attempt := 1; attempt <= 3; attempt++ {
		hostOut, guestOut, err := runNATAttempt(ctx, t,
			natParticipant{role: "host", bridge: "127.0.0.1"},
			natParticipant{role: "guest", bridge: "127.0.0.1"},
		)
		if err == nil {
			assert.Contains(t, hostOut, "OK", "the host participant did not finish:\n%s", hostOut)
			assert.Contains(t, guestOut, "OK", "the guest participant did not finish:\n%s", guestOut)
			return
		}
		t.Logf("attempt %d of 3 did not meet: %v", attempt, err)
	}
	t.Fatal("the two participants never met on one host in 3 attempts")
}
