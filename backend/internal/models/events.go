// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package models

// Typed payloads for the room event stream (idea-0013).
//
// These used to be built as map[string]interface{} at each call site. Nothing
// described the payload except the literal that happened to construct it, so a
// renamed key was a runtime bug rather than a compile error, and the four sync
// call sites drifted apart without any of them noticing.
//
// The JSON produced here is byte-identical to what the maps produced; the tests
// in events_test.go are what guarantee that. This is a refactor of the shape,
// not a protocol change.

// Event types carried in P2PEvent.Type.
const (
	EventTypePeerJoined = "peer_joined"
	EventTypePeerLeft   = "peer_left"
	EventTypeSignal     = "signal"
	EventTypeSync       = "sync"
	// EventTypePeerList carries the whole roster (idea-0017).
	//
	// It supplements peer_joined/peer_left rather than replacing them: those
	// only describe a single change, so a client that joined late has no way to
	// learn about the peers that were already there. This event is the
	// authoritative snapshot that closes that gap, and it is the one a client
	// should trust over any bookkeeping it does itself.
	EventTypePeerList = "peer_list"
)

// Sync actions carried in SyncEvent.Action.
//
// "buffering" is deliberately in this list although it is not a playback
// command: any peer may report its own buffering state (idea-0018), so it
// travels the same channel as play/pause/seek.
const (
	SyncActionPlay      = "play"
	SyncActionPause     = "pause"
	SyncActionSeek      = "seek"
	SyncActionBuffering = "buffering"
)

// PeerJoinedEvent is the payload of an EventTypePeerJoined event.
//
// IsHost is reported for every peer rather than only the host, because a peer
// cannot otherwise learn that the room has a host at all without waiting for
// that host's own event.
type PeerJoinedEvent struct {
	PeerID string `json:"peerID" example:"peer-abc"`
	UserID string `json:"userID" example:"user-1"`
	IsHost bool   `json:"isHost" example:"true"`
	// PeerCount counts participants INCLUDING the host, and is measured AFTER
	// this peer joined. See RoomInfo.PeerCount for why this is worth spelling
	// out.
	PeerCount int `json:"peerCount" example:"3"`
}

// PeerLeftEvent is the payload of an EventTypePeerLeft event.
//
// There is no IsHost: when the host leaves, the room is torn down rather than
// promoted, so every peer_left leaves the remaining peers without a host.
type PeerLeftEvent struct {
	PeerID string `json:"peerID" example:"peer-abc"`
	UserID string `json:"userID" example:"user-1"`
	// PeerCount counts participants INCLUDING the host, and is measured AFTER
	// this peer left.
	PeerCount int `json:"peerCount" example:"2"`
}

// PeerListEvent is the payload of an EventTypePeerList event: the roster as the
// server sees it right now (idea-0017).
//
// It is a full replacement, not a delta. A client that missed an earlier event
// — because it joined late, or its stream dropped — converges on the next one
// instead of staying permanently out of date. That is also why applying it twice
// is harmless, which is what makes it safe to arrive on top of the reconciliation
// done after a reconnect (bug-0012).
type PeerListEvent struct {
	Peers []PeerInfo `json:"peers"`
	// RosterSeq is the room's roster version, comparable against the one in
	// RoomInfo. See RoomInfo.RosterSeq for why a client has to check it.
	RosterSeq int64 `json:"rosterSeq"`
}

// SyncEvent is the payload of an EventTypeSync event, describing what the room
// just did to playback.
//
// Position and Buffering are variant-specific and absent for actions that do
// not need them. Both are pointers rather than plain values for a reason:
//
//   - a seek duplicates Position at the top level, where the map literals did,
//     because the frontend reads status.position first and falls back to the
//     top level. omitempty on a float64 would drop a legitimate seek to 0:0.
//   - a buffering report of "I caught up" is buffering:false, and omitempty on
//     a bool would drop exactly the message that lifts a stalled room.
//
// A pointer distinguishes "this action has no such field" from "this field is
// zero", which is the distinction the omitempty tags above cannot express.
type SyncEvent struct {
	Action    string     `json:"action" example:"seek" enums:"play,pause,seek,buffering"`
	Position  *float64   `json:"position,omitempty" example:"42.5"`
	Buffering *bool      `json:"buffering,omitempty" example:"false"`
	Status    SyncStatus `json:"status"`
	Initiator string     `json:"initiator" example:"user-1"`
}

// NewSyncEvent builds a SyncEvent for action, attaching the variant-specific
// fields that action implies.
//
// The variant fields are decided here rather than at each call site: four
// map literals previously each had to remember whether a seek duplicated the
// position and a buffering report duplicated the flag, and the frontend depends
// on those spellings being present.
//
// The buffering flag is derived from status.Buffering rather than passed
// separately. Service.Buffering always writes the reported value into the
// status it returns, so the two could never have differed — passing them
// separately would only have offered a way to make them differ.
func NewSyncEvent(action string, status SyncStatus, initiator string) SyncEvent {
	e := SyncEvent{
		Action:    action,
		Status:    status,
		Initiator: initiator,
	}
	if action == SyncActionBuffering {
		buffering := status.Buffering
		e.Buffering = &buffering
	}
	return e
}

// WithRequestedPosition records the position the host asked for, returning the
// event for chaining.
//
// This is deliberately a different number from Status.Position on a seek.
// Service.Seek runs server-side smoothing, so a jump beyond the compensation
// threshold only moves part of the way: the payload carries both "where the host
// wanted to go" and "where the room actually is now". Collapsing them would lose
// the distinction the frontend uses to tell a deliberate jump from drift.
func (e SyncEvent) WithRequestedPosition(position float64) SyncEvent {
	e.Position = &position
	return e
}
