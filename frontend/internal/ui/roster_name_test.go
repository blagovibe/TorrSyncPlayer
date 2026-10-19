// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"path/filepath"
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A room that says who is in it is the promise; a room that says which machine
// is in it is not. The protocol has carried a display name since the greeting
// gained one, and the window never filled it in — so every row was a short hex
// id and the roster answered "which device", not "who am I watching with".
// These tests pin the three answers the owner settled on: the name comes from
// the settings, an unnamed participant says so, and a name is not a licence to
// lie about the host or the stalling (idea-0125).

func trEN(key string) string { return catalog.Lookup(catalog.English, key) }

// TestTheRosterShowsTheNameWhenThereIsOne pins the first answer: a participant
// who set a name is shown by it, in the roster line and in the arrival notice.
func TestTheRosterShowsTheNameWhenThereIsOne(t *testing.T) {
	peers := []rtc.Peer{{PeerID: "abc", UserID: "a1b2c3d4", Username: "Аня"}}

	assert.Equal(t, []string{"Аня"}, describeRoster(trEN, peers))

	notices := rosterNotices(trEN, nil, peers, "self")
	assert.Empty(t, notices, "the first roster after joining is a baseline, not arrivals")

	// A previous roster that does not contain this peer: an empty map is the
	// baseline case, which by design announces nothing.
	notices = rosterNotices(trEN, map[string]string{"other": "device 0000"}, peers, "other")
	require.Len(t, notices, 1)
	assert.Equal(t, "Аня joined", notices[0],
		"the arrival notice must name the person, not their device")
}

// TestAnUnnamedParticipantIsCalledADevice pins the second answer. The short id
// stays — it is how two machines with the same name are told apart — but it is
// labelled, so nobody reads a hex string as somebody's name.
func TestAnUnnamedParticipantIsCalledADevice(t *testing.T) {
	peers := []rtc.Peer{{PeerID: "abc", UserID: "a1b2c3d4e5f6"}}

	assert.Equal(t, []string{"device a1b2c3d4"}, describeRoster(trEN, peers))
}

// TestTheHostAndTheStallerAreStillMarked pins that a name changes nothing
// else: the role and the stall are the room's shared state and must survive
// any label.
func TestTheHostAndTheStallerAreStillMarked(t *testing.T) {
	peers := []rtc.Peer{
		{PeerID: "a", UserID: "aaaa1111", Username: "Аня", IsHost: true},
		{PeerID: "b", UserID: "bbbb2222", Buffering: true},
	}

	assert.Equal(t, []string{
		"Аня (host)",
		"device bbbb2222 — buffering",
	}, describeRoster(trEN, peers))
}

// TestTheNameIsSavedWithTheSettings pins where the name comes from: the
// settings, not the room. A name typed once and forgotten is the point —
// asking again on every join is what made this a question instead of a
// feature.
func TestTheNameIsSavedWithTheSettings(t *testing.T) {
	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	require.NoError(t, store.Save(Settings{DisplayName: "Аня"}))

	assert.Equal(t, "Аня", store.Load().DisplayName)
}

// TestANameIsOptional pins that an empty name is not a broken settings file:
// most people will never set one, and the roster says «устройство» for them.
func TestANameIsOptional(t *testing.T) {
	s := Settings{}
	assert.Empty(t, s.Problem(), "no name must not be a settings problem")

	s.DisplayName = "   "
	assert.Empty(t, s.Problem(), "a blank name is no name")
}
