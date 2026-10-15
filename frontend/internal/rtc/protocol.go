// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package rtc is the room's voice once the server broker is gone: the messages
// that travel a WebRTC data channel between peers, and the two rules that decide
// whose commands count.
//
// The transport underneath is weron (signaler for the introduction, DTLS for the
// channel itself, AES on the signaling). What this package owns is everything
// above that: the shape of each message and the host authority the server used
// to enforce. Nothing here knows about sockets, signalers, or widgets, so the
// whole of it runs in a unit test.
package rtc

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Message types on the data channel. They mirror what the room's SSE stream
// used to carry, minus everything that was the server's bookkeeping (roster
// versions, sequence numbers, replay): the channel is reliable and ordered while
// it lives, so there is nothing to compensate for.
const (
	TypeMagnet    = "magnet"
	TypePause     = "pause"
	TypePosition  = "position"
	TypeBuffering = "buffering"
	TypeHello     = "hello"
	TypeHost      = "host"
	TypeBye       = "bye"
)

// Envelope is the one field every message carries, so a follower can route
// before it parses.
type Envelope struct {
	Type string `json:"type"`
}

// Magnet names what the room is watching. It travels the room for the same
// reason it used to travel the server broadcast: only members learn what the
// room is watching, and an infohash alone carries no torrent.
type Magnet struct {
	Envelope
	MagnetURI    string `json:"magnetUri"`
	InfoHash     string `json:"infoHash"`
	SelectedFile int    `json:"selectedFile"`
	Name         string `json:"name"`
}

// NewMagnet wraps a magnet announcement for the wire.
func NewMagnet(m Magnet) Magnet {
	m.Type = TypeMagnet
	return m
}

// Pause starts or stops playback. There is no separate play: pause=false is it,
// the way mpv itself spells it.
type Pause struct {
	Envelope
	Pause bool `json:"pause"`
}

// NewPause wraps a pause state for the wire.
func NewPause(paused bool) Pause {
	return Pause{Envelope: Envelope{Type: TypePause}, Pause: paused}
}

// Position is a raw seek target in seconds, as the host's player reports it.
// Smoothing happens on the follower, which knows its own lag and the host does
// not: the host sending a pre-smoothed number would steer everyone by one
// peer's delay.
type Position struct {
	Envelope
	Position float64 `json:"position"`
}

// NewPosition wraps a seek target for the wire.
func NewPosition(seconds float64) Position {
	return Position{Envelope: Envelope{Type: TypePosition}, Position: seconds}
}

// Buffering is a peer's report about itself, not a command: anyone may send it,
// host or not, because a recovered stream that cannot say so keeps the room
// paused for everyone.
type Buffering struct {
	Envelope
	Buffering bool `json:"buffering"`
}

// NewBuffering wraps a buffering report for the wire.
func NewBuffering(buffering bool) Buffering {
	return Buffering{Envelope: Envelope{Type: TypeBuffering}, Buffering: buffering}
}

// Hello announces a peer to the mesh. There is no server roster anymore; the
// roster is whoever said hello and has not said bye, or gone quiet.
type Hello struct {
	Envelope
	PeerID   string `json:"peerId"`
	UserID   string `json:"userId"`
	Username string `json:"username,omitempty"`
	IsHost   bool   `json:"isHost,omitempty"`
}

// NewHello wraps a presence announcement for the wire.
func NewHello(peerID, userID, username string, isHost bool) Hello {
	return Hello{
		Envelope: Envelope{Type: TypeHello},
		PeerID:   peerID,
		UserID:   userID,
		Username: username,
		IsHost:   isHost,
	}
}

// Host names the peer that holds the host role. It is sent on explicit transfer
// and on takeover, and it is believable only together with the sender: the name
// in the message must be the peer that sent it.
type Host struct {
	Envelope
	PeerID string `json:"peerId"`
}

// NewHost wraps a host announcement for the wire.
func NewHost(peerID string) Host {
	return Host{Envelope: Envelope{Type: TypeHost}, PeerID: peerID}
}

// Bye is a graceful leave. A quiet peer is noticed by the mesh going silent;
// a bye is noticed at once, which is what keeps the roster honest when someone
// closes the window instead of losing the network.
type Bye struct {
	Envelope
	PeerID string `json:"peerId"`
}

// NewBye wraps a leave announcement for the wire.
func NewBye(peerID string) Bye {
	return Bye{Envelope: Envelope{Type: TypeBye}, PeerID: peerID}
}

// Parse routes one wire message to its type. An unknown type is an error, not a
// guess: a newer host must not drive an older follower into nonsense.
func Parse(raw []byte) (any, error) {
	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("rtc: decode envelope: %w", err)
	}

	switch envelope.Type {
	case TypeMagnet:
		var m Magnet
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("rtc: decode magnet: %w", err)
		}
		return m, nil
	case TypePause:
		var m Pause
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("rtc: decode pause: %w", err)
		}
		return m, nil
	case TypePosition:
		var m Position
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("rtc: decode position: %w", err)
		}
		return m, nil
	case TypeBuffering:
		var m Buffering
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("rtc: decode buffering: %w", err)
		}
		return m, nil
	case TypeHello:
		var m Hello
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("rtc: decode hello: %w", err)
		}
		return m, nil
	case TypeHost:
		var m Host
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("rtc: decode host: %w", err)
		}
		return m, nil
	case TypeBye:
		var m Bye
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("rtc: decode bye: %w", err)
		}
		return m, nil
	default:
		return nil, fmt.Errorf("rtc: unknown message type %q", envelope.Type)
	}
}

// ShouldFollow is the one rule that replaces the server's host check: playback
// commands count only from the host peer — the one that sent the magnet. Anyone
// holding the room key can open a data channel, so arrival is not authority.
func ShouldFollow(fromPeerID, hostPeerID string) bool {
	return fromPeerID != "" && fromPeerID == hostPeerID
}

// TakeoverHost picks the new host when the old one is gone: the smallest peer
// ID among the living. No election, no clocks, no chatter — everyone sees the
// same peer set, so everyone independently arrives at the same answer. Empty in,
// empty out.
func TakeoverHost(peerIDs []string) string {
	if len(peerIDs) == 0 {
		return ""
	}
	ids := append([]string(nil), peerIDs...)
	sort.Strings(ids)
	return ids[0]
}
