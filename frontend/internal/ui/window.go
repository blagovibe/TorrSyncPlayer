// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package ui draws the application window.
//
// Kept free of network and process logic on purpose: everything it needs
// arrives through the client it is handed, so a window can be exercised without
// starting a backend.
package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/adopt"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/room"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
)

// Torrent is one row of the list, as the data plane reports it.
type Torrent struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
	Peers    int     `json:"peers"`
}

// window is the top-level window.
type window struct {
	client   *api.Client
	clientID string
	settings Settings
	store    *SettingsStore
	// parent is the application window error dialogs hang off. Tests build
	// windows without one; then failures still reach the event line, just
	// without the modal.
	parent fyne.Window

	list     *widget.List
	fileList *widget.List
	status   *widget.Label
	// events is what a human must not miss: arrivals, departures, failures.
	// The poll rewrites status every two seconds, so anything reported
	// through status alone is gone by the next tick — an error that lives
	// two seconds is not reported at all. Events persist until the next
	// event; the poll never touches this line.
	events    *widget.Label
	language  string
	entry     *widget.Entry
	addButton *widget.Button

	// socketPath is where mpv's control socket goes. Per process, so two
	// instances on one machine — which the project's own tests do — cannot
	// end up talking to each other's player.
	socketPath string

	mu       sync.RWMutex
	torrents []Torrent
	current  *Torrent
	files    []fileEntry
	mpv      *player.Player

	// pendingRoomFile is the file the host is playing, remembered until the
	// file list arrives. Selection is per-peer, so without it a guest would
	// pick its own and the room would watch two parts of one release.
	// -1 means the host has not chosen one.
	pendingRoomFile int

	// selectedFile is the file THIS peer chose in its own backend, or -1. The
	// host announces it so the room knows what is playing; the guest records
	// it the same way when it follows.
	selectedFile int

	// roomInfohash is the torrent the room is watching, and adoptedRoomFile
	// records that the guest has already applied the host's choice. Both exist
	// so that the adoption happens once, when the torrent appears in the
	// guest's own list, and not again on every two-second poll tick
	// (bug-0078). Before they existed the guest joined a room and nothing
	// played.
	roomInfohash    string
	adoptedRoomFile bool

	// Room state. There is no server anymore: the roster is whoever said hello
	// over the mesh, the host role moves by message, and the film arrives as a
	// magnet from the host. The rules live in the rtc package because they are
	// subtle; here they are only driven.
	mesh    *rtc.Mesh
	rtcRoom *rtc.Room
	roomID  string
	invite  rtc.Invite
	// selfPeerID is this window's mesh peer id, set on join. Roster notices
	// skip it: my own arrival is answered by the room opening.
	selfPeerID string
	// lastRoster is the previous roster by peer id. OnRoster hands the whole
	// roster every time, so the window diffs it itself to notice arrivals
	// and departures. Reset on leave: the next join baselines again.
	lastRoster map[string]string
	roomLabel  *widget.Label
	// connLabel is the compact connection diagnostics: DHT readiness, peer
	// count split into direct and relayed, and the short self id. It lives
	// next to the room line because that is where "who is here" is answered —
	// "how am I connected" belongs beside it.
	connLabel *widget.Label
	// Playback controls: the window's remote over the same mpv the room
	// drives. Widgets live here; their decisions live in controls.go.
	playBtn                  *widget.Button
	elapsedLbl, remainingLbl *widget.Label
	mediaLbl                 *widget.Label
	seekSlider, volSlider    *widget.Slider
	fsBtn                    *widget.Button
	hasSeek                  bool
	seekPos                  float64
	seekAt                   time.Time
	ctlPaused                bool
	ctlOn                    bool
	buffer                   *room.BufferReporter

	// preparingLine shows the playing torrent's download state: name, percent
	// and backend status from the 2-second list poll. Playback starts before
	// the download finishes, so "nothing is ready yet" is a normal state,
	// not an error — and it deserves its own line rather than the status
	// line, which carries transient messages.
	preparing *widget.Label

	// bufferCancel ends the buffer report of the file that was playing before
	// the current one. One report per file: a switch must stop steering the
	// backend's download window toward a film nobody is watching.
	bufferCancel context.CancelFunc

	// bootstrapFlag is the --bootstrap value for this run (empty when the flag
	// was not given), kept apart from the settings because the flag
	// deliberately outranks the saved choice. Where rooms actually meet is
	// rendezvous(), read at every join.
	bootstrapFlag string

	// relaysFlag is the --relays value for this run, same precedence shape:
	// flag, then the saved choice, then DHT discovery. Read at every join
	// beside rendezvous().
	relaysFlag string

	inviteEntry *widget.Entry
	createRoom  *widget.Button

	// Track choosers. mpv knows every voice and every subtitle track in the
	// file; until this existed a person who needed the dub had no way to reach
	// it from the application (idea-0080).
	audioSelect *widget.Select
	subSelect   *widget.Select
	audioTracks []player.Track
	subTracks   []player.Track

	// Subtitle-file chooser. Container tracks live in the film; subtitle
	// files live next to it in the torrent, so they get a chooser of their
	// own (idea-0080, variant 2).
	subFileSelect *widget.Select
}

// playerSocketPath is what mpv should listen on for this process.
//
// On Unix it is a path under the temp directory. On Windows it is a bare pipe
// name: a named pipe takes a name, and dial adds `\\.\pipe\` — a path is not
// a valid pipe name, and handing mpv one leaves it with nothing to open. The pid
// keeps two instances of the application from driving each other's player, which
// is the same reason the socket is per process there too.
func playerSocketPath(pid int) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("torrsync-%d", pid)
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("torrsync-%d.sock", pid))
}

// WindowOptions are the launch choices a flag can also make. Each outranks the
// saved setting for this run; an empty value means "the setting decides".
type WindowOptions struct {
	// Language is the --lang value.
	Language string
	// Bootstrap is the --bootstrap value.
	Bootstrap string
	// Relays is the --relays value.
	Relays string
	// Parent is the application window dialogs hang off. Empty in tests,
	// where there is no window to hang one off.
	Parent fyne.Window
}

// OpenSettings opens the settings form over the given window.
//
// Handed back by NewWindow because the tray menu has no widget tree of its own:
// a click there arrives with nothing to hang a dialog on, while the settings
// read and write the window's own store. Safe to call from any goroutine —
// the tray's clicks do not arrive on the main one.
type OpenSettings func(parent fyne.Window)

// NewWindow builds the main window around a client and starts its loops.
//
// It hands back a stop function beside the content: quitting must put the
// player down, because an mpv nobody closed stays running idle with its
// window open and its control socket behind. The settings opener is there for
// the tray, which is the only way back into a window the owner has closed.
func NewWindow(client *api.Client, clientID string, store *SettingsStore, opts WindowOptions) (fyne.CanvasObject, OpenSettings, func()) {
	w, content := newWindow(client, clientID, store, opts)
	go w.poll()
	go w.watchPlayer()
	return content, w.openSettingsFromAnywhere, w.stopPlayer
}

// openSettingsFromAnywhere is OpenSettings on the main thread.
//
// A dialog built off it would be built on the wrong one: the tray's clicks
// arrive on systray's goroutine, and every widget change in this package
// already goes through onMain for the same reason (bug-0111).
func (w *window) openSettingsFromAnywhere(parent fyne.Window) {
	onMain(func() { w.ShowSettings(parent) })
}

// stopPlayer puts the player down: without it the application exits and
// the film window stays, playing to nobody.
func (w *window) stopPlayer() {
	w.mu.Lock()
	mpv := w.mpv
	w.mpv = nil
	w.mu.Unlock()
	if mpv != nil {
		mpv.Close()
	}
}

// newWindow builds the window and its contents, and starts nothing: the
// polling goroutines belong to NewWindow, so a test can look at the wiring
// without leaving a poller behind.
func newWindow(client *api.Client, clientID string, store *SettingsStore, opts WindowOptions) (*window, fyne.CanvasObject) {
	w := &window{
		client:          client,
		clientID:        clientID,
		store:           store,
		parent:          opts.Parent,
		bootstrapFlag:   opts.Bootstrap,
		relaysFlag:      opts.Relays,
		socketPath:      playerSocketPath(os.Getpid()),
		status:          widget.NewLabelWithStyle("Starting…", fyne.TextAlignLeading, fyne.TextStyle{}),
		events:          widget.NewLabel(""),
		pendingRoomFile: -1,
		selectedFile:    -1,
	}
	if store != nil {
		w.settings = store.Load()
	}
	w.language = PreferredLanguage(opts.Language, w.settings.Language)

	w.entry = widget.NewEntry()
	w.entry.SetPlaceHolder("magnet:?xt=urn:btih:…")
	w.entry.OnSubmitted = func(string) { w.addTorrent() }

	w.addButton = widget.NewButtonWithIcon(w.tr("action.add"), theme.ContentAddIcon(), w.addTorrent)
	w.addButton.Importance = widget.HighImportance

	w.list = widget.NewList(
		func() int {
			w.mu.RLock()
			defer w.mu.RUnlock()
			return len(w.torrents)
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			w.mu.RLock()
			defer w.mu.RUnlock()
			if id < 0 || id >= len(w.torrents) {
				return
			}
			label := obj.(*widget.Label)
			label.SetText(w.torrents[id].Name)
		},
	)
	w.list.OnSelected = func(id widget.ListItemID) { w.play(w.item(id)) }

	// The magnet field takes the room it needs: in a plain row it was
	// clipped to a few characters, which made the one field everything
	// starts from look like a leftover.
	// The magnet field is the border's object, not its left panel: a side
	// panel is handed its minimum width, which clipped the one field
	// everything starts from down to a few characters.
	toolbar := container.NewBorder(nil, nil,
		w.addButton,
		container.NewHBox(
			widget.NewButtonWithIcon(w.tr("action.refresh"), theme.ViewRefreshIcon(), w.refresh),
			layout.NewSpacer(),
			widget.NewButtonWithIcon(w.tr("action.shortcuts"), theme.InfoIcon(), w.showShortcuts),
			widget.NewButtonWithIcon(w.tr("action.settings"), theme.SettingsIcon(), func() { w.ShowSettings(w.parentWindow()) }),
		),
		w.entry,
	)

	// The file list sits next to the torrent list: picking a torrent answers
	// "which file", and that question is asked every time.
	w.fileList = widget.NewList(
		func() int {
			w.mu.RLock()
			defer w.mu.RUnlock()
			return len(w.files)
		},
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			w.mu.RLock()
			defer w.mu.RUnlock()
			if id < 0 || id >= len(w.files) {
				return
			}
			obj.(*widget.Label).SetText(fmt.Sprintf("%d. %s (%s)",
				w.files[id].Index, w.files[id].Name, humanSize(w.files[id].Size)))
		},
	)
	w.fileList.OnSelected = func(id widget.ListItemID) {
		w.mu.RLock()
		var f fileEntry
		if id >= 0 && id < len(w.files) {
			f = w.files[id]
		}
		w.mu.RUnlock()
		w.playFile(f)
	}

	// The choosers start hidden: most films have one voice, and a row of
	// empty selectors teaches a person that the app has nothing to offer.
	// The choosers get their placeholder from the catalogue, not from Fyne's
	// default: that default is an English string looked up in translations
	// the build does not carry, and the failure is logged on every redraw —
	// a person running the Russian interface got a log full of somebody
	// else's error (bug-0110).
	w.audioSelect = widget.NewSelect(nil, nil)
	w.audioSelect.PlaceHolder = w.tr("track.none")
	w.audioSelect.OnChanged = func(choice string) { w.chooseTrack(player.TrackAudio, choice) }
	w.subSelect = widget.NewSelect(nil, nil)
	w.subSelect.PlaceHolder = w.tr("track.none")
	w.subSelect.OnChanged = func(choice string) { w.chooseTrack(player.TrackSub, choice) }
	w.audioSelect.Hide()
	w.subSelect.Hide()
	w.subFileSelect = widget.NewSelect(nil, nil)
	w.subFileSelect.PlaceHolder = w.tr("track.none")
	w.subFileSelect.OnChanged = func(choice string) { w.chooseSubFile(choice) }
	w.subFileSelect.Hide()

	// The room line sits above the lists: who is watching with you is a
	// one-line answer that belongs where it is always visible. The preparing
	// line sits beside it: what is still downloading while you watch.
	// All three start hidden: they carry nothing before a room or a film
	// exists, and an empty label is a gap in a ruled zone (see setLine).
	w.roomLabel = widget.NewLabel("")
	w.connLabel = widget.NewLabel("")
	w.preparing = widget.NewLabel("")
	w.roomLabel.Hide()
	w.connLabel.Hide()
	w.preparing.Hide()

	// Joining takes an invite code, not a room id and a password: the room has
	// no server to ask, so there is nothing to look up. The triple carries the
	// community, the password that admits, and the key that encrypts.
	w.inviteEntry = widget.NewEntry()
	w.inviteEntry.SetPlaceHolder(w.tr("field.invite"))

	// Opening a room sits next to joining one: they are the same two actions on
	// the same panel, and before this button existed the panel could do only
	// half of them. It starts disabled and wakes up when a torrent is picked —
	// a room is bound to one, and a button that could only fail is worse than a
	// grey one.
	w.createRoom = widget.NewButtonWithIcon(w.tr("action.createRoom"), theme.ContentAddIcon(), w.onCreatePressed)
	w.createRoom.Disable()

	roomPanel := container.NewVBox(
		container.NewHBox(
			widget.NewButtonWithIcon(w.tr("action.join"), theme.MediaRecordIcon(), w.onJoinPressed),
			w.createRoom,
		),
		container.NewBorder(nil, nil,
			widget.NewButtonWithIcon(w.tr("action.paste"), theme.ContentPasteIcon(), w.onPasteCodePressed),
			widget.NewButtonWithIcon(w.tr("action.copy"), theme.ContentCopyIcon(), w.onCopyCodePressed),
			w.inviteEntry,
		),
	)

	tracks := container.NewHBox(
		widget.NewLabel(w.tr("track.audio")), w.audioSelect,
		widget.NewLabel(w.tr("track.sub")), w.subSelect,
		widget.NewLabel(w.tr("track.subfile")), w.subFileSelect,
	)

	controls := w.buildControls()

	// The zones, in the order a person meets them: what plays, what it
	// needs, what it is still fetching, and the room — which is about
	// several people and therefore its own block. Everything above it is
	// about one person watching alone.
	//
	// The toolbar and the zones share the top row, and the library is the
	// border's centre: a zone in a VBox row is handed its minimum height
	// and leaves the window empty below, so the one list that grows grows
	// here. Its heading goes with it — a heading over an empty strip tells
	// the reader nothing.
	body := container.NewBorder(
		container.NewVBox(toolbar, contentStack(
			section(w.zoneHeading("zone.playback"), controls),
			section(w.zoneHeading("zone.tracks"), tracks),
			w.preparing,
			section(w.zoneHeading("zone.room"),
				roomPanel,
				w.roomLabel,
				w.connLabel,
			),
		)),
		container.NewVBox(w.events, w.status),
		nil,
		nil,
		// The heading goes in the nested border's top, not in a VBox with the
		// split: a VBox hands its child the minimum height, which left the two
		// lists a thin strip with the window empty below them.
		container.NewBorder(
			w.zoneHeading("zone.library"),
			rule(),
			nil, nil,
			container.NewHSplit(container.NewScroll(w.list), container.NewScroll(w.fileList)),
		),
	)

	w.buffer = room.NewBufferReporter(room.ReporterOptions{
		Report: func(ctx context.Context, torrentID string, offset int64) error {
			var out struct{}
			return w.client.Post(ctx,
				fmt.Sprintf("/api/v1/torrents/%s/buffer/position", torrentID),
				map[string]int64{"position": offset}, &out)
		},
		// The position comes from the same mpv the room commands drive. Until
		// a player exists there is nothing to report, and an error — not a
		// zero — is the honest answer: a zero would tell the prioritiser the
		// film is at its beginning, which is exactly the wrong number to
		// report about a player that cannot be asked (bug-0082).
		Position: func() (float64, error) {
			w.mu.RLock()
			mpv := w.mpv
			w.mu.RUnlock()
			if mpv == nil {
				return 0, errors.New("no player is running")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return mpv.Position(ctx)
		},
	})

	w.bindShortcuts()

	return w, container.NewStack(body)
}

// onJoinPressed joins the room named by the invite code in the field.
func (w *window) onJoinPressed() {
	w.joinRoom(w.inviteEntry.Text)
}

// onPasteCodePressed reads the invitation code pasted into the field.
func (w *window) onPasteCodePressed() {
	if _, err := rtc.ParseInviteCode(w.inviteEntry.Text); err != nil {
		w.fail(err.Error())
		return
	}
	w.notify(w.tr("status.codeRead"))
}

// onCopyCodePressed fills the field with this room's invitation code.
func (w *window) onCopyCodePressed() {
	w.mu.RLock()
	invite := w.invite
	inRoom := w.rtcRoom != nil
	w.mu.RUnlock()
	if !inRoom {
		w.notify(w.tr("status.notIn"))
		return
	}
	onMain(func() { w.inviteEntry.SetText(invite.Encode()) })
}

// ── room ───────────────────────────────────────────────────────────────────

// leaveRoom closes the current room, if any. Joining or creating while in a
// room leaves it first: one window, one room, and the old mesh stops hearing
// the new one's traffic the moment its connections close.
func (w *window) leaveRoom() {
	w.mu.Lock()
	mesh := w.mesh
	w.mesh, w.rtcRoom = nil, nil
	w.roomID = ""
	w.invite = rtc.Invite{}
	w.lastRoster = nil
	w.mu.Unlock()

	if mesh != nil {
		_ = mesh.Close()
	}
	onMain(func() {
		setLine(w.roomLabel, "")
		setLine(w.connLabel, "")
	})
}

// rendezvous is where rooms meet now: the --bootstrap flag, then the saved
// choice, then the public defaults. Read at every join, so a change in the
// settings dialog applies to the next room without a restart.
func (w *window) rendezvous() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return EffectiveBootstrap(w.bootstrapFlag, w.settings.Bootstrap)
}

// fallbackRelays is where room traffic turns when direct paths fail: the
// --relays flag, then the saved choice, then DHT discovery. Read at every
// join, so a change in the settings dialog applies to the next room without
// a restart.
func (w *window) fallbackRelays() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return EffectiveRelays(w.relaysFlag, w.settings.Relays)
}

// joinMesh opens the room mesh for an invite and starts the room driver on
// it. isHost seeds the role: the creator drives, joiners follow.
func (w *window) joinMesh(invite rtc.Invite, isHost bool) (*rtc.Room, error) {
	w.leaveRoom()

	mesh := rtc.NewMesh(rtc.MeshOptions{
		Bootstrap: w.rendezvous(),
		Relays:    w.fallbackRelays(),
		Community: invite.Community,
		Password:  invite.Password,
		Key:       invite.Key,
		// The identity the room shows people by. It must be the same value
		// the room below is given: the greeting is the only thing a peer hears
		// about anyone, and a greeting without a name leaves the roster a list
		// of blank rows (bug-0119).
		UserID: w.clientID,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := mesh.Join(ctx, isHost); err != nil {
		_ = mesh.Close()
		return nil, err
	}

	room := rtc.NewRoom(mesh, mesh.SelfID(), w.clientID, isHost, w.roomCallbacks())
	go room.Run(context.Background())

	w.mu.Lock()
	w.mesh = mesh
	w.rtcRoom = room
	w.roomID = invite.Community
	w.invite = invite
	w.selfPeerID = mesh.SelfID()
	w.mu.Unlock()

	return room, nil
}

// roomCallbacks translates room events into window actions. The driver decides;
// this is where the window acts — players and widgets stay out of rtc.
func (w *window) roomCallbacks() *rtc.Callbacks {
	return &rtc.Callbacks{
		OnRoster: func(peers []rtc.Peer) {
			w.mu.Lock()
			lines := rosterNotices(w.tr, w.lastRoster, peers, w.selfPeerID)
			w.lastRoster = rosterSnapshot(peers)
			w.mu.Unlock()
			onMain(func() {
				setLine(w.roomLabel, w.trf("room.label", strings.Join(describeRoster(peers), ", ")))
			})
			for _, line := range lines {
				w.notify(line)
			}
		},
		OnMagnet: func(m rtc.Magnet) {
			w.adoptRoomTorrent(m)
		},
		OnPause: func(paused bool) {
			w.applyPause(paused)
		},
		OnSeek: func(seconds float64) {
			w.applySeek(seconds)
		},
		OnBuffering: func(peerID string, buffering bool) {
			w.showRoster()
		},
		OnHostChanged: func(hostPeerID string) {
			w.showRoster()
		},
	}
}

// joinRoom enters the room named by an invite code.
//
// No torrent is needed upfront: the guest learns the magnet from the host over
// the mesh and adds it to its own backend, pulling the media from the swarm
// itself.
func (w *window) joinRoom(code string) {
	invite, err := rtc.ParseInviteCode(strings.TrimSpace(code))
	if err != nil {
		w.fail(err.Error())
		return
	}

	go func() {
		if _, err := w.joinMesh(invite, false); err != nil {
			w.fail(w.trf("status.joinFail", err.Error()))
			return
		}
		w.notify(w.tr("status.joined"))
	}()
}

// watchPlayer broadcasts what the local player does, when this peer is the
// host, and reports stalls for any member. It also reconciles the window's
// own playback row with the player, room or no room: the row drives the same
// mpv either window shows, and this loop translates player state into room
// commands regardless of which window was pressed.
//
// Play and pause travel on change; a position travels on a seek jump — and
// every five seconds regardless, so a follower that drifted silently
// converges instead of waiting for the next seek. Guests poll too but never
// broadcast playback: a guest's pause is its own business until the host
// moves.
func (w *window) watchPlayer() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastPaused *bool
	var lastPosition float64
	var lastStalled *bool
	var tick uint64

	for range ticker.C {
		w.mu.RLock()
		mpv := w.mpv
		room := w.rtcRoom
		w.mu.RUnlock()
		if mpv == nil {
			lastPaused, lastPosition, lastStalled = nil, 0, nil
			w.setControlsOn(false)
			continue
		}
		w.setControlsOn(true)
		w.updateControls(mpv)
		if room == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		paused, pausedErr := mpv.Paused(ctx)
		position, positionErr := mpv.Position(ctx)
		stalled, stalledErr := mpv.Stalled(ctx)
		cancel()

		if stalledErr == nil && (lastStalled == nil || *lastStalled != stalled) {
			lastStalled = &stalled
			_ = room.ReportBuffering(context.Background(), stalled)
		}

		if !room.IsHost() {
			lastPaused = boolPtr(paused, pausedErr)
			lastPosition = positionOr(position, positionErr, lastPosition)
			continue
		}

		if pausedErr == nil && (lastPaused == nil || *lastPaused != paused) {
			lastPaused = &paused
			if paused {
				_ = room.Pause(context.Background())
			} else {
				_ = room.Play(context.Background())
			}
		}
		if positionErr == nil {
			tick++
			if hostSeekDue(lastPosition, position, tick) {
				_ = room.Seek(context.Background(), position)
			}
			lastPosition = position
		}
	}
}

func boolPtr(v bool, err error) *bool {
	if err != nil {
		return nil
	}
	return &v
}

func positionOr(v float64, err error, fallback float64) float64 {
	if err != nil {
		return fallback
	}
	return v
}

// heartbeatEvery is the heartbeat in watchPlayer ticks: the loop polls every
// 500 ms, so every 10th tick the host repeats its position. A follower that
// drifted silently converges back through SmoothSeek instead of waiting for
// the host's next seek (idea-0093).
const heartbeatEvery = 10

// hostSeekDue reports whether this host tick sends the position: on a real
// seek jump, or on a heartbeat tick. Heartbeats reuse the ordinary seek path,
// so followers converge with the same smoothing and no new message type. The
// first sample never sends: there is nothing to have drifted from yet.
func hostSeekDue(lastPosition, position float64, tick uint64) bool {
	if lastPosition == 0 {
		return false
	}
	if position-lastPosition > rtc.MaxSeekDrift || lastPosition-position > rtc.MaxSeekDrift {
		return true
	}
	return tick%heartbeatEvery == 0
}

// showRoster renders the membership line.
// adoptRoomTorrent adds the room's torrent to THIS client's backend.
func (w *window) adoptRoomTorrent(ev rtc.Magnet) {
	w.mu.RLock()
	have := w.current != nil && w.current.ID == ev.InfoHash
	w.mu.RUnlock()
	if have {
		return
	}

	w.mu.RLock()
	room := w.rtcRoom
	w.mu.RUnlock()
	hostPeer := ""
	if room != nil {
		hostPeer = room.HostPeer()
	}
	if !shouldAdoptRoomTorrent(ev, w.clientID, hostPeer) {
		return
	}

	w.mu.Lock()
	w.pendingRoomFile = ev.SelectedFile
	w.roomInfohash = ev.InfoHash
	w.mu.Unlock()
	onMain(func() { w.entry.SetText(ev.MagnetURI) })
	w.notify(w.trf("status.roomWatching", ev.Name))
	w.addTorrent()
}

// applyPause drives the local player from the host's play state.
func (w *window) applyPause(paused bool) {
	w.mu.RLock()
	mpv := w.mpv
	w.mu.RUnlock()
	if mpv == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if paused {
			_ = mpv.Pause(ctx)
		} else {
			_ = mpv.Resume(ctx)
		}
	}()
}

// applySeek drives the local player to the host's raw target, smoothed against
// this peer's own lag. Following the requested position puts this client
// seconds ahead of everyone else, silently, because the number looks plausible.
func (w *window) applySeek(seconds float64) {
	w.mu.RLock()
	mpv := w.mpv
	w.mu.RUnlock()
	if mpv == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		current, err := mpv.Position(ctx)
		if err != nil {
			current = 0
		}
		_ = mpv.Seek(ctx, rtc.SmoothSeek(current, seconds))
	}()
}

// showRoster renders the membership line.
func (w *window) showRoster() {
	w.mu.RLock()
	room := w.rtcRoom
	w.mu.RUnlock()
	if room == nil {
		return
	}
	peers := room.Peers()
	onMain(func() {
		setLine(w.roomLabel, w.trf("room.label", strings.Join(describeRoster(peers), ", ")))
	})
}

func (w *window) item(id widget.ListItemID) *Torrent {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if id < 0 || id >= len(w.torrents) {
		return nil
	}
	t := w.torrents[id]
	return &t
}

// torrentIDs is the guest's own holdings, as the adoption package wants them.
func torrentIDs(torrents []Torrent) []string {
	ids := make([]string, 0, len(torrents))
	for _, t := range torrents {
		ids = append(ids, t.ID)
	}
	return ids
}

// torrentByID finds the row the adoption just played, so the window shows the
// list that belongs to it.
func (w *window) torrentByID(id string) *Torrent {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for i := range w.torrents {
		if w.torrents[i].ID == id {
			t := w.torrents[i]
			return &t
		}
	}
	return nil
}

// play starts the torrent in mpv.
//
// mpv runs as its own process and gets the access token as an HTTP header, so
// the media endpoint is an ordinary authenticated route, like every other one.
// (idea-0066: mpv sends X-Access-Token.) It gets the file list first: which
// file to play is the user's
// choice, and a torrent is routinely a season of episodes rather than a film.
func (w *window) play(t *Torrent) {
	if t == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var listed struct {
			Files []fileEntry `json:"files"`
		}
		if err := w.client.Get(ctx, "/api/v1/torrents/"+t.ID+"/files", &listed); err != nil {
			w.fail(w.trf("status.torrentReadFail", err.Error()))
			return
		}
		if len(listed.Files) == 0 {
			w.fail(w.tr("status.torrentNoFiles"))
			return
		}

		w.presentFiles(t, listed.Files)
	}()
}

// presentFiles shows the file list and plays the first video once one is picked.
func (w *window) presentFiles(t *Torrent, files []fileEntry) {
	// Fyne's list draws from the length and update callbacks, so the data is
	// just the slice behind them: swap it, refresh, done.
	w.mu.Lock()
	w.current = t
	w.files = files
	w.mu.Unlock()

	w.syncCreateRoom()
	onMain(w.fileList.Refresh)
	w.notify(w.tr("status.choose"))
}

// playFile selects the file on the data plane and hands the stream to mpv.
func (w *window) playFile(f fileEntry) {
	w.mu.RLock()
	torrent := w.current
	w.mu.RUnlock()
	if torrent == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		var selected struct {
			Index int    `json:"index"`
			Name  string `json:"name"`
		}
		body := map[string]int{"fileIndex": f.Index}
		if err := w.client.Post(ctx, "/api/v1/torrents/"+torrent.ID+"/select", body, &selected); err != nil {
			w.fail(w.trf("status.fileSelectFail", err.Error()))
			return
		}
		w.mu.Lock()
		w.selectedFile = f.Index
		w.mu.Unlock()

		p, err := w.startPlayer()
		if err != nil {
			w.fail(w.trf("status.mpvStartFail", err.Error()))
			return
		}

		if err := p.Play(ctx, w.client.StreamURL(torrent.ID)); err != nil {
			w.fail(w.trf("status.playFail", err.Error()))
			return
		}
		w.notify(w.trf("status.playing", f.Name))
		w.showSubFiles()

		// The prioritiser steers by this report: what is playing, how large it
		// is, and where in it playback stands. Without it the download window
		// stays at byte zero for the whole film, and the middle arrives only
		// by luck (bug-0082).
		w.reportBuffer(torrent.ID, f.Size, p.Duration)

		// The tracks exist as soon as mpv has read the file's header, which is
		// not a number anyone should guess at in code. Poll for them and stop at
		// the first answer, or at the deadline: a film with one voice is the
		// common case, and it has nothing to offer anyway.
		go w.loadTracks(p)
	}()
}

// durationWait bounds the wait for the file's length; durationPoll is how often
// mpv is asked while it reads. The same shape as loadTracks, and for the same
// reason: there is no mpv notification worth wiring up, and the answer appears
// within a few hundred milliseconds of playback starting.
const (
	durationWait = 30 * time.Second
	durationPoll = 250 * time.Millisecond
)

// waitForDuration asks until the player can say how long the file is.
//
// While mpv is still reading the header, a zero and an error both mean "not
// yet" and neither may reach the reporter: the byte offset is a fraction of
// the file, and a guessed fraction aims the download window at the wrong part
// of the film — worse than staying quiet, which is what happens instead.
func waitForDuration(ctx context.Context, read func(context.Context) (float64, error)) (float64, bool) {
	deadline := time.Now().Add(durationWait)
	for time.Now().Before(deadline) {
		readingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		duration, err := read(readingCtx)
		cancel()
		if err == nil && duration > 0 {
			return duration, true
		}

		select {
		case <-ctx.Done():
			return 0, false
		case <-time.After(durationPoll):
		}
	}
	return 0, false
}

// reportBuffer starts reporting the playhead of a file to this application's
// own backend, where the piece prioritiser turns it into a download window.
//
// This is the data plane's only feedback loop, and its absence was invisible
// for exactly the reason the goal warns about: the window starts at zero and
// so does the film, so the first minutes look right either way (bug-0082).
//
// A switch to another file cancels the previous report — the old window must
// stop steering pieces of a film nobody is watching. The poll and the reporting
// loop share one context, so one cancel ends both.
func (w *window) reportBuffer(torrentID string, fileSize int64, readDuration func(context.Context) (float64, error)) {
	ctx, cancel := context.WithCancel(context.Background())

	w.mu.Lock()
	previous := w.bufferCancel
	w.bufferCancel = cancel
	w.mu.Unlock()
	if previous != nil {
		previous()
	}

	go func() {
		duration, ok := waitForDuration(ctx, readDuration)
		if !ok {
			return
		}
		w.buffer.Track(torrentID, fileSize, duration, true)
		w.buffer.Run(ctx, torrentID)
	}()
}

// loadTracks fills the choosers from what mpv knows about the file it just
// opened.
//
// Polling rather than waiting on an event, because mpv's IPC has no "tracks are
// ready" notification worth wiring up and playback is already running when this
// matters. The polling stops early: the first answer is the answer.
func (w *window) loadTracks(p *player.Player) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		audio, audioErr := p.Tracks(ctx, player.TrackAudio)
		subs, subErr := p.Tracks(ctx, player.TrackSub)

		// Either a real list or nothing at all — "mpv does not know yet" is
		// reported as an error, so an empty list is a film with no such tracks.
		if audioErr == nil && subErr == nil {
			w.mu.Lock()
			w.audioTracks, w.subTracks = audio, subs
			w.mu.Unlock()
			w.showTracks()
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// showTracks puts the choosers on screen when there is anything to choose.
func (w *window) showTracks() {
	w.mu.RLock()
	audio, subs := w.audioTracks, w.subTracks
	w.mu.RUnlock()

	if !showTrackChooser(player.TrackAudio, audio) && !showTrackChooser(player.TrackSub, subs) {
		return
	}

	onMain(func() {
		// Setting the row the track is already on would fire OnChanged and send
		// mpv the choice it just made. The handler is detached for the duration,
		// so opening the chooser costs no round trip.
		w.audioSelect.OnChanged = nil
		w.audioSelect.Options = optionLabels(player.TrackAudio, audio, "")
		w.audioSelect.SetSelected(w.selectedTrackLabel(player.TrackAudio))
		w.audioSelect.OnChanged = func(choice string) { w.chooseTrack(player.TrackAudio, choice) }

		w.subSelect.OnChanged = nil
		w.subSelect.Options = optionLabels(player.TrackSub, subs, w.tr("track.off"))
		w.subSelect.SetSelected(w.selectedTrackLabel(player.TrackSub))
		w.subSelect.OnChanged = func(choice string) { w.chooseTrack(player.TrackSub, choice) }

		if showTrackChooser(player.TrackAudio, audio) {
			w.audioSelect.Show()
			w.audioSelect.Refresh()
		} else {
			w.audioSelect.Hide()
		}
		if showTrackChooser(player.TrackSub, subs) {
			w.subSelect.Show()
			w.subSelect.Refresh()
		} else {
			w.subSelect.Hide()
		}
	})
}

// mpvHeaders is what mpv sends on every HTTP fetch of its own: the token
// that guards the backend, and the client id that owns the torrents.
// Without the second header mpv is user "" on the backend, and the ownership
// gate answers 403 to the very films the application added (bug-0089).
// --http-header-fields repeats, so both ride the same option. An empty client
// id sends nothing extra: an anonymously added torrent stays reachable
// anonymously, exactly as before.
func mpvHeaders(token, clientID string) ([]string, error) {
	header, err := player.FormatHeader("X-Access-Token", token)
	if err != nil {
		return nil, err
	}
	headers := []string{header}
	if clientID != "" {
		id, err := player.FormatHeader("X-Client-ID", clientID)
		if err != nil {
			return nil, err
		}
		headers = append(headers, id)
	}
	return headers, nil
}

// startPlayer launches mpv once and reuses it for the rest of the run.
// mpvOptions is how this window asks for a player.
//
// Separate from startPlayer so the settings-to-launcher wiring can be checked
// without launching mpv. That wiring is exactly what went missing once: the
// configured binary sat in the settings file, was written and read back
// faithfully, and no code ever asked for it — so a person who set it got
// "no such file" and a setting that looked like it worked.
func (w *window) mpvOptions() player.Options {
	return player.Options{
		// The configured mpv, if there is one. A person whose mpv is not on
		// PATH — a flatpak, a bundle, a build under /opt — would otherwise get
		// "no such file" and no way to say which binary to use.
		Binary:     w.settings.PlayerBinary,
		SocketPath: w.socketPath,
	}
}

func (w *window) startPlayer() (*player.Player, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.mpv != nil {
		return w.mpv, nil
	}

	headers, err := mpvHeaders(w.client.Token(), w.clientID)
	if err != nil {
		return nil, err
	}

	mpv, err := player.Start(context.Background(), w.mpvOptions(), headers)
	if err != nil {
		return nil, err
	}
	w.mpv = mpv
	return mpv, nil
}

// humanSize formats a byte count for the file list.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit && exp < 3; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func (w *window) addTorrent() {
	magnet := w.entry.Text
	if magnet == "" {
		w.notify(w.tr("status.nothing"))
		return
	}

	// The data plane: the magnet goes into this application's own backend.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		var out Torrent
		body := map[string]string{"magnetUri": magnet}
		if err := w.client.Post(ctx, "/api/v1/torrents", body, &out); err != nil {
			w.fail(w.trf("status.couldNot", err.Error()))
			return
		}
		onMain(func() { w.entry.SetText("") })
		w.notify(w.trf("status.added", out.Name))
		w.refresh()
	}()
}

// poll keeps the list current.
//
// A ticker rather than an answer to each request: a torrent's progress moves on
// its own, and the backend has no event to push it.
func (w *window) poll() {
	w.refresh()
	for range time.NewTicker(2 * time.Second).C {
		w.refresh()
	}
}

func (w *window) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The floor comes first and needs no network: a window dragged below it
	// puts the library back in order before anything else is measured.
	w.ensureMinSize()

	// The connection block updates on the same tick as the lists: DHT
	// readiness and peer arrivals change without roster events, and a second
	// poll keeps the line honest without a second ticker. It runs even when
	// the backend is unreachable — the mesh does not need it.
	w.mu.RLock()
	mesh := w.mesh
	w.mu.RUnlock()
	conn := ""
	if mesh != nil {
		conn = connStatusText(w.tr, mesh.ConnStatus())
	}
	onMain(func() { setLine(w.connLabel, conn) })

	var out struct {
		Torrents []Torrent `json:"torrents"`
	}
	if err := w.client.Get(ctx, "/api/v1/torrents", &out); err != nil {
		w.notify(w.trf("status.unreach", err.Error()))
		return
	}

	w.mu.Lock()
	w.torrents = out.Torrents
	playing := ""
	if w.current != nil {
		playing = w.current.ID
	}
	w.mu.Unlock()

	onMain(w.list.Refresh)
	preparing := preparingText(w.tr, playingTorrent(out.Torrents, playing))
	onMain(func() { setLine(w.preparing, preparing) })
	w.setStatus(w.trf("status.torrents", strconv.Itoa(len(out.Torrents))))

	w.adoptTheRoomsFile()
}

// adoptTheRoomsFile finishes what the room's torrent event started.
//
// The room names the file the host is playing, and the guest's own backend
// receives the magnet — but the two do not meet by themselves. A torrent's file
// list only exists once the metadata has arrived, so the choice cannot be
// applied when the event lands; it has to be applied when the torrent shows up
// in the guest's own list. Until this existed, the guest watched its list gain a
// row and nothing else happen: the host pressed play, and the command arrived at
// a client that had never started a player (bug-0078).
func (w *window) adoptTheRoomsFile() {
	w.mu.RLock()
	pending := w.pendingRoomFile
	adopted := w.adoptedRoomFile
	infohash := w.roomInfohash
	playing := ""
	if w.current != nil {
		playing = w.current.ID
	}
	offer := adopt.Offer{
		RoomInfohash: infohash,
		HaveTorrents: torrentIDs(w.torrents),
		Playing:      playing,
		HostFile:     pending,
		Adopted:      adopted,
	}
	w.mu.RUnlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		var playing struct {
			id     string
			files  []fileEntry
			chosen fileEntry
		}
		outcome, err := adopt.CarryOut(ctx, offer, w.roomFiles, func(id string, fs []fileEntry, chosen fileEntry) {
			playing.id, playing.files, playing.chosen = id, fs, chosen
		})

		switch outcome {
		case adopt.NotYet:
			return
		case adopt.Unreadable:
			// Not the same string the manual path uses: this one happens while
			// nobody has touched anything, and "could not read the torrent"
			// over a room the person just joined reads like their own mistake.
			w.notify(w.trf("status.roomReadFail", err.Error()))
			return
		case adopt.NoSuchFile:
			// The metadata arrived but the file the host named is not there.
			// Saying so beats starting the first file of the release and having
			// the room watch two different things. The name comes from the row
			// in the list, because the offer itself carries only an infohash.
			name := offer.RoomInfohash
			if row := w.torrentByID(offer.RoomInfohash); row != nil {
				name = row.Name
			}
			w.notify(w.trf("status.roomFileMissing", name))
			return
		}

		w.mu.Lock()
		w.adoptedRoomFile = true
		w.mu.Unlock()

		// The row is what the person sees, the list belongs to it, and the
		// player then gets exactly the choice a click would have made.
		row := w.torrentByID(playing.id)
		listed := playing.files
		chosen := playing.chosen
		label := playing.id
		if row != nil {
			label = row.Name
		}
		w.notify(w.trf("status.roomJoining", label))
		onMain(func() {
			w.presentFiles(row, listed)
			w.playFile(chosen)
		})
	}()
}

// roomFiles is the guest's own view of a torrent: the file list on its data
// plane. Pulled out of adoptTheRoomsFile so the same call can be made by a test
// against a real backend.
func (w *window) roomFiles(ctx context.Context, torrentID string) ([]fileEntry, error) {
	var listed struct {
		Files []fileEntry `json:"files"`
	}
	if err := w.client.Get(ctx, "/api/v1/torrents/"+torrentID+"/files", &listed); err != nil {
		return nil, err
	}
	return listed.Files, nil
}

// onMain hands a widget change to Fyne's main thread.
//
// Fyne's widgets belong to the thread that runs the event loop, and almost
// everything here arrives from somewhere else: a poll ticker, an HTTP call, the
// room's SSE stream. Touching a widget from those goroutines is a data race
// that Fyne warns about and then, in the next major release, refuses outright.
// The test driver runs these inline, so tests see the change immediately.
func onMain(fn func()) {
	fyne.Do(fn)
}

func (w *window) setStatus(text string) {
	onMain(func() { w.status.SetText(text) })
}

// notify reports what a human must not miss on the event line, which the
// poll never rewrites. Failures, arrivals and one-shot confirmations go
// here; the two-second counters stay on status.
func (w *window) notify(text string) {
	onMain(func() { w.events.SetText(text) })
}

// fail reports a failure of the person's own action: the event line keeps
// it past the next poll, and a modal dialog interrupts for it. Poll and
// room-background failures stay on the line only — a modal every two
// seconds for an unreachable backend would be the poll shouting, not
// reporting. Without a parent window (tests) the dialog has nowhere to
// hang, and the line is the whole report.
func (w *window) fail(text string) {
	w.notify(text)
	if w.parent == nil {
		return
	}
	parent := w.parent
	message := text
	onMain(func() {
		dialog.NewCustom(w.tr("dialog.error"), w.tr("action.ok"), widget.NewLabel(message), parent).Show()
	})
}

// The icons are Fyne's own. Shipping a parallel set would be a second thing to
// translate and keep in step, for no gain.

// ── decisions, separated from the widgets that show them ──────────────────
//
// These are the choices the window makes about the room. They are plain
// functions so they can be tested without a display: a room event that is never
// applied looks exactly like a room where nothing happened, and that is not a
// failure anything else in the suite would notice.

// playingTorrent finds the playing torrent in a fresh list: the poll replaces
// the slice every two seconds, so the window's pointer goes stale on every
// refresh. A torrent the backend dropped (removed elsewhere) reads as "not
// playing" rather than as last week's download.
func playingTorrent(torrents []Torrent, playingID string) *Torrent {
	if playingID == "" {
		return nil
	}
	for i := range torrents {
		if torrents[i].ID == playingID {
			return &torrents[i]
		}
	}
	return nil
}

// preparingText renders the download-visibility line for the playing torrent:
// name, downloaded and total megabytes, percent and backend status. Empty
// when there is nothing to report — no torrent playing, or the download
// already finished. The numbers come from the 2-second list poll: Size and
// Progress are already in it, so no new endpoints for a few more digits.
func preparingText(tr func(string) string, t *Torrent) string {
	if t == nil || t.Name == "" || t.Progress >= 1 {
		return ""
	}
	size := t.Size
	if size < 0 {
		size = 0
	}
	done := int64(float64(size) * t.Progress)
	if done < 0 {
		done = 0
	}
	if done > size {
		done = size
	}
	peers := t.Peers
	if peers < 0 {
		peers = 0
	}
	pct := int(t.Progress * 100)
	if pct < 0 {
		pct = 0
	}
	return fmt.Sprintf(tr("status.preparing"), t.Name, humanSize(done), humanSize(size), pct, torrentStatusWord(tr, t.Status), peers)
}

// torrentStatusWord translates the backend's English status id for display.
// An unknown id shows as it arrived: a new backend state stays visible
// without a frontend change.
func torrentStatusWord(tr func(string) string, status string) string {
	if word := tr("torrent." + status); word != "" {
		return word
	}
	return status
}

// connStatusText renders the compact connection diagnostics for the room
// window: DHT readiness, the room peer count split into direct and relayed,
// and a short self id. Empty outside a room — nothing joined, nothing to
// report. An unready DHT says so instead of going blank: a missing line would
// hide a broken rendezvous.
func connStatusText(tr func(string) string, st rtc.ConnStatus) string {
	if st.SelfID == "" {
		return ""
	}
	if !st.DHTReady {
		return tr("status.connWaiting")
	}
	direct := 0
	for _, p := range st.Peers {
		if !p.Relayed {
			direct++
		}
	}
	return fmt.Sprintf(tr("status.connReady"), len(st.Peers), direct, len(st.Peers)-direct, shortID(st.SelfID))
}

// shortID keeps an identifier readable: a full peer ID is dozens of base58
// characters and a client id is 32 of hex, and a line needs the fact of
// identity, not its entirety.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// rosterSnapshot records a roster by peer id for the next diff.
func rosterSnapshot(peers []rtc.Peer) map[string]string {
	out := make(map[string]string, len(peers))
	for _, p := range peers {
		out[p.PeerID] = p.UserID
	}
	return out
}

// rosterNotices diffs the previous roster against the new one and renders one
// status line per arrival and departure, arrivals in roster order. The room
// line already shows who is here; these lines say what just changed. The
// first roster after joining is a baseline, not an arrival burst: everyone
// already there is discovered, not joined. Self is silent.
func rosterNotices(tr func(string) string, prev map[string]string, next []rtc.Peer, selfID string) []string {
	if len(prev) == 0 {
		return nil
	}
	var lines []string
	for _, p := range next {
		if p.PeerID == selfID {
			continue
		}
		if _, known := prev[p.PeerID]; !known {
			lines = append(lines, fmt.Sprintf(tr("status.peerJoined"), shortID(p.UserID)))
		}
	}
	inNext := make(map[string]struct{}, len(next))
	for _, p := range next {
		inNext[p.PeerID] = struct{}{}
	}
	var left []string
	for id, user := range prev {
		if id == selfID {
			continue
		}
		if _, still := inNext[id]; !still {
			left = append(left, fmt.Sprintf(tr("status.peerLeft"), shortID(user)))
		}
	}
	sort.Strings(left)
	return append(lines, left...)
}

// describeRoster renders the membership line.
func describeRoster(peers []rtc.Peer) []string {
	labels := make([]string, 0, len(peers))
	for _, peer := range peers {
		// The short form, because the full one is a wall of hex: the roster
		// is a line, and a line of 32-character ids says nothing at a glance.
		label := shortID(peer.UserID)
		if peer.IsHost {
			// The host is an ordinary row with a flag. Saying so is what lets a
			// person understand who is driving playback.
			label += " (host)"
		}
		if peer.Buffering {
			// The room has one shared playback state, so the roster is the only
			// place that can say WHICH peer is stalled.
			label += " — buffering"
		}
		labels = append(labels, label)
	}
	return labels
}

// shouldAdoptRoomTorrent decides whether to add the room's torrent to this
// client's backend.
//
// The room broadcasts its torrent on every join, so the host is offered its own
// film back each time. The host already has it and re-adding would start a fresh
// download of something it is playing.
func shouldAdoptRoomTorrent(ev rtc.Magnet, thisPeer, hostPeer string) bool {
	if ev.MagnetURI == "" || ev.InfoHash == "" {
		// An infohash identifies a torrent without carrying one. Adding by hash
		// alone builds a magnet with no trackers: it resolves over DHT and
		// loses every seeder.
		return false
	}
	return thisPeer != hostPeer
}

// The guest's half of a room lives in internal/adopt, so that the whole path —
// decision, file list, choice, hand-off to the player — can be driven by a
// scenario against a real backend instead of only by a running application.
// That is the shape the defect had: the wiring lived inside window, which is
// built from widgets, so nothing but a live run ever executed it (bug-0078).
type (
	roomOffer = adopt.Offer
	fileEntry = adopt.File
)

// roomOffer is everything an adoption decision is made from: the guest's own
// torrent list, what the room is watching, what the guest is playing right now,
// the file the host named, and whether it has been applied already.
//
// It is a value rather than four arguments because every one of those is a
// reason to answer "not yet" — and a decision with five inputs, passed one by
// one, is how a field ends up written and never read (bug-0078).
// roomOutcome is why carrying out a room offer stopped where it did.
//
// It is reported instead of a bool because "nothing happened" and "something went
// tr renders a catalogue string in the window's language.
//
// The source language is English, so the source string is the identity: a
// missing translation falls back to readable text rather than to a key.
func (w *window) tr(key string) string {
	return catalog.Lookup(w.language, key)
}

// trf renders a catalogue string with one substitution.
//
// The verbs are part of the translation, not an afterthought: "%d torrents" and
// "торрентов: %d" are not the same sentence rearranged.
func (w *window) trf(key, arg string) string {
	return fmt.Sprintf(w.tr(key), arg)
}
