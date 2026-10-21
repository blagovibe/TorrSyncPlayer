// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The room's protocol has a version, and the window is where a person meets
// it. A participant on the build before the version field existed is in the
// room and works — but its rooms will not have everything this build has, and
// a person watching should be able to tell whose experience is the older one.
//
// A participant that speaks a protocol this build does not know is not in the
// room at all, and the room says so: a participant who silently did not
// arrive is a participant somebody is waiting for.

// TestTheRosterMarksAParticipantOnAnOlderBuild pins the mark: a legacy peer
// gets a word after its name, and a peer on this build does not.
func TestTheRosterMarksAParticipantOnAnOlderBuild(t *testing.T) {
	peers := []rtc.Peer{
		{PeerID: "a", UserID: "aaaa1111", Username: "Аня", Version: rtc.ProtocolVersion},
		{PeerID: "b", UserID: "bbbb2222", Username: "Борис", Version: rtc.ProtocolVersionLegacy},
	}

	labels := describeRoster(trEN, peers)
	require.Len(t, labels, 2)
	assert.Equal(t, "Аня", labels[0], "a participant on this build needs no mark")
	assert.Contains(t, labels[1], "Борис")
	assert.Contains(t, labels[1], trEN("room.oldVersion"),
		"a participant on an older build must be marked: %q", labels[1])
}

// TestARefusedParticipantIsSaidOutLoud pins the refusal: the window says
// that somebody was not admitted, and says which version they speak.
func TestARefusedParticipantIsSaidOutLoud(t *testing.T) {
	_ = test.NewApp()
	client := api.New("http://127.0.0.1:1", "test-token")

	w, _ := newWindow(client, "peer-test", nil, WindowOptions{})
	w.roomCallbacks().OnPeerRefused("newer-peer", rtc.ProtocolVersion+1)

	got := w.events.Text
	require.NotEmpty(t, got, "a refused participant must be said out loud")
	assert.NotContains(t, got, "room.refused",
		"the line must be a sentence, not a key: %q", got)
	assert.Contains(t, got, "3",
		"the line must name the version they speak: %q", got)
}
