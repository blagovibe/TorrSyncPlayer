// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package room

import (
	"encoding/json"
	"sync"
)

// Roster is the membership of the room as this client believes it.
//
// Two things make it more than a slice:
//
//   - Snapshots are versioned. `/rooms/info` is an asynchronous GET, so it is a
//     snapshot taken when the server served it, while `peer_list` events arrive
//     as they happen. A guest who joined after the GET was issued can be
//     delivered by the event stream BEFORE the GET's answer lands — and a naive
//     apply would then wipe that guest out of the list. rosterSeq is what lets a
//     stale snapshot be discarded instead.
//
//   - Snapshots replace, they do not merge. Applying the same list twice
//     changes nothing, which is what lets a `peer_list` arriving on top of a
//     reconnect be applied without doubling anyone up.
type Roster struct {
	mu      sync.RWMutex
	peers   []Peer
	seq     int
	applied bool
}

// Apply installs a snapshot, unless it is older than what is already held.
//
// Returns whether the snapshot was applied. A false return is not an error: it
// is the expected outcome of a response that overtook a newer event.
func (r *Roster) Apply(peers []Peer, seq int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.applied && seq <= r.seq {
		return false
	}

	r.peers = peers
	r.seq = seq
	r.applied = true
	return true
}

// Peers returns the current membership.
func (r *Roster) Peers() []Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Peer, len(r.peers))
	copy(out, r.peers)
	return out
}

// Count is how many participants, including the host.
func (r *Roster) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.peers)
}

// FindHost returns the host's peer entry, if the roster names one.
//
// The host is an ordinary row with a flag, exactly as every other member is.
// There is no separate list of hosts and no special casing for one.
func (r *Roster) FindHost() (Peer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, peer := range r.peers {
		if peer.IsHost {
			return peer, true
		}
	}
	return Peer{}, false
}

// SetBuffering records that a peer reported itself buffering or caught up.
//
// It is a local correction, not a replacement: the next roster snapshot is
// authoritative and will say whatever the server last believed. Keeping it
// local means a buffering state never waits for a round trip.
func (r *Roster) SetBuffering(peerID string, buffering bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.peers {
		if r.peers[i].PeerID == peerID {
			r.peers[i].Buffering = buffering
		}
	}
}

// Reset forgets everything, so a later snapshot is applied even if its number
// is lower than the one held before.
func (r *Roster) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.peers = nil
	r.seq = 0
	r.applied = false
}

// DecodePeerList reads a peer_list payload.
func DecodePeerList(data json.RawMessage) ([]Peer, int, error) {
	var payload struct {
		Peers     []Peer `json:"peers"`
		RosterSeq int    `json:"rosterSeq"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, 0, err
	}
	return payload.Peers, payload.RosterSeq, nil
}

// DecodeRoomInfo reads the torrent id and roster from a /rooms/info response.
//
// It carries both on purpose: a client that joined late needs the roster (it
// never saw peer_joined for anyone already there) and the torrent binding.
func DecodeRoomInfo(data json.RawMessage) (torrentID string, peers []Peer, seq int, hostUserID string, err error) {
	var payload struct {
		TorrentID  string `json:"torrentId"`
		Peers      []Peer `json:"peers"`
		RosterSeq  int    `json:"rosterSeq"`
		HostUserID string `json:"hostUserId"`
	}
	if err = json.Unmarshal(data, &payload); err != nil {
		return "", nil, 0, "", err
	}
	return payload.TorrentID, payload.Peers, payload.RosterSeq, payload.HostUserID, nil
}
