// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The room protocol had no number. The greeting carried what it carried, and a
// peer that spoke a different shape was either ignored or an error. The invite
// code changed shape once already — three parts became four — and the only
// place that was visible was the code itself: a peer that spoke the old shape
// was a peer nobody could name.
//
// A number changes that from "something differs" to "here is what differs":
// the code says which shape the host speaks before the dial, and the greeting
// says which shape the peer speaks before the room admits it.

// TestTheInviteCodeSaysWhichProtocolTheHostSpeaks pins the first half: a fresh
// code says its version, and says it where the code can be read.
func TestTheInviteCodeSaysWhichProtocolTheHostSpeaks(t *testing.T) {
	invite := GenerateInvite()
	invite.Addrs = []string{"/ip4/127.0.0.1/tcp/4001"}

	code := invite.Encode()
	require.True(t, strings.HasPrefix(code, "v"+itoa(ProtocolVersion)+":"),
		"the code must say which protocol the host speaks: %q", code)

	back, err := ParseInviteCode(code)
	require.NoError(t, err)
	require.Equal(t, ProtocolVersion, back.Version)
	require.Equal(t, invite.Community, back.Community)
	require.Equal(t, invite.Password, back.Password)
	require.Equal(t, invite.Key, back.Key)
	require.Equal(t, invite.Addrs, back.Addrs)
}

// TestACodeWithoutAVersionIsTheLegacyOneAndStillOpensARoom pins the second
// half: a code from the build before the version existed still works, and is
// known to be old. The old shape is a subset of the new one, so refusing it
// would refuse a room that is perfectly fine.
func TestACodeWithoutAVersionIsTheLegacyOneAndStillOpensARoom(t *testing.T) {
	code := "community:password:key:/ip4/127.0.0.1/tcp/4001"

	invite, err := ParseInviteCode(code)
	require.NoError(t, err, "a code from an older build must still open a room: %v", err)
	require.Equal(t, ProtocolVersionLegacy, invite.Version)
	require.Equal(t, "community", invite.Community)
	require.Equal(t, "password", invite.Password)
	require.Equal(t, "key", invite.Key)
	require.Equal(t, []string{"/ip4/127.0.0.1/tcp/4001"}, invite.Addrs)
}

// TestACodeFromANewerVersionIsRefusedBeforeTheDial pins the refusal: a code
// that says "I speak a protocol you do not know" must not become a dial. The
// person pasting it is told what is wrong instead of watching a room that
// assembles in silence.
func TestACodeFromANewerVersionIsRefusedBeforeTheDial(t *testing.T) {
	code := "v99:community:password:key:/ip4/127.0.0.1/tcp/4001"

	_, err := ParseInviteCode(code)
	require.Error(t, err, "a code from a newer build must not parse")
	require.Contains(t, err.Error(), "newer version",
		"the refusal must say which way the versions differ: %v", err)
}

// TestTheGreetingCarriesTheProtocolVersion pins the wire side: this build says
// what it speaks, and a peer that says nothing is the legacy one rather than
// an unknown one.
func TestTheGreetingCarriesTheProtocolVersion(t *testing.T) {
	hello := NewHello("peer", "user", "name", true, nil)
	require.Equal(t, ProtocolVersion, hello.Version)

	var legacy Hello
	require.NoError(t, json.Unmarshal([]byte(`{"type":"hello","peerId":"p"}`), &legacy))
	require.Equal(t, 0, legacy.Version,
		"a greeting without the field is the legacy shape, and says so by saying nothing")
}

// TestTheRoomRefusesAPeerThatSpeaksANewerVersion is the refusal in the room:
// a peer whose greeting names a protocol this build does not know is not
// admitted, and the refusal is reported rather than swallowed.
func TestTheRoomRefusesAPeerThatSpeaksANewerVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := newFakeTransport("self", "newer")
	var mu sync.Mutex
	var roster []Peer
	var refused []string
	cb := &Callbacks{
		OnRoster: func(peers []Peer) {
			mu.Lock()
			defer mu.Unlock()
			roster = peers
		},
		OnPeerRefused: func(peerID string, theirVersion int) {
			mu.Lock()
			defer mu.Unlock()
			refused = append(refused, peerID)
		},
	}
	runRoom(ctx, t, tr, "self", "self-user", true, cb)

	hello := NewHello("newer", "newer-user", "newer", false, nil)
	hello.Version = ProtocolVersion + 1
	tr.feed("newer", hello)

	waitFor(t, 5*time.Second, "the peer to be refused", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(refused) == 1
	})

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"newer"}, refused)
	require.Empty(t, roster, "a refused peer must not be in the room")
}

// TestARefusedPeerCannotStillStealTheRoom pins what the refusal means: the
// connection stays open, so the peer keeps talking, and the room must not
// listen. A refused peer that can still hand the room a film, claim the host
// role or send commands has not been refused at all.
func TestARefusedPeerCannotStillStealTheRoom(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := newFakeTransport("self", "newer")
	var mu sync.Mutex
	var magnets []Magnet
	var hostChanged []string
	cb := &Callbacks{
		OnMagnet: func(m Magnet) {
			mu.Lock()
			defer mu.Unlock()
			magnets = append(magnets, m)
		},
		OnHostChanged: func(peerID string) {
			mu.Lock()
			defer mu.Unlock()
			hostChanged = append(hostChanged, peerID)
		},
	}
	room := runRoom(ctx, t, tr, "self", "self-user", true, cb)

	hello := NewHello("newer", "newer-user", "newer", false, nil)
	hello.Version = ProtocolVersion + 1
	tr.feed("newer", hello)

	// The refusal has to have happened before the rest is tried, or the test
	// proves nothing about a peer that was already in the room.
	waitFor(t, 5*time.Second, "the peer to be refused", func() bool {
		return room.refusedPeer("newer")
	})

	// Now the refused peer tries to take the room.
	tr.feed("newer", NewMagnet(Magnet{Envelope: Envelope{Type: TypeMagnet}, MagnetURI: "magnet:?xt=urn:btih:abc", InfoHash: "abc"}))
	tr.feed("newer", NewHost("newer"))
	tr.feed("newer", NewPause(false))

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.Empty(t, magnets, "a refused peer must not hand the room a film")
	require.Empty(t, hostChanged, "a refused peer must not become the host")
	require.Equal(t, "self", room.HostPeer(), "the host must still be the one who started the room")
}

// TestTheRoomAcceptsAPeerThatSpeaksTheLegacyVersion pins the other side: the
// old shape is a subset of the new one, so the peer is admitted — and marked,
// because a participant on an older build is a participant whose rooms will
// not have everything this build has.
func TestTheRoomAcceptsAPeerThatSpeaksTheLegacyVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := newFakeTransport("self", "old")
	var mu sync.Mutex
	var roster []Peer
	cb := &Callbacks{
		OnRoster: func(peers []Peer) {
			mu.Lock()
			defer mu.Unlock()
			roster = peers
		},
	}
	runRoom(ctx, t, tr, "self", "self-user", true, cb)

	// A legacy greeting: the field is not there at all.
	legacy := NewHello("old", "old-user", "old", false, nil)
	legacy.Version = 0
	tr.feed("old", legacy)

	waitFor(t, 5*time.Second, "the legacy peer to join", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(roster) == 1
	})

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, ProtocolVersionLegacy, roster[0].Version,
		"the room must know that this peer speaks the old shape")
}

// itoa is a tiny helper so the test does not depend on a formatting choice
// made in the implementation.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
