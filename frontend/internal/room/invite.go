// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package room

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// roomIDLength is the room id size in characters: 16 bytes of hex.
const roomIDLength = 32

// InviteCode is the "ID:password" line a host hands to guests.
//
// It is convenience, not security. It encrypts nothing and signs nothing: a
// room password is a password inside it, and anything the host would not send
// in the clear must still travel another way.
//
// The access token is deliberately NOT in the code. A host already has it, but
// it is a credential for the whole server rather than for one room — putting it
// in an invitation would raise a guest from "join this room" to "control this
// server". The media endpoint is authenticated the same way by now, so an
// invite no longer has to carry any URL credential at all.
type InviteCode struct {
	RoomID   string
	Password string
}

// Encode renders the code.
//
// The separator is the FIRST colon: a room id is hex and contains no colon,
// while a password is arbitrary and may contain any. Without that rule a
// password containing a colon would be cut in half at the wrong place.
func (c InviteCode) Encode() string {
	return c.RoomID + ":" + c.Password
}

// ParseInviteCode reads a code back.
//
// A code with no colon is read as an id with an empty password rather than
// rejected: someone pasting only the id has made a typo, not a mistake worth
// refusing over, and the server is the one that judges the password.
func ParseInviteCode(code string) (InviteCode, error) {
	id, password, found := strings.Cut(strings.TrimSpace(code), ":")

	if err := ValidateRoomID(id); err != nil {
		return InviteCode{}, err
	}

	if !found {
		return InviteCode{RoomID: strings.ToLower(id)}, nil
	}
	return InviteCode{RoomID: strings.ToLower(id), Password: password}, nil
}

// ValidateRoomID applies the same rule as the server: 32 hex characters.
//
// Checking on the client is not a security measure — the server checks again —
// but it turns a typo into an immediate message instead of a round trip, and it
// stops the app from sending rubbish to a stranger's server.
func ValidateRoomID(id string) error {
	if len(id) != roomIDLength {
		return fmt.Errorf("a room id is %d characters, this one has %d", roomIDLength, len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		return fmt.Errorf("a room id is hexadecimal: %w", err)
	}
	return nil
}
