// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Both ends of a mesh dial each other — discovery is symmetric, so two
// participants that find each other in the same second open two streams between
// themselves, one from each side. Until now the mesh kept whichever arrived
// first and closed the other, which sounds harmless and is not: whichever side
// closed its copy of the loser was tearing down a stream the *other* side may
// have been the one using. The symptom was a room that met — both rosters
// showed each other — and then went silent, with the host's magnet never
// arriving at the guest.
//
// The rule has to be one both ends can compute without talking: the peer with
// the smaller id opened the stream that survives. That is the same rule the
// room already uses to decide who hosts after a host leaves, so it is a rule
// the codebase already believes and can reason about.

// dialPeer is a peer reached by a dial, i.e. one this side opened the stream
// for — the initiator is us.
func TestTwoPeersThatDialEachOtherKeepOneStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	host, gs := joinMesh(t, ctx, 1)
	guest := gs[0]

	require.Eventually(t, func() bool {
		return len(host.Peers()) == 1 && len(guest.Peers()) == 1
	}, 30*time.Second, 200*time.Millisecond, "the two must see each other")

	// Each side must still be holding the peer a moment later. A stream closed
	// by the wrong end takes the roster entry with it, and this is where that
	// showed up: both sides saw each other, and then one quietly forgot.
	time.Sleep(2 * time.Second)
	assert.Contains(t, host.Peers(), guest.SelfID(), "the host forgot the guest")
	assert.Contains(t, guest.Peers(), host.SelfID(), "the guest forgot the host")

	// And the room must still carry traffic afterwards — this is the symptom
	// that the duplicated stream used to cause.
	require.NoError(t, host.Send(ctx, NewMagnet(Magnet{
		Envelope:  Envelope{Type: TypeMagnet},
		MagnetURI: "magnet:?xt=urn:btih:race",
		InfoHash:  "race",
		Name:      "race.mkv",
	})))
	got := guest.WaitFor(ctx, "magnet", 30*time.Second)
	require.NotNil(t, got, "a room that met must still carry what the host sends")
}
