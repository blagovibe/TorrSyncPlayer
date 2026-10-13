// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package p2p

import (
	"context"
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

// The roster (idea-0017): who is in the room, who is buffering, who is the
// host. Delivered twice on purpose — as a snapshot in /rooms/info for a client
// that just connected, and as a peer_list event after any change for everyone
// else.

func rosterOf(t *testing.T, svc *Service, userID string) []models.PeerInfo {
	t.Helper()
	info, err := svc.GetRoomInfo(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetRoomInfo: %v", err)
	}
	return info.Peers
}

func peerListData(t *testing.T, svc *Service, userID string) models.PeerListEvent {
	t.Helper()
	ev := awaitEvent(t, svc.GetEvents(userID), models.EventTypePeerList)
	data, ok := ev.Data.(models.PeerListEvent)
	if !ok {
		t.Fatalf("peer_list payload is %T, want models.PeerListEvent", ev.Data)
	}
	return data
}

func findPeer(roster []models.PeerInfo, userID string) (models.PeerInfo, bool) {
	for _, p := range roster {
		if p.UserID == userID {
			return p, true
		}
	}
	return models.PeerInfo{}, false
}

func TestRoomInfoCarriesRosterIncludingHost(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Roster Room", "", "torrent-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-host-1"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	info, err := svc.GetRoomInfo(ctx, "host-1")
	if err != nil {
		t.Fatalf("GetRoomInfo: %v", err)
	}
	if len(info.Peers) != 2 {
		t.Fatalf("roster has %d entries, want 2", len(info.Peers))
	}

	host, ok := findPeer(info.Peers, "host-1")
	if !ok {
		t.Fatal("the host is missing from the roster")
	}
	if !host.IsHost {
		t.Error("the host is not marked isHost")
	}
	if host.PeerID == "" {
		t.Error("the host entry has no peerID, so a client cannot address it")
	}

	guest, ok := findPeer(info.Peers, "guest-1")
	if !ok {
		t.Fatal("the guest is missing from the roster")
	}
	if guest.IsHost {
		t.Error("a guest is marked isHost")
	}
	if guest.PeerID == "" || guest.PeerID == host.PeerID {
		t.Errorf("guest peerID = %q, want a distinct non-empty id", guest.PeerID)
	}

	// The count and the list must agree, or one of them is lying again (bug-0021).
	if info.PeerCount != len(info.Peers) {
		t.Errorf("peerCount = %d but the roster has %d entries", info.PeerCount, len(info.Peers))
	}
}

// A client that joined late never saw peer_joined for whoever was already there.
// This is the invariant that /rooms/info has to hold up.
func TestLateJoinerSeesExistingPeersInRoomInfo(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Late Room", "", "torrent-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "early-1", room.ID, "", "torrent-host-1"); err != nil {
		t.Fatalf("JoinRoom early: %v", err)
	}

	// The late client joins without having observed any event at all.
	late, err := svc.GetRoomInfo(ctx, "host-1")
	if err != nil {
		t.Fatalf("GetRoomInfo: %v", err)
	}
	if _, ok := findPeer(late.Peers, "early-1"); !ok {
		t.Fatal("the late client does not see the peer that was already in the room")
	}
}

func TestPeerListBroadcastOnJoin(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Join Room", "", "torrent-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-host-1"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// The host is watching its own stream and must be told about the newcomer.
	data := peerListData(t, svc, "host-1")
	if len(data.Peers) != 2 {
		t.Errorf("peer_list on join has %d entries, want 2", len(data.Peers))
	}
	if _, ok := findPeer(data.Peers, "guest-1"); !ok {
		t.Error("peer_list on join does not mention the peer that joined")
	}
}

func TestPeerListBroadcastOnLeave(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Leave Room", "", "torrent-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-host-1"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	// Drain the join broadcast so the next one read is the leave.
	awaitEvent(t, svc.GetEvents("host-1"), models.EventTypePeerList)

	if err := svc.LeaveRoom(ctx, "guest-1"); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}

	data := peerListData(t, svc, "host-1")
	if _, ok := findPeer(data.Peers, "guest-1"); ok {
		t.Error("peer_list on leave still lists the peer that left")
	}
	if len(data.Peers) != 1 {
		t.Errorf("peer_list on leave has %d entries, want 1 (the host)", len(data.Peers))
	}
}

// The roster has to name WHO is buffering. The room's SyncStatus is a single
// shared value and cannot say that, which is why Buffering lives on the peer.
func TestPeerListNamesTheBufferingPeer(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Buffer Room", "", "torrent-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-host-1"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	// Drain the broadcast caused by the join: awaitEvent returns the first
	// peer_list it sees, and reading the stale one would prove nothing.
	awaitEvent(t, svc.GetEvents("host-1"), models.EventTypePeerList)

	svc.SetPeerBuffering(ctx, "guest-1", true)
	svc.BroadcastPeerList(room.ID)

	data := peerListData(t, svc, "host-1")
	guest, ok := findPeer(data.Peers, "guest-1")
	if !ok {
		t.Fatal("the guest is missing from the roster")
	}
	if !guest.Buffering {
		t.Error("the guest reported buffering but the roster does not say so")
	}
	if host, ok := findPeer(data.Peers, "host-1"); !ok || host.Buffering {
		t.Error("the host did not report buffering but is marked as buffering")
	}

	// And catching up clears it — the message that lifts a stalled room.
	svc.SetPeerBuffering(ctx, "guest-1", false)
	svc.BroadcastPeerList(room.ID)

	data = peerListData(t, svc, "host-1")
	if guest, ok = findPeer(data.Peers, "guest-1"); !ok || guest.Buffering {
		t.Error("the guest caught up but the roster still says it is buffering")
	}
}

// SetPeerBuffering must not invent membership for a user who is in no room.
func TestSetPeerBufferingOutsideRoomIsNoOp(t *testing.T) {
	svc := newTestService(t)

	svc.SetPeerBuffering(context.Background(), "nobody", true)

	if _, err := svc.GetRoomInfo(context.Background(), "nobody"); err == nil {
		t.Error("SetPeerBuffering must not make an outsider a member of a room")
	}
}

// The roster is a full replacement, so a client that applies it twice — which is
// what happens when a peer_list lands on top of the post-reconnect reconciliation
// (bug-0012) — ends up in the same state. Idempotence is the property that makes
// the two mechanisms safe to combine.
func TestPeerListIsAStableFullSnapshot(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Stable Room", "", "torrent-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, g := range []string{"guest-1", "guest-2", "guest-3"} {
		if err := svc.JoinRoom(ctx, g, room.ID, "", "torrent-host-1"); err != nil {
			t.Fatalf("JoinRoom %s: %v", g, err)
		}
	}

	first := rosterOf(t, svc, "host-1")
	second := rosterOf(t, svc, "host-1")

	if len(first) != len(second) {
		t.Fatalf("roster size changed between two reads: %d then %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("entry %d differs between reads:\n  %+v\n  %+v", i, first[i], second[i])
		}
	}

	// A roster the server produces must not depend on map iteration order, or
	// every broadcast would look different even when nothing changed.
	for i := 0; i < 5; i++ {
		got := rosterOf(t, svc, "host-1")
		for j := range got {
			if got[j] != second[j] {
				t.Fatalf("roster order is not stable: %+v vs %+v", got, second)
			}
		}
	}
}

// The roster version has to rise on every change, in both delivery channels, so
// a client can tell which of two snapshots is older. It matters because
// /rooms/info is answered at request time while peer_list is pushed as things
// happen, so the two genuinely race.
func TestRosterSeqRisesWithEveryChange(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Seq Room", "", "torrent-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	seqOf := func(userID string) int64 {
		info, err := svc.GetRoomInfo(ctx, userID)
		if err != nil {
			t.Fatalf("GetRoomInfo: %v", err)
		}
		return info.RosterSeq
	}
	eventSeq := func(userID string) int64 {
		return peerListData(t, svc, userID).RosterSeq
	}

	previous := seqOf("host-1")
	if previous == 0 {
		t.Fatal("a new room starts at roster version 0, which reads as 'unknown'")
	}

	steps := []struct {
		name string
		run  func()
	}{
		{"guest joins", func() {
			if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-host-1"); err != nil {
				t.Fatalf("JoinRoom: %v", err)
			}
		}},
		{"guest reports buffering", func() {
			svc.SetPeerBuffering(ctx, "guest-1", true)
			svc.BroadcastPeerList(room.ID)
		}},
		{"guest catches up", func() {
			svc.SetPeerBuffering(ctx, "guest-1", false)
			svc.BroadcastPeerList(room.ID)
		}},
		{"guest leaves", func() {
			if err := svc.LeaveRoom(ctx, "guest-1"); err != nil {
				t.Fatalf("LeaveRoom: %v", err)
			}
		}},
	}

	for _, step := range steps {
		step.run()

		now := seqOf("host-1")
		if now <= previous {
			t.Errorf("after %q the roster version did not rise: %d then %d",
				step.name, previous, now)
		}
		previous = now

		// The event must carry the same version the endpoint reports, or a
		// client comparing the two would discard the wrong one.
		if seq := eventSeq("host-1"); seq != now {
			t.Errorf("after %q peer_list reports version %d but /rooms/info says %d",
				step.name, seq, now)
		}
	}
}

// Reporting the same state twice must not invent a new version: the roster did
// not change, and a client would otherwise keep discarding snapshots.
func TestRosterSeqStableWhenNothingChanged(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Stable Seq Room", "", "torrent-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	before, err := svc.GetRoomInfo(ctx, "host-1")
	if err != nil {
		t.Fatalf("GetRoomInfo: %v", err)
	}

	// Same value it already has.
	svc.SetPeerBuffering(ctx, "host-1", false)
	svc.BroadcastPeerList(room.ID)

	after, err := svc.GetRoomInfo(ctx, "host-1")
	if err != nil {
		t.Fatalf("GetRoomInfo: %v", err)
	}
	if after.RosterSeq != before.RosterSeq {
		t.Errorf("roster version moved without a change: %d then %d",
			before.RosterSeq, after.RosterSeq)
	}
}
