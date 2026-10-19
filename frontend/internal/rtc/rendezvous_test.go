// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The rendezvous is the one part of the room the test suite cannot run
// hermetically: the testbed is two loopback nodes that meet by handing over
// an address. Everything it proved about production therefore came from
// reading, and the room did not work: on a real network, four of the five
// bootstrap addresses libp2p ships do not resolve, and the one that does
// answers alone (bug-0121).
//
// The room no longer meets through the rendezvous — the address in the code
// does that, and the lookup by peer id is the fallback. What the rendezvous is
// for now is the routing table: the public address, the relay candidates, and
// the way to find a peer whose address stopped working. These tests pin the
// two rules that follow — own seeds, and a seed that does not answer costs
// that seed and not the room.

// TestTheProjectsOwnSeedsAreAddressesThatDial pins the first rule. The seeds
// are the project's own, and every one of them is an address that can
// actually be dialled: a /dnsaddr/ address depends on a name publishing SRV
// records, and bootstrap.libp2p.io publishes none, which is precisely how four
// of the five default seeds became dead weight.
func TestTheProjectsOwnSeedsAreAddressesThatDial(t *testing.T) {
	for _, raw := range ownSeedAddrs {
		_, err := multiaddr.NewMultiaddr(raw)
		assert.NoError(t, err, "a seed that does not parse is a seed nobody can dial: %s", raw)
	}

	own := ownSeeds()
	require.GreaterOrEqual(t, len(own), 2,
		"one reachable rendezvous node is a single point of failure, not a fallback")
	require.Len(t, own, len(ownSeedAddrs), "every listed seed must reach the dial list")
	for _, info := range own {
		require.NotEmpty(t, info.Addrs, "a seed with no address cannot be dialled")
		for _, addr := range info.Addrs {
			assert.NotContains(t, addr.String(), "/dnsaddr/",
				"a /dnsaddr/ seed is undiallable while its name publishes no SRV record")
		}
	}
}

// TestDefaultBootstrapStartsWithTheProjectsOwnSeeds pins the fallback order:
// our seeds first, the library's after, one entry per peer. The point is not
// the order for its own sake — it is that a peer we can dial by address never
// gets replaced by a copy we cannot.
func TestDefaultBootstrapStartsWithTheProjectsOwnSeeds(t *testing.T) {
	seeds := defaultBootstrap()
	require.GreaterOrEqual(t, len(seeds), len(ownSeeds()))

	seen := map[peer.ID]struct{}{}
	for _, info := range seeds {
		_, dup := seen[info.ID]
		assert.False(t, dup, "a peer listed twice is dialled twice for nothing")
		seen[info.ID] = struct{}{}
	}
	for _, own := range ownSeeds() {
		found := false
		for _, info := range seeds {
			if info.ID == own.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "the project's own seed must reach the dial list")
	}
}

// TestASeedThatDoesNotAnswerDoesNotStopTheRoom pins the second rule. One dead
// seed must cost the room that seed, not the join: today Bootstrap returned
// the first Connect error, so a single unreachable address took the whole
// room down with it.
func TestASeedThatDoesNotAnswerDoesNotStopTheRoom(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	inv := testInvite()
	bed := startTestbed(t, ctx, inv)
	host := startNode(t, inv)

	require.NoError(t, host.Bootstrap(ctx, []peer.AddrInfo{deadSeed(t, inv), bed.AddrInfo()}),
		"a seed that does not answer must not fail the join")
	require.NoError(t, host.WaitReady(ctx), "the live seed still carries the room")
	assert.True(t, host.DHTReady())
}

// TestARoomWhoseSeedsAreAllDeadSaysTheRendezvousIsUnreachable pins the failure
// the other way round. A room that cannot meet anyone must say so in words a
// person can act on — the goal is that nobody has to guess what is happening —
// instead of reporting a raw dial error or, worse, claiming success.
func TestARoomWhoseSeedsAreAllDeadSaysTheRendezvousIsUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	host := startNode(t, testInvite())
	err := host.Bootstrap(ctx, []peer.AddrInfo{deadSeed(t, testInvite())})
	require.Error(t, err, "a room with no reachable rendezvous cannot be entered")
	assert.Contains(t, err.Error(), "rendezvous",
		"the error must name the rendezvous, not the dial: %v", err)
	assert.False(t, strings.Contains(err.Error(), "connect to bootstrap peer"),
		"the old error reported one dial, hiding that every seed failed")
}

// deadSeed is a rendezvous node that was alive and is now gone: a real peer
// identity on a closed port. A made-up address would fail for the wrong
// reason (identity mismatch) and prove nothing about a peer that simply does
// not answer.
func deadSeed(t *testing.T, inv Invite) peer.AddrInfo {
	t.Helper()
	gone := startNode(t, inv)
	info := gone.AddrInfo()
	require.NoError(t, gone.Close())
	return info
}
