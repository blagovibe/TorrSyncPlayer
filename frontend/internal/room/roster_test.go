// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package room

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func peers(ids ...string) []Peer {
	out := make([]Peer, 0, len(ids))
	for i, id := range ids {
		out = append(out, Peer{PeerID: id, UserID: id, IsHost: i == 0})
	}
	return out
}

// The roster arrives from two directions that race: the answer to /rooms/info
// and the events on the stream. A snapshot that overtook a newer event must be
// discarded, or it silently deletes whoever joined in between.
func TestStaleSnapshotDoesNotWipeANewcomer(t *testing.T) {
	var r Roster

	// A guest joined while our GET was in flight; the event lands first.
	require.True(t, r.Apply(peers("host"), 1), "the first snapshot must be applied")
	require.True(t, r.Apply(peers("host", "guest"), 2), "a newer event must be applied")

	// Now the older GET finally answers, still describing one member.
	require.False(t, r.Apply(peers("host"), 1), "a stale snapshot was applied and the newcomer lost")

	assert.Equal(t, []string{"host", "guest"}, ids(r.Peers()))
}

func TestSnapshotReplacesRatherThanMerges(t *testing.T) {
	// Applying the same list twice must change nothing, which is what lets a
	// peer_list arriving on top of a reconnect be applied safely.
	var r Roster

	require.True(t, r.Apply(peers("host", "guest"), 1))
	require.True(t, r.Apply(peers("host", "guest", "newcomer"), 2))
	require.True(t, r.Apply(peers("host", "guest", "newcomer"), 3),
		"an equal sequence number with new content should still be applied")

	assert.Equal(t, []string{"host", "guest", "newcomer"}, ids(r.Peers()))
	assert.Equal(t, 3, r.Count())
}

func TestResetLetsALowerSequenceBeAppliedAgain(t *testing.T) {
	// Leaving and rejoining is a new room as far as this client is concerned.
	var r Roster

	require.True(t, r.Apply(peers("host", "guest", "newcomer"), 7))

	r.Reset()
	assert.Equal(t, 0, r.Count())

	require.True(t, r.Apply(peers("host"), 1))
	assert.Equal(t, []string{"host"}, ids(r.Peers()))
}

func TestTheHostIsAnOrdinaryRowWithAFlag(t *testing.T) {
	// There is no separate list of hosts, so nothing can forget to check the
	// flag and treat the host as somebody else.
	var r Roster
	require.True(t, r.Apply(peers("host", "guest"), 1))

	host, ok := r.FindHost()
	require.True(t, ok)
	assert.Equal(t, "host", host.UserID)
	assert.True(t, host.IsHost)
	assert.Equal(t, 2, r.Count(), "the host counts as a member")
}

func TestBufferingIsTrackedPerPeer(t *testing.T) {
	// The room has one shared playback state, so a roster is the only place
	// that can say WHICH peer is stalled.
	var r Roster
	require.True(t, r.Apply(peers("host", "guest"), 1))

	r.SetBuffering("guest", true)

	got := r.Peers()
	assert.True(t, got[1].Buffering)
	assert.False(t, got[0].Buffering, "buffering state leaked to another peer")

	r.SetBuffering("guest", false)
	assert.False(t, r.Peers()[1].Buffering)
}

func TestPeersIsACopy(t *testing.T) {
	// A caller that mutates the returned slice would corrupt the roster from
	// outside its lock.
	var r Roster
	require.True(t, r.Apply(peers("host", "guest"), 1))

	snapshot := r.Peers()
	snapshot[0].UserID = "tampered"

	assert.Equal(t, "host", r.Peers()[0].UserID)
}

func TestDecodePeerList(t *testing.T) {
	raw := json.RawMessage(`{"peers":[{"peerID":"a","userID":"host","isHost":true,"buffering":false}],"rosterSeq":3}`)

	list, seq, err := DecodePeerList(raw)
	require.NoError(t, err)
	assert.Equal(t, 3, seq)
	require.Len(t, list, 1)
	assert.Equal(t, "host", list[0].UserID)
	assert.True(t, list[0].IsHost)
}

func TestDecodeRoomInfoCarriesTheTorrentBinding(t *testing.T) {
	// The guest needs both halves: who is in the room, and what it is watching.
	raw := json.RawMessage(`{"id":"room","name":"Movie night","torrentId":"abc123",` +
		`"hostUserId":"host","peerCount":2,"rosterSeq":4,` +
		`"peers":[{"peerID":"a","userID":"host","isHost":true},{"peerID":"b","userID":"guest"}]}`)

	torrentID, list, seq, hostUserID, err := DecodeRoomInfo(raw)
	require.NoError(t, err)
	assert.Equal(t, "abc123", torrentID)
	assert.Equal(t, 4, seq)
	assert.Equal(t, "host", hostUserID)
	assert.Len(t, list, 2)
}

func TestDecodeRejectsGarbageRatherThanReturningEmpty(t *testing.T) {
	// An empty roster and a failed parse look identical to a caller that
	// ignores the error, and the difference is "nobody is here" versus
	// "nobody is known".
	_, _, err := DecodePeerList(json.RawMessage(`not json`))
	assert.Error(t, err)

	_, _, _, _, err = DecodeRoomInfo(json.RawMessage(`{`))
	assert.Error(t, err)
}

func ids(list []Peer) []string {
	out := make([]string, 0, len(list))
	for _, p := range list {
		out = append(out, p.UserID)
	}
	return out
}
