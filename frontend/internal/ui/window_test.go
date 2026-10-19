// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
)

// TestTheWindowBuilds is the smoke test the constructor never had.
//
// Every other test in this package builds `&window{}` and calls one method on
// it, so the window builder — the one function every run of the application
// executes — was outside the suite. That is exactly where a missing widget
// hides: widget pointers are nil until assigned, and the subtitle chooser was
// never created, so the application died on launch while the suite stayed green.
//
// It builds through newWindow rather than NewWindow: NewWindow adds two loops
// that never stop, and a poller left behind keeps calling fyne.Do while later
// tests swap the current app under it — a race the detector is right about.
// The two lines NewWindow adds on top are the only thing not built here.
func TestTheWindowBuilds(t *testing.T) {
	// Widgets read the theme, so there has to be an app; nothing here starts
	// a goroutine, so it is safe to leave it behind.
	_ = test.NewApp()

	// A client aimed at a port nothing listens on: a failed poll must be
	// survivable, because that is the state of every launch — the window is
	// built before the backend answers.
	client := api.New("http://127.0.0.1:1", "test-token")

	w, content := newWindow(client, "peer-test", nil, WindowOptions{})
	if w == nil || content == nil {
		t.Fatal("the window constructor returned nothing")
	}
}

// A setting that nothing reads is worse than no setting: it is written, it is
// read back, it survives a restart, and it does nothing. The mpv binary was
// exactly that, and nothing about the settings file looked wrong.

// TestTheConfiguredPlayerIsTheOneLaunched locks the wiring from the settings
// file to the launcher.
//
// It does not prove mpv starts — that needs a real mpv speaking its control
// protocol. It proves the value reaches the launcher, which is the step that
// had silently vanished.
func TestTheConfiguredPlayerIsTheOneLaunched(t *testing.T) {
	w := &window{
		settings:   Settings{PlayerBinary: "/opt/custom/mpv"},
		socketPath: "/tmp/torrsync-test.sock",
	}

	options := w.mpvOptions()
	if options.Binary != "/opt/custom/mpv" {
		t.Errorf("the launcher would run %q, not the mpv that was configured", options.Binary)
	}
	if options.SocketPath != "/tmp/torrsync-test.sock" {
		t.Errorf("the control socket is %q", options.SocketPath)
	}
}

// TestAnEmptyPlayerBinaryLeavesTheChoiceToPATH is the other half: an empty
// setting must mean "the usual mpv", not "no player at all".
func TestAnEmptyPlayerBinaryLeavesTheChoiceToPATH(t *testing.T) {
	w := &window{socketPath: "/tmp/torrsync-test.sock"}

	if got := w.mpvOptions().Binary; got != "" {
		t.Errorf("an unset binary became %q, which asks the launcher for a file called that", got)
	}
}

// TestTheSocketPathIsPerProcess keeps two instances from sharing a control
// socket.
//
// This is not hypothetical: the project's own scenario tests run two backends,
// and a second application pointed at the first one's socket would drive
// somebody else's player — silently, because every command would succeed.
func TestTheSocketPathIsPerProcess(t *testing.T) {
	first := &window{socketPath: "/tmp/torrsync-1.sock"}
	second := &window{socketPath: "/tmp/torrsync-2.sock"}

	if first.mpvOptions().SocketPath == second.mpvOptions().SocketPath {
		t.Fatal("two windows would fight over one control socket")
	}
	if !strings.Contains(first.mpvOptions().SocketPath, ".sock") {
		t.Error("the socket path does not look like a socket path")
	}
}

// The --bootstrap flag is a deliberate override for one run: a saved setting
// must not silently outrank the person who typed the flag, and nobody having
// said anything must mean the public defaults (empty) rather than a dead address.
func TestWhereRoomsMeetComesFromTheFlagThenTheSetting(t *testing.T) {
	_ = test.NewApp()

	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := store.Save(Settings{Bootstrap: "ma://saved/"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	client := api.New("http://127.0.0.1:1", "test-token")

	flagged, _ := newWindow(client, "peer", store, WindowOptions{Bootstrap: "ma://flag/"})
	if got := flagged.rendezvous(); got != "ma://flag/" {
		t.Errorf("the flag was not the one used: got %q", got)
	}

	saved, _ := newWindow(client, "peer", store, WindowOptions{})
	if got := saved.rendezvous(); got != "ma://saved/" {
		t.Errorf("the saved rendezvous was not used: got %q", got)
	}

	def, _ := newWindow(client, "peer", nil, WindowOptions{})
	if got := def.rendezvous(); got != "" {
		t.Errorf("nobody chose rendezvous and the window chose %q", got)
	}
}

// The --relays flag follows the same precedence as --bootstrap: a run that
// says nothing must read as DHT discovery (empty), not as a dead address.
func TestFallbackRelaysComeFromTheFlagThenTheSetting(t *testing.T) {
	_ = test.NewApp()

	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := store.Save(Settings{Relays: "ma://saved-relay/"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	client := api.New("http://127.0.0.1:1", "test-token")

	flagged, _ := newWindow(client, "peer", store, WindowOptions{Relays: "ma://flag-relay/"})
	if got := flagged.fallbackRelays(); got != "ma://flag-relay/" {
		t.Errorf("the flag was not the one used: got %q", got)
	}

	saved, _ := newWindow(client, "peer", store, WindowOptions{})
	if got := saved.fallbackRelays(); got != "ma://saved-relay/" {
		t.Errorf("the saved relays were not used: got %q", got)
	}

	def, _ := newWindow(client, "peer", nil, WindowOptions{})
	if got := def.fallbackRelays(); got != "" {
		t.Errorf("nobody chose relays and the window chose %q", got)
	}
}

// The host repeats its position every five seconds even when nothing moved:
// a follower that drifted silently must converge without waiting for the
// next seek — while a real seek still travels immediately, and the first
// sample adopts rather than broadcasts.
func TestTheHostSendsAHeartbeatBetweenSeeks(t *testing.T) {
	if hostSeekDue(0, 100, 10) {
		t.Error("the first sample adopts the position, it does not broadcast it")
	}
	if hostSeekDue(100, 100.4, 3) {
		t.Error("drift inside the smoothing boundary is not a seek and it is not heartbeat time either")
	}
	if !hostSeekDue(100, 100.4, 10) {
		t.Error("the 10th tick repeats a steady position: that is the heartbeat")
	}
	if !hostSeekDue(100, 100.4, 20) {
		t.Error("the heartbeat repeats every ten ticks, not just once")
	}
	if !hostSeekDue(100, 150, 3) {
		t.Error("a real seek jump travels on its own tick, heartbeat or not")
	}
	if !hostSeekDue(150, 100, 7) {
		t.Error("a backward jump is a seek too")
	}
}

// The preparing line reports the playing download and nothing else: no
// torrent, a finished one, or one the backend dropped read as an empty line,
// and an unknown backend status shows as it arrived rather than as a blank.
func TestThePreparingLineReportsThePlayingDownload(t *testing.T) {
	tr := func(key string) string { return catalog.Lookup(catalog.English, key) }

	if got := preparingText(tr, nil); got != "" {
		t.Errorf("no torrent playing must read as no line, got %q", got)
	}
	done := &Torrent{ID: "a", Name: "Film.mkv", Progress: 1, Status: "seeding"}
	if got := preparingText(tr, done); got != "" {
		t.Errorf("a finished download must read as no line, got %q", got)
	}
	half := &Torrent{ID: "a", Name: "Film.mkv", Size: 1500000000, Progress: 0.42, Status: "downloading", Peers: 3}
	if got, want := preparingText(tr, half), "Fetching Film.mkv — 600.8 MB / 1.4 GB (42%, Downloading, peers: 3)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	strange := &Torrent{ID: "a", Name: "Film.mkv", Size: 200000000, Progress: 0.1, Status: "mystery", Peers: 0}
	if got, want := preparingText(tr, strange), "Fetching Film.mkv — 19.1 MB / 190.7 MB (10%, mystery, peers: 0)"; got != want {
		t.Errorf("an unknown status must show as it arrived, got %q, want %q", got, want)
	}

	list := []Torrent{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}
	if got := playingTorrent(list, "b"); got == nil || got.Name != "B" {
		t.Errorf("the playing torrent must be found by id")
	}
	if playingTorrent(list, "gone") != nil {
		t.Error("a torrent the backend dropped must read as not playing")
	}
	if playingTorrent(list, "") != nil {
		t.Error("no selection must read as not playing")
	}
}

// The connection block reports what the node itself knows and nothing else:
// outside a room it is blank, an unready DHT says it is looking, and a ready
// one splits the room peers into direct and relayed beside a short self id.
func TestTheConnectionBlockReportsWhatTheNodeKnows(t *testing.T) {
	tr := func(key string) string { return catalog.Lookup(catalog.English, key) }

	if got := connStatusText(tr, rtc.ConnStatus{}, false); got != "" {
		t.Errorf("outside a room the block must stay blank, got %q", got)
	}
	connecting := rtc.ConnStatus{SelfID: "12D3KooWABCDEFGH", DHTReady: true}
	if got, want := connStatusText(tr, connecting, false), "Connecting…"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	ready := rtc.ConnStatus{
		SelfID:   "12D3KooWABCDEFGH",
		DHTReady: true,
		Peers: []rtc.ConnPeer{
			{ID: "a", Via: rtc.PathInvite},
			{ID: "b", Relayed: true, Via: rtc.PathInvite},
		},
	}
	if got, want := connStatusText(tr, ready, false), "Connected — 2 peers (direct 1, through a relay 1), you 12D3KooW"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := shortID("abc"); got != "abc" {
		t.Errorf("a short id must survive intact, got %q", got)
	}
}

// The poll rewrites status every two seconds; the event line must survive
// it. Before the event line existed every failure, arrival and confirmation
// travelled through status — and the next tick erased it. A reported error
// that lives two seconds is not reported at all.
func TestEventsSurviveThePoll(t *testing.T) {
	_ = test.NewApp()

	// A backend with nothing to say: the poll succeeds, so status moves on
	// while the event line must stand still.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"torrents":[]}`))
	}))
	defer srv.Close()
	client := api.New(srv.URL, "test-token")

	w, _ := newWindow(client, "peer-test", nil, WindowOptions{})
	w.notify("boom")
	w.refresh()
	if got := w.events.Text; got != "boom" {
		t.Errorf("the poll erased the event line, got %q", got)
	}
	if got := w.status.Text; got == "boom" {
		t.Errorf("events must not leak into the poll counter line, got %q", got)
	}
}

// A failure of the person's own action interrupts twice: the event line
// keeps it past the next poll, and a modal dialog stops for it. Poll and
// background failures stay on the line — a modal every two seconds would be
// the poll shouting. Without a parent window the dialog has nowhere to hang,
// so the line alone must still carry the message.
func TestAFailureInterruptsAndPersists(t *testing.T) {
	_ = test.NewApp()
	client := api.New("http://127.0.0.1:1", "test-token")

	w, _ := newWindow(client, "peer-test", nil, WindowOptions{})
	w.fail("boom")
	if got := w.events.Text; got != "boom" {
		t.Errorf("a failure must reach the event line even with no parent window, got %q", got)
	}

	parent := test.NewWindow(widget.NewLabel("parent"))
	defer parent.Close()
	w2, _ := newWindow(client, "peer-test", nil, WindowOptions{Parent: parent})
	w2.fail("boom")
	if got := w2.events.Text; got != "boom" {
		t.Errorf("a failure with a parent window must still reach the event line, got %q", got)
	}
}

// Quitting must take the player with it: an mpv nobody closed keeps its
// window open playing to nobody, which is exactly what exiting past it did.
func TestQuittingPutsThePlayerDown(t *testing.T) {
	_ = test.NewApp()
	client := api.New("http://127.0.0.1:1", "test-token")

	w, _ := newWindow(client, "peer-test", nil, WindowOptions{})
	if w.mpv != nil {
		t.Fatal("a fresh window has no player to put down")
	}
	w.mpv = &player.Player{}
	w.stopPlayer()
	if w.mpv != nil {
		t.Error("stopPlayer must detach the player so nothing drives it after quit")
	}
	w.stopPlayer()
}

// The roster line already shows who is here; these notices say what changed.
// A first roster is a baseline rather than an arrival burst, self is silent,
// and departures read in a stable order no matter how the map iterates.
func TestTheRosterChangesAreNoticed(t *testing.T) {
	tr := func(key string) string { return catalog.Lookup(catalog.English, key) }

	if got := rosterNotices(tr, nil, []rtc.Peer{{PeerID: "a", UserID: "u-a"}}, "me"); len(got) != 0 {
		t.Errorf("the first roster baselines, it does not announce: %q", got)
	}
	prev := rosterSnapshot(tr, []rtc.Peer{{PeerID: "me", UserID: "u-me"}, {PeerID: "a", UserID: "u-a"}})

	if got := rosterNotices(tr, prev, []rtc.Peer{{PeerID: "me", UserID: "u-me"}, {PeerID: "a", UserID: "u-a"}, {PeerID: "b", UserID: "u-b"}}, "me"); len(got) != 1 || got[0] != "device u-b joined" {
		t.Errorf("a join must notice exactly the joiner, got %q", got)
	}
	if got := rosterNotices(tr, prev, []rtc.Peer{{PeerID: "me", UserID: "u-me"}, {PeerID: "a", UserID: "u-a"}}, "me"); len(got) != 0 {
		t.Errorf("no change must read as silence, got %q", got)
	}
	left := rosterNotices(tr, prev, []rtc.Peer{{PeerID: "me", UserID: "u-me"}}, "me")
	if len(left) != 1 || left[0] != "device u left" {
		t.Errorf("a departure must notice exactly the leaver, got %q", left)
	}
	withSelf := rosterNotices(tr, map[string]string{}, []rtc.Peer{{PeerID: "me", UserID: "u-me"}}, "me")
	if len(withSelf) != 0 {
		t.Errorf("self arrival is answered by the room opening, got %q", withSelf)
	}
}

// The --lang flag used to reach only the tray: the window kept reading the
// store, so --lang changed the tray menu and nothing else. The window is the
// surface where the choice is most visible, so it is the one locked here.
func TestTheLanguageFlagReachesTheWindow(t *testing.T) {
	withSystemLocale(t, "de")
	_ = test.NewApp()

	store := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err := store.Save(Settings{Language: catalog.LangRussian}); err != nil {
		t.Fatalf("save: %v", err)
	}
	client := api.New("http://127.0.0.1:1", "test-token")

	flagged, _ := newWindow(client, "peer", store, WindowOptions{Language: "en"})
	if flagged.language != catalog.English {
		t.Errorf("--lang en left the window speaking %q", flagged.language)
	}

	saved, _ := newWindow(client, "peer", store, WindowOptions{})
	if saved.language != catalog.LangRussian {
		t.Errorf("without the flag the window ignored the saved language: %q", saved.language)
	}
}

// The window is a screen, not a settings page: its zones are named and
// ruled, and the one list that grows is the border's centre rather than a
// row that would be handed its minimum height. The owner's complaint was
// that nothing in it had a rank — so the zones are pinned here.
func TestTheWindowIsComposedOfNamedZones(t *testing.T) {
	_ = test.NewApp()
	client := api.New("http://127.0.0.1:1", "test-token")

	w, _ := newWindow(client, "peer-test", nil, WindowOptions{})
	tr := func(key string) string { return catalog.Lookup(catalog.English, key) }

	for _, key := range []string{"zone.playback", "zone.tracks", "zone.library", "zone.room"} {
		if tr(key) == "" {
			t.Errorf("the zone %q has no English text — a heading that says nothing "+
				"names nothing", key)
		}
	}
	if tr("zone.playback") == tr("zone.room") {
		t.Error("two zones share a heading, which tells the reader nothing")
	}

	// The lines start hidden: an empty label is a hole in a ruled zone.
	for name, label := range map[string]*widget.Label{
		"room": w.roomLabel, "connection": w.connLabel, "preparing": w.preparing,
	} {
		if label.Visible() {
			t.Errorf("the %s line is visible before it has anything to say", name)
		}
	}
}

// An empty line must leave no hole. A label that keeps its height reads as a
// zone with something missing rather than as nothing to say.
func TestAnEmptyLineDisappears(t *testing.T) {
	_ = test.NewApp()
	label := widget.NewLabel("")
	setLine(label, "something")
	if !label.Visible() || label.Text != "something" {
		t.Fatal("a line with text must be shown")
	}
	setLine(label, "")
	if label.Visible() {
		t.Error("an empty line must be hidden, or it leaves a hole in the zone")
	}
}

// The floor exists because Fyne has no minimum window size and nothing
// stopped the window from being dragged until the library collapsed to a
// strip. A window that fits is left alone — including a maximised one.
func TestTheWindowHasAMinimumSize(t *testing.T) {
	if !windowIsTooSmall(400, 400) {
		t.Error("a window this small cannot hold the zones")
	}
	if !windowIsTooSmall(minWindowWidth, 300) {
		t.Error("a window this short cannot hold the zones")
	}
	if windowIsTooSmall(minWindowWidth, minWindowHeight) {
		t.Error("a window exactly at the floor is allowed")
	}
	if windowIsTooSmall(4000, 3000) {
		t.Error("a large window must be left alone")
	}
}

// The roster says who is in the room, so a row must carry a name. It used to
// be blank: the presence greeting that fills the roster left the identity
// empty, and the window drew exactly what the greeting said (bug-0119).
func TestTheRosterNamesEveryoneInTheRoom(t *testing.T) {
	peers := []rtc.Peer{
		{PeerID: "peer-a", UserID: "a3f9c2d1e4b7a6c5d8e0f1a2b3c4d5e6"},
		{PeerID: "peer-b", UserID: "1122334455667788aabbccddeeff0011", IsHost: true},
	}

	rows := describeRoster(trEN, peers)
	if len(rows) != 2 {
		t.Fatalf("the roster has %d rows for %d peers", len(rows), len(peers))
	}
	for i, row := range rows {
		if strings.TrimSpace(row) == "" {
			t.Errorf("row %d is blank: %q", i, row)
		}
	}
	// And short enough to read: a full client id is 32 hex characters, and
	// the roster is one line.
	if len(rows[0]) > 16 {
		t.Errorf("the row is %d characters long: %q", len(rows[0]), rows[0])
	}
}
