// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package adopt

import (
	"context"
	"errors"
	"testing"
)

// A room is a shared viewing of one torrent, and the host decides which file that
// is: `SelectedFile` travels with the room's torrent event. The guest's copy of
// that decision was written into a field and read by nothing — so the guest
// joined, watched its own backend receive the magnet, and then sat there. "The
// others follow the host" was a server feature with no client half (bug-0078).

// TestTheRoomsFileReachesTheGuestsPlayer is the whole bug in one assertion: the
// file the host named is the file that gets played.
func TestTheRoomsFileReachesTheGuestsPlayer(t *testing.T) {
	files := []File{
		{Index: 0, Name: "Movie.2024.mkv"},
		{Index: 1, Name: "Movie.2024.srt"},
		{Index: 2, Name: "Movie.2024.nfo"},
	}

	got, ok := FileFor(files, 0)
	if !ok {
		t.Fatal("the host named a file and the guest would not adopt it")
	}
	if got.Index != 0 || got.Name != "Movie.2024.mkv" {
		t.Errorf("the guest would play file %d (%q), not the one the host chose", got.Index, got.Name)
	}
}

// TestTheHostMayNotHaveChosenYet is the other half: the room can name a torrent
// before anybody has picked a file, and inventing one is worse than waiting.
func TestTheHostMayNotHaveChosenYet(t *testing.T) {
	files := []File{{Index: 0, Name: "Movie.2024.mkv"}}

	if _, ok := FileFor(files, -1); ok {
		t.Error("the host had chosen no file and the guest picked one anyway")
	}

	// A file number the torrent does not have is the same silence: the torrent
	// may have three files and the room may name a fourth, and selecting it is
	// a 400 the guest cannot explain.
	if _, ok := FileFor(files, 3); ok {
		t.Error("the room named a file the torrent does not have, and it was adopted")
	}

	// And a torrent with no file list yet — metadata still arriving — offers
	// nothing to play.
	if _, ok := FileFor(nil, 0); ok {
		t.Error("a torrent with no files yet was played")
	}
}

// TestTheOfferIsAdoptedOncePerRoom is what keeps a two-second poll tick from
// restarting playback: the adoption happens when the torrent appears in the
// guest's own list, and never again.
func TestTheOfferIsAdoptedOncePerRoom(t *testing.T) {
	offer := Offer{
		RoomInfohash: "abc",
		HaveTorrents: []string{"abc"},
		HostFile:     0,
	}

	if got, ok := offer.Torrent(); !ok || got != "abc" {
		t.Fatalf("the room's torrent is in the guest's list and nothing would be adopted (%q, %v)", got, ok)
	}

	// Already adopted: a poll tick must not play it again.
	offer.Adopted = true
	if _, ok := offer.Torrent(); ok {
		t.Error("an already adopted offer was adopted again — every poll tick would restart playback")
	}
	offer.Adopted = false

	// The room has named no file yet: there is nothing to follow.
	offer.HostFile = -1
	if _, ok := offer.Torrent(); ok {
		t.Error("the room named no file and the guest started playing something")
	}
	offer.HostFile = 0

	// The room's torrent is not in the guest's list: metadata has not arrived,
	// which is the ordinary state for the first few seconds after a join.
	offer.HaveTorrents = []string{"def"}
	if _, ok := offer.Torrent(); ok {
		t.Error("a torrent the guest does not have was played")
	}
	offer.HaveTorrents = []string{"abc"}
}

// TestABusyGuestIsLeftAlone is the decision the owner made about a guest who is
// already watching something else: the room's own torrent is adopted, anything
// else is not touched, because that is no longer "a guest joining" but "a guest
// being interrupted".
func TestABusyGuestIsLeftAlone(t *testing.T) {
	offer := Offer{
		RoomInfohash: "abc",
		HaveTorrents: []string{"abc", "zzz"},
		HostFile:     0,
		Playing:      "zzz",
	}

	if _, ok := offer.Torrent(); ok {
		t.Error("the guest was watching something else and the room took the player away")
	}

	// Watching the room's own torrent is not "busy": the host's file choice
	// still has to be applied, and that is exactly the case this fix is for.
	offer.Playing = "abc"
	if _, ok := offer.Torrent(); !ok {
		t.Error("the guest was already on the room's torrent and the host's file was dropped")
	}
}

// TestTheGuestsPathRunsFromTheRoomToThePlayer exercises the whole carry-out, not
// just the decisions inside it.
//
// This is the part that had no test at all: the sequence lived inside window,
// which is built from Fyne widgets, so nothing but a running application ever
// ran it. The extraction exists so that a scenario against a real backend can
// drive the same code.
func TestTheGuestsPathRunsFromTheRoomToThePlayer(t *testing.T) {
	offer := Offer{
		RoomInfohash: "abc",
		HaveTorrents: []string{"abc"},
		HostFile:     1,
	}
	files := []File{
		{Index: 0, Name: "Movie.2024.mkv"},
		{Index: 1, Name: "Movie.2024.srt"},
	}

	var gotID string
	var gotFiles []File
	var gotChosen File
	played := 0
	play := func(id string, listed []File, chosen File) {
		played++
		gotID, gotFiles, gotChosen = id, listed, chosen
	}

	outcome, err := CarryOut(context.Background(), offer,
		func(context.Context, string) ([]File, error) { return files, nil },
		play)
	if err != nil {
		t.Fatalf("carrying out the offer failed: %v", err)
	}
	if outcome != Adopted {
		t.Fatalf("outcome was %v, not adopted", outcome)
	}
	if played != 1 {
		t.Fatalf("the player was handed the file %d times", played)
	}
	if gotID != "abc" {
		t.Errorf("the player would play %q, not the room's torrent", gotID)
	}
	if gotChosen.Index != 1 || gotChosen.Name != "Movie.2024.srt" {
		t.Errorf("the player was handed file %d (%q), not the one the host chose", gotChosen.Index, gotChosen.Name)
	}
	if len(gotFiles) != 2 {
		t.Errorf("the list handed to the player has %d entries", len(gotFiles))
	}
}

// TestAnUnreadableFileListSaysSoInsteadOfPlayingSomething is the failure the
// guest cannot otherwise explain: the torrent arrived, the room named a file, and
// the guest's own backend did not answer. Playing the first file of the release
// would look like success and would put the room in two places at once.
func TestAnUnreadableFileListSaysSoInsteadOfPlayingSomething(t *testing.T) {
	offer := Offer{RoomInfohash: "abc", HaveTorrents: []string{"abc"}, HostFile: 0}

	played := 0
	outcome, err := CarryOut(context.Background(), offer,
		func(context.Context, string) ([]File, error) { return nil, errors.New("no answer") },
		func(string, []File, File) { played++ })

	if outcome != Unreadable {
		t.Errorf("outcome was %v, not unreadable", outcome)
	}
	if err == nil {
		t.Error("the error was swallowed, so the window would have nothing to say")
	}
	if played != 0 {
		t.Error("something was played after the file list failed to arrive")
	}
}

// TestAFileTheTorrentDoesNotHaveIsRefused is the other fork: the metadata arrived
// and the number the room sent matches nothing. Selecting it is a 400 the guest
// cannot explain, and rounding it to the first file quietly breaks the room.
func TestAFileTheTorrentDoesNotHaveIsRefused(t *testing.T) {
	offer := Offer{RoomInfohash: "abc", HaveTorrents: []string{"abc"}, HostFile: 7}

	played := 0
	outcome, err := CarryOut(context.Background(), offer,
		func(context.Context, string) ([]File, error) {
			return []File{{Index: 0, Name: "Movie.mkv"}}, nil
		},
		func(string, []File, File) { played++ })

	if outcome != NoSuchFile {
		t.Errorf("outcome was %v, not no-such-file", outcome)
	}
	if err != nil {
		t.Errorf("an absent file is not an error to report: %v", err)
	}
	if played != 0 {
		t.Error("a file the torrent does not have was played")
	}
}

// TestNothingIsAskedOfTheBackendBeforeTheTorrentArrives is the state the guest
// spends its first seconds in: the magnet was added and no swarm has answered
// yet, so there is no file list to read. Asking anyway is a guaranteed failure
// every two seconds.
func TestNothingIsAskedOfTheBackendBeforeTheTorrentArrives(t *testing.T) {
	offer := Offer{RoomInfohash: "abc", HaveTorrents: []string{}, HostFile: 0}

	asked := 0
	played := 0
	outcome, err := CarryOut(context.Background(), offer,
		func(context.Context, string) ([]File, error) {
			asked++
			return nil, nil
		},
		func(string, []File, File) { played++ })

	if outcome != NotYet {
		t.Errorf("outcome was %v, not not-yet", outcome)
	}
	if err != nil {
		t.Errorf("not yet is not a failure: %v", err)
	}
	if asked != 0 || played != 0 {
		t.Errorf("the backend was asked %d times and the player %d, for a torrent that is not there", asked, played)
	}
}
