// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
)

// A room of two is a pair, a room of three is a room, and what happens at
// fifty is a question the ADR names as its own condition for review: "rooms
// of a hundred participants needed a server".
//
// Nothing in the project has ever put more than three people in a room, so
// that condition has never been asked of the code. This asks it, and says
// how long the gathering and the fan-out took.
//
// The mesh is quadratic, and that is the whole of it: every pair of
// participants holds a stream, every newcomer is handed the address book of
// everyone before it, and every command travels to everybody. Nothing here is
// a bottleneck until the numbers say so — and the numbers are what make the
// ADR's condition a question with an answer instead of a worry.
//
// The room is measured without the backends: they carry the film and the
// swarm, and neither is part of what a room costs. The film still reaches
// everybody in the scenario with the film in it.

// TestARoomOfManyIsStillARoom measures the room at two sizes: a handful of
// people, and enough of them that the mesh's quadratic shape has something
// to say.
//
// The thresholds are the goal's own words — the room assembles in seconds —
// with room to spare for a loaded runner. What the mesh costs at a hundred
// participants is measured separately and written down in
// measurements/scale.md: a hundred of them is too many for every run of every
// job, and the point of the number is that it was taken once, carefully.
func TestARoomOfManyIsStillARoom(t *testing.T) {
	for _, participants := range []int{5, 25} {
		gathered, fanned := measureRoomOf(t, participants)

		// The gathering is a promise with a number in it: the size of the
		// room must not be what decides how long the room takes to form.
		assert.Less(t, gathered, 30*time.Second,
			"a room of %d participants took %s to gather", participants, gathered)
		assert.Less(t, fanned, 10*time.Second,
			"the host's command took %s to reach a room of %d", fanned, participants)
	}
}

// TestTheRoomsCostIsQuadratic is a statement about the design, not a
// measurement: the mesh holds a stream per pair, and the address book is
// handed to every newcomer whole.
//
// It is here because the cost is easy to forget and expensive to discover at
// a hundred participants: the ADR names "rooms of a hundred participants
// needed a server" as its condition for review, and the condition is about
// this shape, not about a number someone guessed.
func TestTheRoomsCostIsQuadratic(t *testing.T) {
	// A pair holds one stream, three hold three, ten hold forty-five.
	for _, participants := range []int{2, 3, 10, 100} {
		streams := participants * (participants - 1) / 2
		t.Logf("%d participants: %d streams, and every command travels along all of them",
			participants, streams)
	}
}

// measureRoomOf puts participants in one room and says what it cost: how long
// the room gathered, and how long the host's command took to reach everybody.
//
// The room is built the way the product builds it: the guests come by the
// address the code carried, and the address book is what carries a newcomer
// to the others.
func measureRoomOf(t *testing.T, participants int) (gathered, fanned time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	inv := rtc.Invite{
		Community: fmt.Sprintf("scale-%d", participants),
		Password:  "scale-password",
		Key:       "scale-key",
	}
	bootstrap := startTestbed(t, inv)

	// The host, and the room it leads.
	hostMesh := rtc.NewMesh(rtc.MeshOptions{
		Community: inv.Community,
		Password:  inv.Password,
		Key:       inv.Key,
		Bootstrap: bootstrap,
		UserID:    "host-peer",
	})
	defer func() { _ = hostMesh.Close() }()
	require.NoError(t, hostMesh.Join(ctx, true), "the host could not join")
	hostRoom := rtc.NewRoom(hostMesh, hostMesh.SelfID(), "host-peer", true, (&roomEvents{}).callbacks())
	go hostRoom.Run(ctx)

	// The guests, one after another, the way people arrive.
	type guest struct {
		mesh   *rtc.Mesh
		events *roomEvents
	}
	var guests []guest
	for i := 0; i < participants-1; i++ {
		name := fmt.Sprintf("guest-%d", i)
		events := &roomEvents{}
		mesh := rtc.NewMesh(rtc.MeshOptions{
			Community: inv.Community,
			Password:  inv.Password,
			Key:       inv.Key,
			Bootstrap: bootstrap,
			UserID:    name,
			// The address the code carried: the room is met by a direct
			// dial, and the address book is what carries the newcomer to
			// the others.
			Addrs: hostMesh.Addrs(),
		})
		require.NoError(t, mesh.Join(ctx, false), "%s could not join", name)
		defer func() { _ = mesh.Close() }()
		room := rtc.NewRoom(mesh, mesh.SelfID(), name, false, events.callbacks())
		go room.Run(ctx)
		guests = append(guests, guest{mesh: mesh, events: events})
	}

	// The room gathers: every participant sees every other one. That is the
	// mesh's own claim, and it is the claim that costs.
	started := time.Now()
	for _, g := range guests {
		waitForMesh(t, g.mesh, participants-1, 120*time.Second,
			fmt.Sprintf("a guest never saw the whole room (saw %d peers)", len(g.mesh.Peers())))
	}
	waitForMesh(t, hostMesh, participants-1, 120*time.Second, "the host never saw the whole room")
	gathered = time.Since(started)

	// And the room knows the same: every roster names everybody.
	for _, g := range guests {
		waitForGuestValue(t, func() bool { return g.events.rosterSize() >= participants-1 },
			"a guest's roster never named the whole room")
	}

	// The host's command reaches everybody — the fan-out is the other half
	// of the quadratic, and the half a person feels.
	paused := time.Now()
	require.NoError(t, hostRoom.Pause(ctx))
	for _, g := range guests {
		waitForGuestValue(t, func() bool { return g.events.pauseCount() == 1 },
			"the host's pause never reached a guest")
	}
	fanned = time.Since(paused)

	t.Logf("a room of %d participants: gathered in %s, a command reached everyone in %s, %d streams",
		participants, gathered.Round(time.Millisecond), fanned.Round(time.Millisecond),
		participants*(participants-1)/2)

	return gathered, fanned
}
