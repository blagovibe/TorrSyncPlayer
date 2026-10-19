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
// bootstrap addresses libp2p ships do not resolve, the one that does answers
// alone, and a broadcast announce of the room key took 2m16s against a 60s
// join budget (bug-0121). These tests pin the three rules that follow from
// that measurement — own seeds, unreachable seeds do not stop the room, and
// the announce asks for the closest peers only.

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

// TestTheRoomIsAnnouncedQuicklyOnJoinAndWidenedAfterwards pins the third
// rule, and it is a pin rather than a proof: what makes the difference is not
// observable hermetically — on the loopback testbed every policy is instant.
// The measurements are on the public DHT: the wide announce of this exact key
// took 2m16s and blew past the join budget, so the join cannot wait for it;
// and the narrow one alone was not found by a guest that looked at the same
// time, so it cannot be the only publish either. Both halves are named and
// watched here, where flipping either back fails instead of rotting silently.
func TestTheRoomIsAnnouncedQuicklyOnJoinAndWidenedAfterwards(t *testing.T) {
	assert.False(t, announceOnJoinBroadcast,
		"the wide announce of this key took 2m16s on the public DHT — a join must not wait for it")
	assert.True(t, announceWideningBroadcast,
		"a record left on the closest peer alone was not found by a guest looking at the same time")
	assert.Greater(t, announceTimeout, time.Duration(0),
		"the announce needs its own deadline: the join budget is not its to spend")
	assert.LessOrEqual(t, announceTimeout, 20*time.Second,
		"an announce slower than this is a dead rendezvous, not a slow one")
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
