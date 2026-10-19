// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"sort"
	"strings"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ConnPeer is one room peer's connection as the local node sees it: present
// and either direct or relayed. There is no third state to report — a peer
// the node holds no connection to is not a peer it can describe.
type ConnPeer struct {
	ID      string
	Relayed bool
}

// ConnStatus is the compact connection diagnostics the room window shows:
// whether the DHT rendezvous is usable, whether this room announced itself,
// who is around, and how each of them is reached. It is built from the node
// itself — no new protocol, no new messages — so it cannot disagree with what
// the node can actually do.
type ConnStatus struct {
	SelfID   string
	DHTReady bool
	// Published says whether the room announced itself to the DHT. False
	// means a guest who is not here yet will never be told this room
	// exists, and a person waiting for that guest needs to know it.
	Published bool
	// InviteFailed says the address in the invite code did not work. The
	// room is then on the fallback path alone, and if the fallback is down
	// too, the room cannot form at all — which is the one failure a person
	// can do something about.
	InviteFailed bool
	Peers        []ConnPeer
}

// DHTReady reports whether the DHT rendezvous is usable: created and with a
// non-empty routing table. Before Bootstrap it is false, not an error —
// there is simply nothing to ask yet.
func (n *Node) DHTReady() bool {
	d := n.getDHT()
	return d != nil && d.RoutingTable().Size() > 0
}

// PeerRelayed reports whether the connection to the peer goes through a
// relay: any of its connection addresses carries /p2p-circuit. A peer with
// no connection at all reads as direct — absence of a connection is the
// roster's business, and a missing peer must not look relayed.
func (n *Node) PeerRelayed(id peer.ID) bool {
	for _, c := range n.host.Network().ConnsToPeer(id) {
		if strings.Contains(c.RemoteMultiaddr().String(), "/p2p-circuit") {
			return true
		}
	}
	return false
}

// ConnStatus snapshots this node's view for the room window. Peers are the
// room's admitted members with their relay flags resolved here; the DHT flag
// and the self ID come along. Sorted by ID: the window must not reshuffle
// lines under the reader on every poll.
func (m *Mesh) ConnStatus() ConnStatus {
	m.mu.RLock()
	node := m.node
	inviteFailed := m.inviteFailed
	ids := make([]string, 0, len(m.peers))
	for id := range m.peers {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	st := ConnStatus{}
	if node == nil {
		return st
	}
	st.SelfID = node.ID().String()
	st.DHTReady = node.DHTReady()
	st.Published = node.Published()
	st.InviteFailed = inviteFailed
	for _, id := range ids {
		pid, err := peer.Decode(id)
		if err != nil {
			continue
		}
		st.Peers = append(st.Peers, ConnPeer{ID: id, Relayed: node.PeerRelayed(pid)})
	}
	sort.Slice(st.Peers, func(i, j int) bool { return st.Peers[i].ID < st.Peers[j].ID })
	return st
}
