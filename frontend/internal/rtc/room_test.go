// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"sync"
	"testing"
	"time"
)

// The room driver is where the mesh becomes a room: roster from hellos, host
// authority, takeover, magnet adoption, and the callbacks the window hangs its
// widgets on. It talks through a Transport — the mesh in production, channels
// here — because rendezvous fails for the weather and logic must not.

// fakeTransport is a mesh with the network taken out: sends land in a slice,
// receives come from a channel the test feeds.
type fakeTransport struct {
	mu       sync.Mutex
	self     string
	peers    []string
	incoming chan incomingMessage
	closed   bool
}

func newFakeTransport(self string, peers ...string) *fakeTransport {
	return &fakeTransport{
		self:     self,
		peers:    peers,
		incoming: make(chan incomingMessage, 64),
	}
}

func (f *fakeTransport) Send(_ context.Context, msg any) error {
	return nil
}

func (f *fakeTransport) Next(ctx context.Context) (string, any, bool) {
	select {
	case in := <-f.incoming:
		return in.from, in.msg, true
	case <-ctx.Done():
		return "", nil, false
	}
}

func (f *fakeTransport) Peers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.peers...)
}

func (f *fakeTransport) SelfID() string { return f.self }

func (f *fakeTransport) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeTransport) feed(from string, msg any) {
	f.incoming <- incomingMessage{from: from, msg: msg}
}

// runRoom starts the driver and returns it. The test owns stopping via ctx.
func runRoom(ctx context.Context, t *testing.T, tr *fakeTransport, selfID, userID string, isHost bool, cb *Callbacks) *Room {
	t.Helper()
	r := NewRoom(tr, selfID, userID, isHost, cb)
	go r.Run(ctx)
	return r
}

// TestAGuestLearnsTheRoomFromHellosAndTheMagnet is the join path: hellos build
// the roster and name the host, the magnet names the film, and the callbacks
// fire in an order the window can act on.
func TestAGuestLearnsTheRoomFromHellosAndTheMagnet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	var roster [][]Peer
	var magnets []Magnet
	cb := &Callbacks{
		OnRoster: func(peers []Peer) {
			mu.Lock()
			defer mu.Unlock()
			roster = append(roster, peers)
		},
		OnMagnet: func(m Magnet) {
			mu.Lock()
			defer mu.Unlock()
			magnets = append(magnets, m)
		},
	}

	tr := newFakeTransport("guest-id")
	runRoom(ctx, t, tr, "guest-id", "guest-user", false, cb)

	tr.feed("host-id", NewHello("host-id", "host-user", "Host", true))
	tr.feed("host-id", NewMagnet(Magnet{MagnetURI: "magnet:?xt=urn:btih:abc", InfoHash: "abc", SelectedFile: 1, Name: "Film.mkv"}))

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := len(magnets) == 1
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(magnets) != 1 {
		t.Fatalf("the guest never adopted the magnet (roster updates: %d)", len(roster))
	}
	if magnets[0].InfoHash != "abc" || magnets[0].SelectedFile != 1 {
		t.Errorf("the guest adopted the wrong film: %+v", magnets[0])
	}
	if len(roster) == 0 {
		t.Fatal("the roster never updated")
	}
	found := false
	for _, p := range roster[len(roster)-1] {
		if p.PeerID == "host-id" && p.IsHost {
			found = true
		}
	}
	if !found {
		t.Errorf("the host is not in the roster as host: %+v", roster[len(roster)-1])
	}
}

// TestCommandsFromStrangersAreDropped pins the authority rule at the driver
// level: a pause from anyone but the host must not reach the player.
func TestCommandsFromStrangersAreDropped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	pauses := 0
	cb := &Callbacks{
		OnPause: func(paused bool) {
			mu.Lock()
			defer mu.Unlock()
			pauses++
		},
	}

	tr := newFakeTransport("guest-id")
	r := runRoom(ctx, t, tr, "guest-id", "guest-user", false, cb)

	tr.feed("host-id", NewHello("host-id", "host-user", "", true))
	tr.feed("stranger-id", NewHello("stranger-id", "stranger-user", "", false))
	tr.feed("stranger-id", NewPause(true))

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	strangerPauses := pauses
	mu.Unlock()
	if strangerPauses != 0 {
		t.Fatalf("a stranger's pause reached the player %d times", strangerPauses)
	}

	_ = r
	tr.feed("host-id", NewPause(true))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := pauses == 1
		mu.Unlock()
		if done {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the host's pause never reached the player")
}

// TestTakeoverElectsTheSmallestLivingPeer is the host-leave rule end to end:
// the host says bye, everyone independently picks the smallest ID, and the
// winner announces itself.
func TestTakeoverElectsTheSmallestLivingPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	var hosts []string
	cb := &Callbacks{
		OnHostChanged: func(hostPeerID string) {
			mu.Lock()
			defer mu.Unlock()
			hosts = append(hosts, hostPeerID)
		},
	}

	// This peer is "peer-b": not the smallest while the host lives, the
	// smallest once it leaves and peer-c is also still around... no: peer-b
	// beats peer-c, loses to nothing here. The winner must be peer-b.
	tr := newFakeTransport("peer-b", "peer-c")
	runRoom(ctx, t, tr, "peer-b", "user-b", false, cb)

	tr.feed("host-id", NewHello("host-id", "host-user", "", true))
	tr.feed("peer-c", NewHello("peer-c", "user-c", "", false))
	tr.feed("host-id", NewBye("host-id"))

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := len(hosts) > 0
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(hosts) == 0 {
		t.Fatal("no takeover happened after the host left")
	}
	if hosts[len(hosts)-1] != "peer-b" {
		t.Errorf("takeover picked %q, want peer-b (smallest living ID)", hosts[len(hosts)-1])
	}
}

// TestTakeoverAnnouncementFromAStrangerIsIgnored is the other half of the rule:
// a Host message is believable only from the mouth of the one giving it away —
// either the current host naming a successor, or the takeover winner naming
// itself. Anyone else naming anyone is noise.
func TestTakeoverAnnouncementFromAStrangerIsIgnored(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	var hosts []string
	cb := &Callbacks{
		OnHostChanged: func(hostPeerID string) {
			mu.Lock()
			defer mu.Unlock()
			hosts = append(hosts, hostPeerID)
		},
	}

	tr := newFakeTransport("peer-b", "peer-c")
	runRoom(ctx, t, tr, "peer-b", "user-b", false, cb)

	tr.feed("host-id", NewHello("host-id", "host-user", "", true))
	// peer-c crowns itself while the host is still alive and peer-c is not the
	// smallest anything: must change nothing. The one announcement the test
	// expects is the legitimate one — learning the actual host from its hello.
	tr.feed("peer-c", NewHost("peer-c"))

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(hosts) != 1 || hosts[0] != "host-id" {
		t.Errorf("host announcements are %v, want exactly the legitimate [host-id]", hosts)
	}
}

// TestSeekSmoothingStaysWithTheFollower pins where the numbers come from: the
// host sends raw positions, and the follower decides where to actually go. A
// small jump lands as asked; a big one moves partway, so a lagging follower
// does not teleport past everyone else.
func TestSeekSmoothingStaysWithTheFollower(t *testing.T) {
	if got := SmoothSeek(100, 101); got != 101 {
		t.Errorf("a 1s jump smoothed to %v, want it as asked", got)
	}
	if got := SmoothSeek(100, 200); got != 100+(200-100)*0.3 {
		t.Errorf("a 100s jump smoothed to %v, want 30%% of the way", got)
	}
	if got := SmoothSeek(100, 102); got != 102 {
		t.Errorf("exactly the 2s boundary smoothed to %v, want it as asked", got)
	}
}

// TestInviteCodeRoundTrips pins the room's address: community, password and key
// in one string the host copies and the guest pastes. Colons separate the parts,
// so none of the parts may contain one — a generated code that does is a code
// that does not parse back.
func TestInviteCodeRoundTrips(t *testing.T) {
	for i := 0; i < 50; i++ {
		invite := GenerateInvite()
		if invite.Community == "" || invite.Password == "" || invite.Key == "" {
			t.Fatalf("a generated invite has an empty part: %+v", invite)
		}
		parsed, err := ParseInviteCode(invite.Encode())
		if err != nil {
			t.Fatalf("a generated code does not parse back: %v", err)
		}
		if parsed.Community != invite.Community || parsed.Password != invite.Password || parsed.Key != invite.Key {
			t.Fatalf("a code round-tripped into something else: %+v", parsed)
		}
	}
}

// TestInviteCodeRefusesGarbage is the pasted-magnet half: a magnet is not an
// invite, two parts are not three, and an empty string is not a room.
func TestInviteCodeRefusesGarbage(t *testing.T) {
	for _, bad := range []string{
		"",
		"magnet:?xt=urn:btih:abc",
		"community:password",
		"a:b:c:d",
		":password:key",
		"community::key",
	} {
		if _, err := ParseInviteCode(bad); err == nil {
			t.Errorf("garbage %q parsed as an invite", bad)
		}
	}
}
