// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A stream that ends on its own — a crash, a kill, a cut cable — is a
// departure. Before this, readLoop said nothing on EOF, so the room kept the
// peer in every roster forever and a host that vanished that way never
// handed over: the scenario test caught exactly that under load, and
// bug-0106 is the name of it.
func TestADroppedStreamReadsAsADeparture(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// A pipe that ends after one frame: the far side is a net.Pipe partner
	// we close by hand, which is what a severed stream looks like.
	local, remote := net.Pipe()

	m := NewMesh(MeshOptions{Community: "room", Password: "p", Key: "k"})
	m.peers["guest"] = &meshPeer{id: "guest", conn: local, enc: nil}
	go m.readLoop(m.peers["guest"])

	// One real message so the loop is established, then the far end dies.
	// Nothing reads the near end here: readLoop owns it, and a second
	// reader would steal the frame this test is about.
	_, err := remote.Write([]byte(`{"type":"hello","peerId":"guest"}` + "\n"))
	require.NoError(t, err)
	require.NoError(t, remote.Close())

	// The departure the room needs, manufactured on the listening side.
	for {
		select {
		case in := <-m.incoming:
			if messageType(in.msg) == TypeBye && in.from == "guest" {
				return
			}
		case <-time.After(300 * time.Millisecond):
			t.Fatal("the room must learn of the departure")
		case <-ctx.Done():
			t.Fatal("the room never learned of the departure")
		}
	}
}

// The wire shape of a bye is what the room parses, so a manufactured one
// must be indistinguishable from a sent one.
func TestAManufacturedByeLooksLikeASentOne(t *testing.T) {
	raw, err := json.Marshal(NewBye("peer-x"))
	require.NoError(t, err)
	parsed, err := Parse(raw)
	require.NoError(t, err)
	bye, ok := parsed.(Bye)
	require.True(t, ok, "a bye must parse back as a bye, got %T", parsed)
	assert.Equal(t, "peer-x", bye.PeerID)
}

// A room fed by the mesh's own departures must converge on a new host —
// the end-to-end shape of the fix: readLoop manufactures the bye, Room's
// existing onBye does the takeover. Nothing new in Room.
func TestARoomConvergesWhenTheStreamDropsSilently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	var hosts []string
	cb := &Callbacks{
		OnHostChanged: func(id string) {
			mu.Lock()
			defer mu.Unlock()
			hosts = append(hosts, id)
		},
	}

	// peer-b is the smallest once the host leaves.
	tr := newFakeTransport("peer-b", "peer-c")
	runRoom(ctx, t, tr, "peer-b", "user-b", false, cb)

	tr.feed("host-id", NewHello("host-id", "host-user", "", true, nil))
	tr.feed("peer-c", NewHello("peer-c", "user-c", "", false, nil))
	// No bye from the host — exactly the silent-drop case.
	tr.feed("host-id", NewBye("host-id"))

	deadline := time.Now().Add(3 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		mu.Lock()
		if len(hosts) > 0 {
			got = hosts[len(hosts)-1]
		}
		mu.Unlock()
		if got != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, hosts, "no takeover happened after the host left")
	assert.Equal(t, "peer-b", hosts[len(hosts)-1])
}
