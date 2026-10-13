// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/models"
)

// bug-0021: the host is a participant. It sits in the SSE recipient list, it
// passes the membership check, it drives playback — but it was not in room.Peers,
// so every peer count was one short. Adding it to room.Peers changes when a room
// is considered empty, and these tests pin each combination.
//
// The real client flow is modelled throughout: the host creates the room and
// subscribes to SSE but never calls join.

func peerCountOf(t *testing.T, svc *Service, roomID string) int {
	t.Helper()
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	room, ok := svc.rooms[roomID]
	if !ok {
		t.Fatalf("room %s does not exist", roomID)
	}
	return len(room.Peers)
}

func roomExists(svc *Service, roomID string) bool {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	_, ok := svc.rooms[roomID]
	return ok
}

func TestPeerCountIncludesHost(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Count Room", "", "torrent-test-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// The host alone is one participant. Reported 0 before.
	if got := peerCountOf(t, svc, room.ID); got != 1 {
		t.Errorf("peerCount after create = %d, want 1 (the host is a participant)", got)
	}

	info, err := svc.GetRoomInfo(ctx, "host-1")
	if err != nil {
		t.Fatalf("GetRoomInfo: %v", err)
	}
	if info.PeerCount != 1 {
		t.Errorf("RoomInfo.PeerCount = %d, want 1", info.PeerCount)
	}

	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-test-host-1"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if got := peerCountOf(t, svc, room.ID); got != 2 {
		t.Errorf("peerCount after guest joins = %d, want 2", got)
	}
	info, err = svc.GetRoomInfo(ctx, "host-1")
	if err != nil {
		t.Fatalf("GetRoomInfo: %v", err)
	}
	if info.PeerCount != 2 {
		t.Errorf("RoomInfo.PeerCount = %d, want 2", info.PeerCount)
	}
}

// The host leaves first while the guest stays: the room survives with the guest
// in it. This is a pre-existing behaviour, not a fix — recorded so the change
// to peer counting does not silently alter it.
func TestRoomSurvivesWhenHostLeavesAndGuestRemains(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Order Room", "", "torrent-test-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-test-host-1"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	if err := svc.LeaveRoom(ctx, "host-1"); err != nil {
		t.Fatalf("LeaveRoom host: %v", err)
	}

	if !roomExists(svc, room.ID) {
		t.Fatal("room was deleted although a guest is still in it")
	}
	if got := peerCountOf(t, svc, room.ID); got != 1 {
		t.Errorf("peerCount = %d, want 1", got)
	}
}

// The reverse order is the case that was broken. With the host outside
// room.Peers, the last guest leaving emptied Peers and deleted the room —
// while the host was still connected, still watching, still driving playback.
// The host's own SSE stream then pointed at a room that no longer existed.
func TestRoomSurvivesWhenLastGuestLeavesAndHostRemains(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Order Room", "", "torrent-test-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-test-host-1"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	if err := svc.LeaveRoom(ctx, "guest-1"); err != nil {
		t.Fatalf("LeaveRoom guest: %v", err)
	}

	if !roomExists(svc, room.ID) {
		t.Fatal("room was deleted although the host is still in it and connected")
	}
	if got := peerCountOf(t, svc, room.ID); got != 1 {
		t.Errorf("peerCount = %d, want 1 (the host)", got)
	}

	// And the host must still be recognised as a member.
	if _, err := svc.GetRoomInfo(ctx, "host-1"); err != nil {
		t.Errorf("host lost membership when the last guest left: %v", err)
	}
}

// Everyone gone: the room is deleted, so it cannot accumulate forever.
func TestRoomDeletedWhenEveryoneLeaves(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Order Room", "", "torrent-test-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-test-host-1"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	if err := svc.LeaveRoom(ctx, "guest-1"); err != nil {
		t.Fatalf("LeaveRoom guest: %v", err)
	}
	if !roomExists(svc, room.ID) {
		t.Error("room should still exist with the host in it")
	}

	if err := svc.LeaveRoom(ctx, "host-1"); err != nil {
		t.Fatalf("LeaveRoom host: %v", err)
	}
	if roomExists(svc, room.ID) {
		t.Error("room must be deleted once the host leaves too")
	}
}

// Two guests leave in turn: the count must step down and only then go.
func TestPeerCountStepsDownAsGuestsLeave(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Countdown Room", "", "torrent-test-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, g := range []string{"guest-1", "guest-2", "guest-3"} {
		if err := svc.JoinRoom(ctx, g, room.ID, "", "torrent-test-host-1"); err != nil {
			t.Fatalf("JoinRoom %s: %v", g, err)
		}
	}
	if got := peerCountOf(t, svc, room.ID); got != 4 {
		t.Fatalf("peerCount = %d, want 4", got)
	}

	for i, want := range []int{3, 2, 1} {
		guest := []string{"guest-1", "guest-2", "guest-3"}[i]
		if err := svc.LeaveRoom(ctx, guest); err != nil {
			t.Fatalf("LeaveRoom %s: %v", guest, err)
		}
		if got := peerCountOf(t, svc, room.ID); got != want {
			t.Errorf("peerCount after %s left = %d, want %d", guest, got, want)
		}
		if !roomExists(svc, room.ID) {
			t.Fatalf("room deleted too early, after %s left", guest)
		}
	}
}

// The idle pruner must not reap a peer that is demonstrably connected. Before
// this, LastHeartbeat was only written by JoinRoom, so every peer — host
// included — was reaped five minutes in while its SSE stream was still open for
// thirty. The heartbeat now follows the SSE stream.
//
// The model is the real one: pings arrive every 30 s, so a live peer's last
// ping is recent. Ageing the heartbeat by the ping interval and then pruning is
// exactly what happens one interval after a ping on a healthy stream.
func TestPruneKeepsPeerWithLiveSSEStream(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Live Room", "", "torrent-test-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// A ping landed one ping-interval ago.
	if err := svc.TouchPeer(ctx, "host-1"); err != nil {
		t.Fatalf("TouchPeer: %v", err)
	}
	ageAllPeers(t, svc, -constants.SSEPingInterval)

	svc.pruneIdlePeers()

	if !roomExists(svc, room.ID) {
		t.Fatal("room was reaped even though the host is connected")
	}
	if got := peerCountOf(t, svc, room.ID); got != 1 {
		t.Errorf("peerCount = %d, want 1: a connected peer was reaped as idle", got)
	}
}

// A peer whose stream died stops being pinged and must still be reaped — the
// timeout has to keep meaning something.
func TestPruneStillReapsSilentPeer(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	room, err := svc.CreateRoom(ctx, "host-1", "Quiet Room", "", "torrent-test-host-1")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := svc.JoinRoom(ctx, "guest-1", room.ID, "", "torrent-test-host-1"); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// Host keeps pinging; the guest's stream is gone.
	if err := svc.TouchPeer(ctx, "host-1"); err != nil {
		t.Fatalf("TouchPeer: %v", err)
	}
	ageAllPeers(t, svc, -constants.SSEPingInterval)
	agePeer(t, svc, room.ID, "guest-1", -2*constants.PeerIdleTimeout)

	svc.pruneIdlePeers()

	if got := peerCountOf(t, svc, room.ID); got != 1 {
		t.Errorf("peerCount = %d, want 1: the silent peer should have been reaped", got)
	}
}

// TouchPeer on someone who is not in a room is a no-op, not a crash and not a
// spurious membership grant.
func TestTouchPeerOutsideRoomIsNoOp(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if err := svc.TouchPeer(ctx, "nobody"); err == nil {
		t.Log("TouchPeer returned nil for a user with no room; tolerated")
	}
	if _, err := svc.GetRoomInfo(ctx, "nobody"); err == nil {
		t.Error("TouchPeer must not make an outsider a member")
	}
}

// ageAllPeers backdates every peer's heartbeat in every room.
func ageAllPeers(t *testing.T, svc *Service, delta time.Duration) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, room := range svc.rooms {
		for _, p := range room.Peers {
			p.LastHeartbeat = p.LastHeartbeat.Add(delta)
		}
	}
}

// agePeer backdates one peer's heartbeat.
func agePeer(t *testing.T, svc *Service, roomID, userID string, delta time.Duration) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	room, ok := svc.rooms[roomID]
	if !ok {
		t.Fatalf("room %s missing", roomID)
	}
	for _, p := range room.Peers {
		if p.UserID == userID {
			p.LastHeartbeat = p.LastHeartbeat.Add(delta)
			return
		}
	}
	t.Fatalf("peer %s not found in room", userID)
}

var _ = models.SyncActionPlay
