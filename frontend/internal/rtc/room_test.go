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
	dialed   []PeerAddr
	// sent keeps what the room sent, so a test can ask what the room said
	// and not only what it heard.
	sent []any
	// invitedHost is the peer ID the invite code named, empty unless a test
	// sets it. The room asks the transport because only the transport saw the
	// invite code (bug-0148).
	invitedHost string
}

func newFakeTransport(self string, peers ...string) *fakeTransport {
	return &fakeTransport{
		self:     self,
		peers:    peers,
		incoming: make(chan incomingMessage, 64),
	}
}

func (f *fakeTransport) Send(_ context.Context, msg any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	return nil
}

// sentMessages returns what the room sent, oldest first.
func (f *fakeTransport) sentMessages() []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]any(nil), f.sent...)
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

// DialHint records where the room was told to reach. The transport is a fake
// and cannot dial, but what it was asked to dial is the point of the room's
// address book.
func (f *fakeTransport) DialHint(peers []PeerAddr) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dialed = append(f.dialed, peers...)
}

func (f *fakeTransport) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeTransport) feed(from string, msg any) {
	f.incoming <- incomingMessage{from: from, msg: msg}
}

// InvitedHost is the peer the code named. A test that leaves it empty is a room
// that knows no invite — the host's own room, and the fallback the room keeps
// for its own transports (bug-0148).
func (f *fakeTransport) InvitedHost() string { return f.invitedHost }

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

	tr.feed("host-id", NewHello("host-id", "host-user", "Host", true, nil))
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

	tr.feed("host-id", NewHello("host-id", "host-user", "", true, nil))
	tr.feed("stranger-id", NewHello("stranger-id", "stranger-user", "", false, nil))
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

	tr.feed("host-id", NewHello("host-id", "host-user", "", true, nil))
	tr.feed("peer-c", NewHello("peer-c", "user-c", "", false, nil))
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

	tr.feed("host-id", NewHello("host-id", "host-user", "", true, nil))
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

// TestTheFollowerGoesWhereTheHostIs pins where the numbers come from: the host
// sends raw positions, and the follower goes where the host is, at once.
//
// There used to be a smoothing here — a third of the gap per heartbeat — and
// it bought a soft landing at the price of the goal's promise: a follower a
// minute behind crawled back for a minute and a half while everyone else
// watched on. The measurement found it, and the promise won (idea-0146).
func TestTheFollowerGoesWhereTheHostIs(t *testing.T) {
	if got := FollowPosition(101); got != 101 {
		t.Errorf("the follower went to %v, want the host's position", got)
	}
	if got := FollowPosition(200); got != 200 {
		t.Errorf("a jump of a minute smoothed to %v, want it taken whole", got)
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
		// The code carries the host's addresses, so a code that does not
		// parse back would be a code a guest cannot use.
		invite.Addrs = []string{p2pAddr(t, "/ip4/127.0.0.1/tcp/4001")}
		parsed, err := ParseInviteCode(invite.Encode())
		if err != nil {
			t.Fatalf("a generated code does not parse back: %v", err)
		}
		if parsed.Community != invite.Community || parsed.Password != invite.Password || parsed.Key != invite.Key {
			t.Fatalf("a code round-tripped into something else: %+v", parsed)
		}
		if len(parsed.Addrs) != 1 || parsed.Addrs[0] != invite.Addrs[0] {
			t.Fatalf("the address came back changed: %+v", parsed.Addrs)
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

// TestAPeerCannotTakeTheRoomByClaimingTheHostRole is the first P0: the greeting
// carries isHost, and believing it let any admitted guest make the room follow
// its commands by saying it held the role (bug-0148). The invite names the
// host's peer ID, so only that peer's claim is believable.
//
// The old host has the mirror-image half of the same defect: after a transfer
// its greeting still said "host", so a newcomer believed the wrong peer. That
// half is closed where the role moves — the window now tells the mesh
// (Mesh.SetHostRole).
func TestAPeerCannotTakeTheRoomByClaimingTheHostRole(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := newFakeTransport("guest-id")
	tr.invitedHost = "host-id"
	r := runRoom(ctx, t, tr, "guest-id", "guest-user", false, nil)

	tr.feed("intruder-id", NewHello("intruder-id", "intruder-user", "Mallory", true, nil))
	waitFor(t, 3*time.Second, "the intruder's greeting to be processed", func() bool {
		for _, p := range r.Peers() {
			if p.PeerID == "intruder-id" {
				return true
			}
		}
		return false
	})
	if got := r.HostPeer(); got != "" {
		t.Fatalf("a peer became the host by claiming the role: host is %q", got)
	}
	for _, p := range r.Peers() {
		if p.PeerID == "intruder-id" && p.IsHost {
			t.Fatalf("the roster marks the peer that claimed the role as the host")
		}
	}

	// The peer the code named is believed — and it is the only one.
	tr.feed("host-id", NewHello("host-id", "host-user", "Host", true, nil))
	waitFor(t, 3*time.Second, "the invited host to be believed", func() bool {
		return r.HostPeer() == "host-id"
	})
	if r.IsHost() {
		t.Fatalf("a guest became the host of the room by hearing a greeting")
	}
}

// TestAMagnetFromAStrangerDoesNotReplaceTheFilm is the second P0: the room
// remembered every magnet it heard, and the host then answered newcomers with
// the stranger's film while believing it was its own (bug-0149).
func TestAMagnetFromAStrangerDoesNotReplaceTheFilm(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	var adopted []Magnet
	cb := &Callbacks{
		OnMagnet: func(m Magnet) {
			mu.Lock()
			defer mu.Unlock()
			adopted = append(adopted, m)
		},
	}

	tr := newFakeTransport("host-id")
	r := runRoom(ctx, t, tr, "host-id", "host-user", true, cb)
	if err := r.AnnounceMagnet(ctx, Magnet{MagnetURI: "magnet:?xt=urn:btih:aaa", InfoHash: "aaa", Name: "Film.mkv"}); err != nil {
		t.Fatalf("announcing the room's film: %v", err)
	}

	tr.feed("stranger-id", NewHello("stranger-id", "stranger-user", "", false, nil))
	tr.feed("stranger-id", NewMagnet(Magnet{MagnetURI: "magnet:?xt=urn:btih:bbb", InfoHash: "bbb", Name: "Other.mkv"}))
	waitFor(t, 3*time.Second, "the stranger's film to be read and ignored", func() bool {
		for _, p := range r.Peers() {
			if p.PeerID == "stranger-id" {
				return true
			}
		}
		return false
	})

	// A newcomer arrives, and the host answers with the film it remembers.
	tr.feed("late-id", NewHello("late-id", "late-user", "", false, nil))
	waitFor(t, 3*time.Second, "the host to answer the newcomer with the film", func() bool {
		last := ""
		for _, msg := range tr.sentMessages() {
			if m, ok := msg.(Magnet); ok {
				last = m.InfoHash
			}
		}
		return last == "aaa"
	})

	for _, msg := range tr.sentMessages() {
		if m, ok := msg.(Magnet); ok && m.InfoHash == "bbb" {
			t.Fatalf("the room carried a stranger's film: %+v", m)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for _, m := range adopted {
		if m.InfoHash == "bbb" {
			t.Fatalf("the host adopted a stranger's film: %+v", m)
		}
	}
}
