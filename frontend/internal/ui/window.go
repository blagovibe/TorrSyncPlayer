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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/adopt"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/catalog"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/player"
	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/room"
)

// Torrent is one row of the list, as the data plane reports it.
type Torrent struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
}

// window is the top-level window.
type window struct {
	client   *api.Client
	clientID string
	settings Settings
	store    *SettingsStore

	list      *widget.List
	fileList  *widget.List
	status    *widget.Label
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

	// roomInfohash is the torrent the room is watching, and adoptedRoomFile
	// records that the guest has already applied the host's choice. Both exist
	// so that the adoption happens once, when the torrent appears in the
	// guest's own list, and not again on every two-second poll tick
	// (bug-0078). Before they existed the guest joined a room and nothing
	// played.
	roomInfohash    string
	adoptedRoomFile bool

	// Room state. The roster and the session live in the room package because
	// their rules are subtle; here they are only driven.
	roster    room.Roster
	stream    *room.Client
	session   *room.Session
	room      string
	roomLabel *widget.Label
	buffer    *room.BufferReporter

	inviteEntry  *widget.Entry
	roomIDField  *widget.Entry
	roomPassword *widget.Entry
	createRoom   *widget.Button

	// Track choosers. mpv knows every voice and every subtitle track in the
	// file; until this existed a person who needed the dub had no way to reach
	// it from the application (idea-0080).
	audioSelect *widget.Select
	subSelect   *widget.Select
	audioTracks []player.Track
	subTracks   []player.Track
}

// NewWindow builds the main window around a client.
func NewWindow(client *api.Client, clientID string, store *SettingsStore) fyne.CanvasObject {
	w := &window{
		client:          client,
		clientID:        clientID,
		store:           store,
		socketPath:      filepath.Join(os.TempDir(), fmt.Sprintf("torrsync-%d.sock", os.Getpid())),
		status:          widget.NewLabelWithStyle("Starting…", fyne.TextAlignLeading, fyne.TextStyle{}),
		pendingRoomFile: -1,
	}
	if store != nil {
		w.settings = store.Load()
	}
	w.language = w.settings.Language

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

	toolbar := container.NewHBox(
		w.entry,
		w.addButton,
		widget.NewButtonWithIcon(w.tr("action.refresh"), theme.ViewRefreshIcon(), w.refresh),
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
	w.audioSelect = widget.NewSelect(nil, nil)
	w.audioSelect.OnChanged = func(choice string) { w.chooseTrack(player.TrackAudio, choice) }
	w.subSelect.OnChanged = func(choice string) { w.chooseTrack(player.TrackSub, choice) }
	w.audioSelect.Hide()
	w.subSelect.Hide()

	go w.poll()

	// The room line sits above the lists: who is watching with you is a
	// one-line answer that belongs where it is always visible.
	w.roomLabel = widget.NewLabel("")

	w.roomIDField = widget.NewEntry()
	w.roomIDField.SetPlaceHolder(w.tr("field.roomID"))
	// A password field, not a plain entry: an invitation screen is exactly where
	// a password gets typed in public.
	w.roomPassword = widget.NewPasswordEntry()
	w.roomPassword.SetPlaceHolder(w.tr("field.roomPassword"))

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
			w.roomIDField,
			w.roomPassword,
			widget.NewButtonWithIcon(w.tr("action.join"), theme.MediaRecordIcon(), w.onJoinPressed),
			w.createRoom,
		),
		container.NewHBox(
			w.inviteEntry,
			widget.NewButtonWithIcon(w.tr("action.paste"), theme.ContentPasteIcon(), w.onPasteCodePressed),
			widget.NewButtonWithIcon(w.tr("action.copy"), theme.ContentCopyIcon(), w.onCopyCodePressed),
		),
	)

	tracks := container.NewHBox(
		widget.NewLabel(w.tr("track.audio")), w.audioSelect,
		widget.NewLabel(w.tr("track.sub")), w.subSelect,
	)

	lists := container.NewHSplit(container.NewScroll(w.list), container.NewScroll(w.fileList))
	body := container.NewBorder(
		toolbar, w.status, nil, nil,
		container.NewVBox(w.roomLabel, roomPanel, tracks, lists),
	)

	w.buffer = room.NewBufferReporter(room.ReporterOptions{
		Report: func(ctx context.Context, torrentID string, offset int64) error {
			var out struct{}
			return w.client.Post(ctx, api.Data,
				fmt.Sprintf("/api/v1/torrents/%s/buffer/position", torrentID),
				map[string]int64{"position": offset}, &out)
		},
	})

	return container.NewStack(body)
}

// onJoinPressed joins the room named in the fields.
func (w *window) onJoinPressed() {
	w.joinRoom(w.roomIDField.Text, w.roomPassword.Text)
}

// onPasteCodePressed fills the fields from a pasted invitation code.
func (w *window) onPasteCodePressed() {
	code, err := room.ParseInviteCode(w.inviteEntry.Text)
	if err != nil {
		w.setStatus(err.Error())
		return
	}
	w.roomIDField.SetText(code.RoomID)
	w.roomPassword.SetText(code.Password)
	w.setStatus(w.tr("status.codeRead"))
}

// onCopyCodePressed fills the field with this room's invitation code.
func (w *window) onCopyCodePressed() {
	id := w.currentRoom()
	if id == "" {
		w.setStatus(w.tr("status.notIn"))
		return
	}
	w.inviteEntry.SetText(room.InviteCode{
		RoomID:   id,
		Password: w.roomPassword.Text,
	}.Encode())
}

// currentRoom is the room this client is in, or an empty string.
func (w *window) currentRoom() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.room
}

// ── room ───────────────────────────────────────────────────────────────────

// joinRoom enters a room on the control plane.
//
// The room is bound to a torrent and the server refuses a mismatch, so this
// cannot succeed without one. That is deliberate: a room with nothing to watch
// is a room with nothing to do.
func (w *window) joinRoom(roomID, password string) {
	if roomID == "" {
		w.setStatus("A room id is required")
		return
	}

	w.mu.RLock()
	torrentID := ""
	if w.current != nil {
		torrentID = w.current.ID
	}
	w.mu.RUnlock()

	if !canJoinRoom(torrentID, roomID) {
		// A room is bound to a torrent and the server refuses a mismatch.
		if torrentID == "" {
			w.setStatus(w.tr("status.pick"))
		} else {
			w.setStatus(w.tr("status.badRoom"))
		}
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		session := room.NewSession(nil, w.client.ControlURL(), w.client.ControlToken(), w.clientID)
		if err := session.Join(ctx, roomID, password, torrentID); err != nil {
			w.setStatus("Could not join: " + err.Error())
			return
		}

		w.mu.Lock()
		w.room = roomID
		w.session = session
		w.mu.Unlock()

		w.subscribe(roomID)
		w.setStatus(w.tr("status.joined"))
	}()
}

// tellRoom sends a playback command to the room.
//
// Only the host's commands change anything — the server refuses the rest — but
// every participant sends, so that "I pressed play" and "the room is playing"
// do not become different beliefs.
func (w *window) tellRoom(send func(ctx context.Context, s *room.Session) error) {
	w.mu.RLock()
	session := w.session
	w.mu.RUnlock()
	if session == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := send(ctx, session); err != nil {
			w.setStatus("The room refused: " + err.Error())
		}
	}()
}

// playRoom asks the room to start.
func (w *window) playRoom() {
	w.tellRoom(func(ctx context.Context, s *room.Session) error { return s.Play(ctx) })
}

// pauseRoom asks the room to stop.
func (w *window) pauseRoom() {
	w.tellRoom(func(ctx context.Context, s *room.Session) error { return s.Pause(ctx) })
}

// reportBuffering tells the room this peer stalled, or caught up.
//
// Any peer may, not only the host: otherwise a participant whose stream
// recovered could not say so and the room would stay paused for everyone.
func (w *window) reportBuffering(buffering bool) {
	w.tellRoom(func(ctx context.Context, s *room.Session) error {
		return s.ReportBuffering(ctx, buffering)
	})
}

func (w *window) subscribe(roomID string) {
	stream := room.New(room.Options{
		BaseURL:  w.client.ControlURL(),
		Token:    w.client.ControlToken(),
		ClientID: w.clientID,
	}, w.onRoomEvent)

	w.mu.Lock()
	w.stream = stream
	w.mu.Unlock()

	stream.Subscribe(context.Background(), roomID)
}

// onRoomEvent is where the room stream lands.
//
// The torrent event is the important one: it is how a guest learns what to
// download, because it pulls the media from the swarm itself.
func (w *window) onRoomEvent(e room.Event) {
	switch e.Type {
	case room.EventPeerList:
		list, seq, err := room.DecodePeerList(e.Data)
		if err != nil {
			return
		}
		if w.roster.Apply(list, seq) {
			w.showRoster()
		}

	case room.EventTorrent:
		var ev room.TorrentEvent
		if err := json.Unmarshal(e.Data, &ev); err != nil || ev.MagnetURI == "" {
			return
		}
		w.adoptRoomTorrent(ev)

	case room.EventSync:
		var ev room.SyncEvent
		if err := json.Unmarshal(e.Data, &ev); err != nil {
			return
		}
		w.applySync(ev)
	}
}

func (w *window) showRoster() {
	onMain(func() {
		w.roomLabel.SetText(w.trf("room.label", strings.Join(describeRoster(w.roster.Peers()), ", ")))
	})
}

// adoptRoomTorrent adds the room's torrent to THIS client's backend.
func (w *window) adoptRoomTorrent(ev room.TorrentEvent) {
	w.mu.RLock()
	have := w.current != nil && w.current.ID == ev.InfoHash
	w.mu.RUnlock()
	if have {
		return
	}

	hostPeer := ""
	if _, isHost := w.roster.FindHost(); isHost {
		for _, peer := range w.roster.Peers() {
			if peer.IsHost {
				hostPeer = peer.UserID
			}
		}
	}
	if !shouldAdoptRoomTorrent(ev, w.clientID, hostPeer) {
		return
	}

	w.mu.Lock()
	w.pendingRoomFile = ev.SelectedFile
	w.roomInfohash = ev.InfoHash
	w.mu.Unlock()
	onMain(func() { w.entry.SetText(ev.MagnetURI) })
	w.setStatus("The room is watching " + ev.Name + " — adding it to your backend")
	w.addTorrent()
}

func (w *window) applySync(ev room.SyncEvent) {
	w.mu.RLock()
	mpv := w.mpv
	w.mu.RUnlock()
	if mpv == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		switch ev.Action {
		case "play":
			_ = mpv.Resume(ctx)
		case "pause":
			_ = mpv.Pause(ctx)
		case "seek":
			_ = mpv.Seek(ctx, positionFor(ev))
		}
	}()
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
		if err := w.client.Get(ctx, api.Data, "/api/v1/torrents/"+t.ID+"/files", &listed); err != nil {
			w.setStatus("Could not read the torrent: " + err.Error())
			return
		}
		if len(listed.Files) == 0 {
			w.setStatus("The torrent has no files yet")
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
	w.setStatus(w.tr("status.choose"))
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
		if err := w.client.Post(ctx, api.Data, "/api/v1/torrents/"+torrent.ID+"/select", body, &selected); err != nil {
			w.setStatus("Could not select the file: " + err.Error())
			return
		}

		p, err := w.startPlayer()
		if err != nil {
			w.setStatus("Could not start mpv: " + err.Error())
			return
		}

		if err := p.Play(ctx, w.client.StreamURL(torrent.ID)); err != nil {
			w.setStatus("Could not play: " + err.Error())
			return
		}
		w.setStatus(w.trf("status.playing", f.Name))

		// The tracks exist as soon as mpv has read the file's header, which is
		// not a number anyone should guess at in code. Poll for them and stop at
		// the first answer, or at the deadline: a film with one voice is the
		// common case, and it has nothing to offer anyway.
		go w.loadTracks(p)
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

	header, err := player.FormatHeader("X-Access-Token", w.client.DataToken())
	if err != nil {
		return nil, err
	}

	mpv, err := player.Start(context.Background(), w.mpvOptions(), []string{header})
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
		w.setStatus(w.tr("status.nothing"))
		return
	}

	// The data plane: the magnet goes into this application's own backend.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		var out Torrent
		body := map[string]string{"magnetUri": magnet}
		if err := w.client.Post(ctx, api.Data, "/api/v1/torrents", body, &out); err != nil {
			w.setStatus(w.trf("status.couldNot", err.Error()))
			return
		}
		onMain(func() { w.entry.SetText("") })
		w.setStatus(w.trf("status.added", out.Name))
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

	var out struct {
		Torrents []Torrent `json:"torrents"`
	}
	if err := w.client.Get(ctx, api.Data, "/api/v1/torrents", &out); err != nil {
		w.setStatus(w.trf("status.unreach", err.Error()))
		return
	}

	w.mu.Lock()
	w.torrents = out.Torrents
	w.mu.Unlock()

	onMain(w.list.Refresh)
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
			w.setStatus(w.trf("status.roomReadFail", err.Error()))
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
			w.setStatus(w.trf("status.roomFileMissing", name))
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
		w.setStatus(w.trf("status.roomJoining", label))
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
	if err := w.client.Get(ctx, api.Data, "/api/v1/torrents/"+torrentID+"/files", &listed); err != nil {
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

// The icons are Fyne's own. Shipping a parallel set would be a second thing to
// translate and keep in step, for no gain.

// ── decisions, separated from the widgets that show them ──────────────────
//
// These are the choices the window makes about the room. They are plain
// functions so they can be tested without a display: a room event that is never
// applied looks exactly like a room where nothing happened, and that is not a
// failure anything else in the suite would notice.

// describeRoster renders the membership line.
func describeRoster(peers []room.Peer) []string {
	labels := make([]string, 0, len(peers))
	for _, peer := range peers {
		label := peer.UserID
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

// positionFor is where a client should actually go after a seek.
//
// The server smooths a jump, so the position asked for and the position the
// room moved to differ on purpose. Following the requested one puts this client
// seconds ahead of everyone else, silently, because the number looks plausible.
// A room that has not moved yet reports zero, and zero is not a place to go.
func positionFor(ev room.SyncEvent) float64 {
	if ev.Status != nil && ev.Status.Position > 0 {
		return ev.Status.Position
	}
	return ev.Position
}

// shouldAdoptRoomTorrent decides whether to add the room's torrent to this
// client's backend.
//
// The room broadcasts its torrent on every join, so the host is offered its own
// film back each time. The host already has it and re-adding would start a fresh
// download of something it is playing.
func shouldAdoptRoomTorrent(ev room.TorrentEvent, thisPeer, hostPeer string) bool {
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

// canJoinRoom refuses a join with no torrent to watch.
func canJoinRoom(torrentID, roomID string) bool {
	if torrentID == "" {
		return false
	}
	return room.ValidateRoomID(roomID) == nil
}

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
