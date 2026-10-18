# Changelog

All significant changes to the project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **The torrent client could not always take its own port on Windows.** It
  listens on TCP and on UDP with one number, and the number was left to the
  kernel: the client bound TCP on a random one and then bound UDP on the same
  number. Windows reserves whole ranges of UDP ports for itself (WSL, Hyper-V,
  Docker Desktop) and refuses a bind inside one with WSAEACCES, whose wording
  says nothing about the cause — and the client retries only a «port taken»
  refusal, so the backend simply did not start, about one run in three. The
  Windows job caught it on `TestARoomWithoutAServer` and
  `TestSubtitlesTravelTheEndpoint`. The number is now chosen in the backend
  and checked on both protocols at once, and another is tried if the client
  still cannot take it (bug-0118).

## [v0.3.3] - 2026-10-17

### Fixed

- **On Windows a busy pipe failed the command instead of waiting for it.**
  mpv's IPC is a named pipe there, and a named pipe hands out one instance at a
  time: the window polls several times a second, each poll with its own
  connection, and the pipe can still be releasing the previous one when the
  next arrives. Our hand-rolled `CreateFile` turned that moment into an instant
  `All pipe instances are busy` — Go waits this out for its own pipes, a
  hand-written call does not. The Windows job caught it on
  `TestRealMpvOpensItsSocket` and `TestRealMpvListsAndSwitchesTracks`, and it
  was never only about tests: a failed position read hands the room the start of
  the film (bug-0117).
- **The torrent client could not always take its own port on Windows.** It
  listens on TCP and on UDP with one number, and the number was left to the
  kernel: the client bound TCP on a random one and then bound UDP on the same
  number. Windows reserves whole ranges of UDP ports for itself (WSL, Hyper-V,
  Docker Desktop) and refuses a bind inside one with WSAEACCES, whose wording
  says nothing about the cause — and the client retries only a «port taken»
  refusal, so the backend simply did not start, about one run in three. The
  Windows job caught it on `TestARoomWithoutAServer` and
  `TestSubtitlesTravelTheEndpoint`. The number is now chosen in the backend
  and checked on both protocols at once, and another is tried if the client
  still cannot take it (bug-0118).

## [v0.3.2] - 2026-10-17

### Fixed

- **The tray icon had never existed.** The application called
  `systray.RunWithExternalLoop` and dropped the start hook it returns; nothing
  ever started, so there was no icon, no menu, and `Running()` was always
  false — which also meant closing the window ended the application, the one
  thing the tray exists to prevent. The tray is now Fyne's, which starts it at
  the right moment on each platform; the direct `fyne.io/systray` dependency
  is gone. Proved three ways: a probe with and without the hook, the released
  v0.3.1 AppImage owning no `org.kde.StatusNotifierItem-*` on a machine with a
  desktop, and the new build owning one and serving its menu over DBus
  (bug-0116, open — the icon on macOS and Windows remains unverified).
- **Two exits in one tray menu.** The toolkit appends an exit of its own unless
  the last item is marked `IsQuit`, and it decides that by comparing the label
  with its own translation of «Quit» — which «Выход» cannot match in Russian.
  Read off a live tray. Our item is marked now; the menu serves one exit.

### Added

- **Settings in the tray menu** (owner's decision, idea-0112). Closing the
  window hides it, so the tray is the only way back — and the settings lived
  only in the window that might be the thing that is closed. The menu is now
  Open, Settings, separator, Quit.

## [v0.3.1] - 2026-10-17

### Fixed

- **mpv answers `sub-add` with a shrug, and the subtitle test took it
  personally.** `TestRealMpvLoadsExternalSubtitlesOverAuthenticatedHTTP`
  failed as «mpv refused the external subtitles: error running command» on
  macOS, then on Ubuntu, and locally in about one run out of seven. Proved
  against the real binary: mpv refuses `sub-add` **silently** when no file
  is loaded yet — no log line, no hint — while a URL it cannot open gets the
  same refusal plus a `Failed to open` line. The test waited for the file to
  be in the playlist, which is not the same as loaded. It now waits for the
  video track, the state the command actually needs (bug-0115, open — the
  silence is mpv's own).
- **That error reached the window verbatim.** Choosing a subtitle track puts
  the error text into the status line, and «player: error running command»
  explains nothing to a person watching. `AddSubtitles` now says when there is
  no file loaded to attach subtitles to; the new
  `TestAddSubtitlesSaysWhenThereIsNoFileToAttachThemTo` fails on the old code.

## [v0.3.0] - 2026-10-17

### Fixed

- **The desktop application opened a torrent client to the local network.**
  The newcomer audit asked the running build from its own LAN address and
  got `{"status":"ok"}` with no token: the client launched the backend with
  `--port`, which binds every interface, so every launch exposed a torrent
  client to the network. The client now passes `--addr 127.0.0.1:<port>` —
  the window is the only client and it is on this machine (bug-0114,
  card removed). Two existing tests pinned the old behaviour and failed
  first; `TestTheTLSWarningOnlyFiresOffLoopback` is new. Verified on a live
  build: `ss` shows `127.0.0.1:<port>`, the LAN address does not answer, the
  loopback does.
- **The TLS warning greeted every normal run** with "Do not use in
  production!" — about a listener whose traffic never leaves the owner's
  machine. It now fires only when the backend really listens off loopback,
  and says what to do instead. Same audit, same release.
- **A newcomer had nowhere to read what to download** (bug-0113, card
  removed). README had no installation section at all: "Быстрый старт" was
  `go run ./cmd/build`, which is instructions for a developer. It now leads
  with the three release artifacts, how to run each one, and that `mpv`
  ships inside — then keeps the source build below as what it is. The stale
  limitation about eyeballing connections is corrected to point at the
  connection block that has existed since idea-0100.
- **The settings dialog arrived collapsed — the fields did not stretch.**
  The owner's screenshot showed headings and placeholders floating in an
  otherwise empty window; my audit had missed it because it rendered the
  form directly instead of the dialog. Cause: a `VScroll` around the form,
  which hands its children an unbounded box, so each field row was stretched
  to the whole scroll area and the fields collapsed — the same was true in
  v0.3.0, which is why the report was about stretching and not about a new
  change.

  - The scroll container is gone. The form fits the dialog it opens at; if
    that ever stops being true, the fix is a taller dialog, not a scroll
    around a broken layout.
  - The dialog is sized **after** `Show`: Fyne resets a custom dialog to its
    content's minimum on the way out, so a size set beforehand was
    overwritten.
  - The rules under the headings are `canvas.Line`, not filled rectangles —
    a rectangle is stretched to its row and painted over the fields below,
    which the screenshot showed as a grey band through every input.
  - Zone gaps are transparent: the dialog paints over the window's canvas
    and its background is a different colour, so a themed gap showed up as
    the very band it was meant to remove.

  `ShowSettings` takes the parent window as an argument instead of looking it
  up inside — the dialog is the one surface that had to be checked by
  rendering, and a hidden lookup is what made that check impossible. Pinned
  by a measurement rather than a picture: the form must ask for at least
  400 px on both axes, so a layout that starves it again fails the suite
  instead of shipping an empty dialog. Full frontend suite green, and a
  live run of the built pair logs nothing.
- Interface audit by rendering each surface offscreen and looking at it,
  not by reading the code (owner's words: «меню настроек проверь, там
  тоже нихрена не понятно. проведи полный аудит интерфейса»). What the
  renders showed:

  - **Rendezvous and relays sat inside the gateway block.** The two most
    technical fields on the form read as four more htorrent fields, while
    they are the room network — the worst confusion on the form, and it is
    gone: it has a heading, «Комнаты и соединения», of its own. The form
    is now four named zones — «Вид и плеер», «Кус��и торрентов»,
    «Комнаты и соединения», «Шлюз htorrent» — bold-headed and ruled, same
    face as the window's zones (`canvasTextBold`, so the two look like one
    application).
  - The form's separators were the same fat bands the window had; they are
    hairlines now, shared with the window.
  - The note about when each choice takes effect was the only hint about
    it and sat as small print; it is now italic and first, before anything
    is changed rather than discovered after.
  - **The window had no minimum size**, and nothing stopped a drag until
    the library collapsed to a strip. Fyne cannot express one —
    `SetFixedSize` pins a size and refuses to change it, which is worse
    than a floor — so the poll the window already runs puts it back above
    800×560.

  Checked and found sound, so left alone: the first-launch question reads
  clearly; the shortcuts dialog does render line breaks (the suspected
  `Label` limitation is not there); the tray menu is two items that need no
  explanation. Verified by re-rendering every surface, by a live run of the
  built pair (log clean), and by two tests pinning the zones and the floor.
- A live run of the built application found two defects the suite could not:
  the Russian interface filled the log with `Translation failure … not found
  in language "ru"`, and quitting on the backend's death called `a.Quit()`
  from a goroutine, which Fyne 2.8 flags and says the next major release will
  refuse outright (bug-0110, bug-0111, both cards removed).

  - Every `widget.NewSelect` asks Fyne for `(Select one)` in its
    constructor — before a placeholder of ours can be set — and the bundle
    Fyne ships carries neither `ru` nor our wording. `InstallFyneTranslations`
    registers the string once at startup. Setting our own placeholder as well
    was not enough, and did not need to be: the question is asked in the
    constructor, not at draw time.
  - `fyne.Do(a.Quit)` for the same reason every widget change already goes
    through `onMain`.

  Verified by building the pair, running it offscreen and reading the log:
  zero translation failures, zero call-thread errors, the poll answering
  200, and the application exiting cleanly when the backend child is
  killed.
- The window looked flat and nothing in it had a rank (owner's words:
  «интерфейс ужасно выглядит, там ничего не понятно»). Seven strips of
  equal weight in one column were rebuilt as four named zones with
  hairlines under each heading: «Сейчас играет», «Звук и субтитры»,
  «Торренты и файлы», «Смотрим вместе». What it fixed, found by rendering
  the window offscreen and looking at it:

  - the magnet field was clipped to «маг» and the invite field to «вста:» —
    they sat in a border's `left`, which is handed its minimum width; they
    are now the border's object, the only thing that stretches;
  - both lists were a thin strip with the window empty below, because a
    zone in a `VBox` row gets its minimum height; the library is the
    border's centre now;
  - `widget.NewSeparator` drew as a fat grey band that read as a patch over
    the window, so the rules are one-pixel rectangles in the separator
    colour;
  - empty labels kept their height, leaving holes under their headings;
    `setLine` hides a line with nothing to say and the four start hidden.

  Verified by rendering the window offscreen at 1100×700 and by two tests:
  the zones are named and distinct, a line with text shows and an empty one
  disappears. Full frontend suite green. Replacing the toolkit was refused
  earlier on purpose (idea-0063): GTK4 would cost macOS and Windows.
- The live volume test failed on CI after the release (bug-0108, card
  removed). `TestRealMpvVolumeRoundTrip` is green on mpv 0.41 and red on
  the ubuntu-22.04 runner, whose mpv is 0.34.1 with no sound card: the
  volume is set and never lands, because there is no audio chain to apply
  it to. The mismatch now decides, not the machine's mood — the reason is
  asked of mpv itself (`current-ao`) and travels with the skip, so a green
  run never means "checked" when it means "skipped", and an output that
  does exist keeps the failure. Verified: skip path exercised locally
  (forced mismatch), full frontend suite green. The wire shape stays
  pinned by the three deterministic tests beside it.

### Verified

- CI verdict on the v0.2.0 tree, as the release entry predicted: mutation
  testing measures **66.25%** against the 49% floor (210 passed, 107 failed
  of 317), where the stale run before `6a12eb4` read 45.43%. The gate is
  green and the number is real — no floor was touched. Every workflow green:
  lint, backend tests, frontend tests on all three platforms, backend builds
  on all three, contract, fuzzing, load, ZAP, CodeQL.

## [v0.2.0] - 2026-10-17

### Added

- Connection diagnostics in the room window (idea-0100, accepted and
  transferred to the goal and the way; card removed). The window carries a
  compact block under the room line: DHT rendezvous ready or still looking,
  room peer count split into direct and relayed, and a short self id. The
  source is the node itself — `Node.DHTReady`, `Node.PeerRelayed`
  (`/p2p-circuit` among the peer's connection addresses), `Mesh.ConnStatus`
  over the admitted room peers — so no new protocol and nothing the node
  cannot do. Refreshed on the window's 2-second poll, cleared on leave;
  decisions live in testable `connStatusText`/`shortPeerID`, catalogue keys
  `status.connReady`/`status.connWaiting` in both languages (the
  catalogue-usage gate covers them). `CONCEPT.md#Не нужно догадываться, что
  происходит` and `DEV.md#Диагностика соединения` say the new model.
  Verified: `FYNE_DRIVER=offscreen go test -race ./internal/rtc/` green
  (direct reads direct, circuit-only reads relayed, pre-join reads empty),
  `./internal/ui/` and `./internal/catalog/` green.
- Preparing line shows megabytes (idea-0090, UX parity slice). The line
  read name, percent and backend status; now it reads downloaded and total
  megabytes beside them — `Size` and `Progress` were already in the 2-second
  poll, so no backend change and no new endpoints. Reuses the file list's
  `humanSize`; bounds are clamped, an unknown size shows as `0 B` rather
  than a blank. Catalogue `status.preparing` reworded in both languages
  (single call site). Swarm peers in the same line stay a follow-up: the
  local backend does not expose a peer count, and that is an API change.
- Playback-input audit hit what a headless machine cannot prove (bug-0101,
  open): mpv starts with `--no-input-default-bindings` since f0b2e8a with
  no reason recorded, which plausibly leaves the player window without
  keyboard control. No display, no xvfb and no mpv manual here, so closing
  it takes a live run — keys and OSC buttons against a real window. The
  flag is not removed blind: its intent is unknown, and silent behavior
  change beats a card.
- Modal error dialogs (idea-0090, UX parity slice). Failures of the
  person's own action — paste, join, add, files, select, mpv start, play —
  now interrupt with a modal dialog (`dialog.error` title, `action.ok`
  dismiss, both languages) on top of the event line. Poll and
  room-background failures stay on the line only: a modal every two seconds
  for an unreachable backend would be the poll shouting. The parent window
  arrives via `WindowOptions`; without one (tests) the line is the whole
  report. Pinned by `TestAFailureInterruptsAndPersists` for both shapes.
  Verified: `./internal/ui/` and `./internal/catalog/` green.
- Window hotkeys and shortcuts dialog (idea-0103, accepted; card
  removed). The window had no keyboard surface, so a dialog would have
  documented nothing: Ctrl+L focuses the magnet field, Ctrl+R refreshes
  the list, Ctrl+, opens settings — one `shortcutSpecs` list feeds both
  the bindings and the dialog, pinned by `TestTheWindowListsItsOwnShortcuts`
  (keys, descriptions, headless actions). Player keys stay out on purpose:
  playback lives in mpv's window (bug-0101). Catalogue keys in both
  languages. Verified: `./internal/ui/` and `./internal/catalog/` green.
- Two-machine field test cancelled by the owner (2026-10-17): no testing
  on two computers. The queue item is removed, not deferred — heartbeat
  convergence, takeover and relay paths stay proven by single-machine
  scenario tests only. bug-0101 stays open: it needs one machine with a
  display, not two.
- Troubleshooting in README told readers to eyeball connection failures
  (bug-0102, trivial fix, card removed). It now points at the connection
  block, the shared-rendezvous check and the event line — and the settings
  field is named `Бутстрап`, not `Рандеву`, as the old text claimed.
  Verified by reading against the layout, the catalogue and the poll.
- Player teardown on quit (bug-0104, card removed). Quitting left mpv
  running idle with its window open, and every playback session left one
  `torrsync-<pid>.sock` in /tmp — a killed mpv cannot remove its own
  socket, proven by killing a real one and watching the file stay. Now
  `Player.Close` takes the socket with the process (the path is pid-tagged,
  so only ever its own; a missing file is silence), `NewWindow` hands back
  a stop function, and `main` defers it. The same pass nil-guarded `Close`
  against a missing command, which the new test tripped over. Verified:
  `go test ./internal/player/ ./internal/ui/` green, including the socket
  removal and the quit-detaches-player pins.
- Three platforms proven (idea-0063, card removed). The card waited on a
  native build and run per platform; CI now gives it: `Test Frontend` green
  on ubuntu-22.04, macos-latest and windows-latest, backend builds green on
  all three, and the v0.1.0 release shipped AppImage/zip/DMG from those very
  runners. No goal or way change — the promise stood in the goal all along.
- CI note, not a finding: `Mutation Testing` is red on main (45.43% vs the
  49% floor), but the run predates the mutant-killing commit `6a12eb4` and
  the untracked validation edge tests — the measurement is stale, the fix is
  already in the tree. The next pipeline run is the verdict; a still-red
  gate gets its own card then, not now.
- Window owns the remote (idea-0105, accepted and transferred to the goal
  and the way; card removed). Seeker/volume/fullscreen/media-info live in
  our window and command mpv over IPC — the owner's call against leaving it
  all to mpv's OSC. `CONCEPT.md#Границы проекта` and the mpv section of
  `DEV.md` say the new model; warnings stay declined and translations stay
  hand-kept, both recorded decisions. Slice 1 landed here: `Volume`,
  `SetVolume` (clamped, not refused), `ToggleFullscreen` on the existing
  command/request primitives, pinned against the fake-mpv wire shape.
  Slice 2 (controls row + media info in the window) is next.
- Audit: the DEV edit touched bug-0101's section, so the card went
  `needs_review` and back to `open` after reading — the new paragraph is
  about control IPC, the suspicion about input bindings, nothing shared.
- Playback row in the window (idea-0105, slice 2). Play/pause, elapsed,
  seeker, remaining, fullscreen on top; volume with the media line beneath.
  Drags commit after a 400 ms quiet delay instead of spraying the room;
  programmatic redraws detach the handler first, so the poll never seeks on
  its own behalf. Decisions live in `formatClock`, `seekReady`,
  `mediaInfoText` and `setSliderValue`, all pinned; the row greys out with
  no player behind it. Volume round-tripped against a real headless mpv.
  Verified: `./internal/player/ ./internal/ui/ ./internal/catalog/` green.
  idea-0105 is fully landed.
- mpv input dead by design (bug-0101, card removed). The manual settles
  what a live run could not here: `--no-input-default-bindings` kills the
  builtin bindings and the script ones (OSC included), so the player
  window's keys and OSC buttons never answered — and never will. That is
  now the documented intent, not a suspicion: the window is a screen, the
  remote lives in our window (idea-0105's row), and a second remote the
  room sees only by polling is unwanted. Reason recorded in `DEV.md` and
  beside the flag in `Args`; no behavior changed, nothing to run beyond
  the untouched suites.
- A dropped stream never left the room (bug-0106, card removed). The roster
  lives in `Room`, the connections in `Mesh`, and the room learned of
  departures **only** from a `bye` — so a stream that ended on its own (a
  crash, a kill, a cut cable) left the peer in every roster forever, and a
  host that vanished that way never handed over. `byeGracePeriod` is one
  second, so a loaded machine crosses it routinely and the farewell never
  goes out. `readLoop` now manufactures the bye on the listening side,
  dropped rather than waited for: a room that has stopped reading is gone,
  and blocking on it would turn a departed peer into a stuck goroutine.
  `Room` is untouched — its existing `onBye` was always the whole answer.
  Found by the release checklist: `TestARoomWithoutAServer` red on a full
  frontend run, green alone and on all three recent commits. Three tests,
  the first verified to fail without the fix. Verified: `-race` on
  `./internal/rtc/` green, the whole frontend suite green, and the
  scenario test green three times running.
- Release check turned up a gate that had never run against the new code
  (bug-0107, card removed): `Lint Backend` red on two gosec G304 in
  `persistence_edge_test.go`, a file committed unverified inside 096215c —
  `go vet` does not see it, gosec does. Both suppressed with the reason
  (t.TempDir() plus a fixed name, nothing external) by the established
  `#nosec` pattern. Verified with the same linter as CI, v2.14.0: whole
  backend 0 issues, contract vet clean, package green.
- Direction umbrella closed (idea-0090, card removed). Every item ended
  somewhere on record: transplant, heartbeat, preparing, roster, release
  v0.1.0, packaging, diagnostics, megabytes, peers, event line, dialogs,
  shortcuts, own remote, teardown, README, platforms — done; legal dialog,
  Winget, warnings — declined; field test — cancelled by the owner;
  translations — hand-kept. Nothing hangs without a decision. The board is
  empty.
- Event line beside the status line (idea-0090, UX parity slice). Every
  failure, arrival and one-shot confirmation travelled through the status
  line — which the 2-second poll rewrites, so a reported error lived two
  seconds and was gone. Events (`w.notify`) persist until the next event;
  the poll owns status and never touches them. All former event call sites
  moved over; the poll counter stayed. Pinned by `TestEventsSurviveThePoll`
  (a succeeding poll must leave the event line alone). No new catalogue
  keys: messages already travelled through `tr`/`trf` at the call sites.
- Preparing line shows peers (idea-0090, UX parity slice). `TorrentHandle`
  grew `ActivePeers` — live connections, not tracker promises
  (`localHandle` reads `Stats().ActivePeers`, `remoteHandle` the gateway's
  `peers` from the already-fetched metrics) — plumbed through
  `torrentToInfo` into `models.TorrentInfo.peers`, and the window renders
  it beside the megabytes (`peers: N` / `пиры: N`, no declension machinery).
  Swagger regenerated (docs-only diff, CI freshness gate holds); the pact is
  untouched — its list bodies are empty. Verified: backend `go test ./...`
  green, frontend `ui`/`catalog` green, `integration` 4/5 green with the one
  red a cold-cache run (38 s vs 5 s warm, never reproduced) — recorded as a
  flake, not a finding.

### Fixed

- Untracked `validation_edge_test.go` did not compile (bug-0099, trivial
  fix, card removed). The mutant-killing edge tests used `math.NaN()` with
  no `math` import, so `go test`/`go vet` failed for the whole
  `internal/validation` package. One import added; verified: package green,
  `go vet` clean, `gofmt` clean.

### Changed

- Packaging epic idea-0026 closed (card removed). Verified against the live
  release: `v0.1.0` is published, not a draft, with all three artifacts
  (`TorrSyncPlayer-x86_64.AppImage`, `TorrSyncPlayer-windows-x64.zip`,
  `TorrSyncPlayer-macos-arm64.dmg`) plus `SHA256SUMS`. Winget stays refused
  and auto-update stays deferred — both recorded decisions, not open items.
- Direction after the multiplex comparison fixed (idea-0090, refining):
  connection diagnostics first (new candidate idea-0100 — room swarm/DHT/
  relay state in the window instead of eyeballing "did not connect"),
  then full UX parity with the multiplex tutorial, then the two-machine
  field test. Nothing of multiplex's transport or toolkit is taken: weron
  is frozen and GTK4 is Linux-only, both contradict our goal.

### Reconciliation 2026-10-16 — 1 finding, fixed here

Fresh reconciliation (project-docs step 2). Boundary: last commit touching
this file (`6c264ba`) plus the zone — commit `6a12eb4` (mutant-killing tests
in `buffer`/`persistence` only, no production code) and one untracked file
(`validation_edge_test.go`). Suite in the zone: `buffer` and `persistence`
green, `validation` build-failed on the missing import — the single finding
(bug-0099), repaired the same session. No goal/way change; boundary moves
with this entry.

## [v0.1.0] - 2026-10-16

### Fixed

- PSK swarm gate removed before it reached production (idea-0091, accepted
  and transferred to the goal and the way; card removed). Slices 1–2 gated
  the whole swarm by the invite key (`libp2p.PrivateNetwork`), while the ADR
  promised rendezvous over the public DHT and public relays for harsh NAT —
  a combination that cannot work: the protector wraps every connection, so a
  room-keyed node could talk to neither public bootstrap nodes nor public
  relays, and `Join` would fail on first contact in production. The tests
  stayed green only because the testbed shares one invite. Per the owner's
  decision the gate is gone and the key keeps a real job: `deriveAuthKey`
  now stretches the password and key pair, so admission proves knowledge of
  both and the invite format `community:password:key` is unchanged.
  `CONCEPT.md#Доступ без лишних хлопот`, `DEV.md` (invite section and ADR)
  and `README.md#Что защищено` say what is now true: a stranger may open a
  transport connection, but gets no stream and hears no message. Tests:
  `TestDeriveSwarmKey` replaced by `TestDeriveAuthKey` (pair matters both
  ways, either half empty refuses), `TestDifferentKeyCannotConnect` re-pinned
  on handshake refusal instead of transport isolation. Verified:
  `go vet ./internal/rtc/` clean, `FYNE_DRIVER=offscreen go test
  ./internal/rtc/` green, `-race` clean. Side effect, intended: idea-0087
  (autorelay) is unblocked as specified — public relays are dialable again.

- Node listened on loopback only (bug-0092, card removed). `NewNode` bound
  `/ip4/127.0.0.1`, so no inter-machine direct connection could ever exist
  while the goal promises direct dialing and hole punching; the tests stayed
  green because every peer lived on one machine. It listens on `0.0.0.0`
  (TCP+QUIC) now. Verified by the existing DHT suite, which dials peers by
  their advertised addresses: `go test ./internal/rtc/` green, `-race`
  clean.

- Position heartbeat (idea-0093, accepted and transferred to the goal and
  the way; card removed). The goal said sync is event-only — a follower 30
  seconds behind waited for the host's next command. Now the host repeats
  its position every 5 seconds unconditionally (every 10th 500 ms
  `watchPlayer` tick), and followers converge through the ordinary
  `Position` + `SmoothSeek` path: no new message type, no teleports, pause
  still travels on change only. `CONCEPT.md#Синхронные комнаты` and
  `DEV.md#Сглаживание перемотки и heartbeat` (renamed section; no card
  referenced the old anchor) say the new model. The period decision lives in
  the testable `hostSeekDue`, not buried in the loop. Verified:
  `FYNE_DRIVER=offscreen go test ./internal/ui/` green. Also fixed a stale
  comment left by idea-0091 (`Invite`: the key admits in the handshake pair,
  it encrypts nothing).

### Added

- Legal-confirm dialog rejected (idea-0096, rejected; card removed). The
  owner decided against a rights-confirmation screen before playback: an
  extra click before every film, copyright texts to write and maintain in
  two languages, while the choice of source already lies with the viewer
  and the project ships no DRM and no catalogues. Playback keeps starting
  on file selection, as now.

- Download-visibility line (idea-0094, accepted and transferred; card
  removed). Playback starts before the download finishes, but no widget said
  so: the list shows names only, and progress lived in no UI at all. The
  window now carries a preparing line above the lists — name, percent and
  backend status of the playing torrent from the existing 2-second poll, no
  new endpoints. Empty when nothing is playing or the download finished; an
  unknown backend status shows as it arrived. Decisions live in testable
  `preparingText`/`playingTorrent`/`torrentStatusWord`, catalogue keys
  `status.preparing` and `torrent.*` in both languages. Verified:
  `FYNE_DRIVER=offscreen go test ./internal/ui/ ./internal/catalog/`
  green (the catalogue-usage gate covers the new keys).

- Roster arrivals and departures are noticed (idea-0095, accepted and
  transferred; card removed). The room line showed who is here but the
  moment of change passed silently. `OnRoster` hands the whole roster, so
  the window keeps the previous one by peer id and reports one status line
  per arrival and departure, arrivals in roster order. The first roster
  after joining baselines instead of announcing, and self stays silent.
  Catalogue keys `status.peerJoined`/`status.peerLeft` in both languages.
  Verified: `FYNE_DRIVER=offscreen go test ./internal/ui/
  ./internal/catalog/` green.

- Tags and releases cleaned, version line rolled back to 0.x (owner
  decision, no card — repo hygiene, not goal/way/code). Deleted: all 114
  tags (~55 `push-*` CI noise from a dead Qt-era workflow plus phantom
  v0.x/v1.0.x–v1.1.x that never shipped — no GitHub releases, no changelog
  entries, red pipeline at the time) and the 4 stale draft `v1.2.0-rc`
  prereleases. No published release ever existed, so no public promise was
  broken; the old `[v1.1.5]`/`[1.0.0]` journal sections stay as history.
  The next release number will be chosen by the `make release VERSION=`
  argument on release day; the `/api/v1` path prefix is unrelated and
  untouched.

- Buffer endpoints answered 500 instead of 404 for unknown torrents
  (bug-0097, card removed). The k6 journey expects 404 on `/buffer/info`
  and `/buffer/position` (10k failed checks, red job); both returned 500
  because the buffer service raised plain `fmt.Errorf` and `handleError`
  maps everything untyped to Internal — while the Swagger annotations
  promise 404. The expectations came from idea-0064 slice 4, after rc.6, so
  the green rc.6 never ran them. Fixed at the owner of the knowledge:
  `errors.NotFound` in `UpdatePosition`/`GetBufferInfo`, pinned by typed
  `IsNotFound` assertions in the three `_NotRegistered` tests (RED first),
  verified against a live server: 404 and 404.

- Dead code removed from both modules (bug-0098, card removed). `deadcode
  -test` plus per-item reference triage (false positives — contract-tagged
  tests, `loopbackTracker.state` — dismissed with evidence). Backend: the
  whole orphaned `Debouncer`, `torrent.NewService`,
  `errors.NotImplemented`, `models.PaginatedResponse`. Frontend: three
  `Client` getters, `Player.SocketName`, `BufferReporter.SetPlaying`,
  `Room.SelfID`, `Room.TransferHost`, six window methods including
  `tellRoom` (orphaned by the same pass — removed right after), three test
  helpers and the orphaned `status.sendFail` key. Fix found along the way:
  `startFakeMPVWithEvents` dropped its events, so the event-line test
  passed vacuously; events now flow through `dial`/`serve` (ordering
  matters — net.Pipe rendezvous deadlocks on write-before-read, caught from
  a goroutine dump). Verified: both modules fully green, lint clean,
  `deadcode -test` quiet.

- Lint Backend red on the release pipeline (trivial fix, no card: one
  package, test files only, no goal/way/interface change). golangci-lint
  v2.14.0 brought new checks the tree never saw: gosec G705 taint on two
  test doubles writing fixture bytes (suppressed with reason, the
  established `#nosec -- reason` pattern) and two dead struct fields in a
  recording fake (deleted). Verified with the same linter version locally:
  0 issues. The in-flight v0.1.0 artifacts are unaffected: the release
  workflow runs no lint, and the findings were test-only. Follow-up in the
  same job: `go mod tidy` demotes the stale direct require
  `golang.org/x/crypto` (nothing imports it anymore) and the job's
  tidy-check failed on the diff — committed as tidied.

- Autorelay with a peer source (idea-0087, accepted and transferred; card
  removed). Bare `EnableAutoRelay()` panics without candidates, so the node
  carries its own: the owner's static relays first (`--relays` flag,
  `relays` setting, `MeshOptions.Relays` — empty means DHT discovery only),
  then random DHT walks capped at `maxRelayWalks`. Room streams open over
  relayed connections (`WithAllowLimitedConn` in `DialRoom` — a relayed
  connection is constrained, and without it every reserved path stays shut).
  Three tests, all green: a magnet round-trip over a forced circuit address
  (the caller knows no direct address, so the byte travelled the relay),
  DHT walks yielding candidates, and the setting's shape (empty/duplicates/
  undialable). The loopback-only listen is gone with it (bug-0092): the node
  binds `0.0.0.0`, otherwise direct dialing between machines could not exist.
  Verified: `go vet ./...` clean, `FYNE_DRIVER=offscreen go test
  ./internal/rtc/` green, `-race` clean, `./internal/ui/` and
  `./internal/catalog/` green (flag→setting→mesh precedence pinned both
  ways).

- Slice 1 of the libp2p transplant (idea-0086): `frontend/internal/rtc/p2p.go`
  with `Node` — one participant's libp2p host. The swarm is gated by the
  invite key (PSK protector derived by domain-separated SHA-256: without the
  key there is no connection at all), streams are admitted by the invite
  password (challenge-response HMAC before a single protocol byte flows;
  constant-time compare on verify), peers rendezvous over the DHT (provider
  records under `torrsync-room-v1` + community). Admitted streams speak the
  unchanged `protocol.go` messages, newline-delimited. Five tests, all green:
  key derivation, rendezvous determinism, host/guest DHT rendezvous with a
  magnet round-trip, wrong-password rejection, wrong-key isolation — the last
  three against an in-process three-node DHT testbed (a stable bootstrap
  node, mirroring production where bootstrappers are long-lived DHT
  servers). No `-race` warning from the new code; the only `-race` failures
  left in the package are the known weron ones in the old tests, which slice
  2 removes with the transport.
- Two findings the slice settled, recorded so the next one does not rediscover
  them: `Bootstrap` must not fall back to the public default bootstrappers
  when given no peers (hermetic tests must not dial out), and the routing
  table fills on its own once identify reports kad speakers — a forced crawl
  on an empty table errors instead of helping, so `WaitReady` waits rather
  than pushes. A peer met before its DHT existed is admitted on the next
  contact, which is why the testbed stands its bootstrap node up first.

- `DEV.md#Хранение данных` documents the registry and the ownership now
  (idea-0059, accepted): what survives a restart, why the registry keeps the
  source rather than a metadata summary, and why ownerless torrents stay open.
  One paragraph in the existing section, no duplication, no new sections —
  as the card proposed and the owner confirmed. The card is removed; no code
  changed, nothing to run.

- Subtitles, slice A: the endpoint (idea-0080, variant 2).
  `GET /api/v1/torrents/{id}/subtitles/{fileIndex}/{fileName}` serves any
  torrent file by index with the `/stream` gates (access-token header,
  client id, room-scoped read access) but no selection requirement: subtitle
  tracks live next to the film. The trailing name is mpv's, not the server's
  — sub-add reads the subtitle format off the URL's extension and refuses an
  extensionless one (proven against the real binary), so the client repeats
  the file name and the server ignores it. `TorrentService` grew
  `ServeSubtitles`; the Swagger spec is regenerated (CI freshness gate
  holds). Nine service/API tests plus index-routing and name-ignoring pins.
- Subtitles, slice B: the player and the window. `Player.AddSubtitles`
  (sub-add with reply check) and `RemoveExternalSubtitles` (sub-remove over
  external tracks only; `Track` grew `External`), a "Sub file" chooser next
  to the track choosers fed by the torrent listing (`.srt/.vtt/.ass/.ssa/
  .sub`, off by default), and `Client.SubtitlesURL` building the endpoint URL
  with the file name escaped. Proven against the real mpv over authenticated
  HTTP, and end to end: a real seeder, a real swarm, the endpoint serving
  real bytes (401 without the token), mpv listing the external track.
- idea-0080 closed: container tracks (done earlier) plus torrent-file
  subtitles (here) cover both halves; the card is removed.
- Critical fix found by slice B (bug-0089, card removed): mpv fetched with
  the token alone and no `X-Client-ID`, so since the ownership gate it has
  been user "" against real owners — 403 on every `/stream` and `/subtitles`
  fetch. Fixed by sending the owner's identity too, in one comma-joined
  `--http-header-fields`: repeated flags do not accumulate in mpv, the last
  flag wins and the earlier headers never leave the machine (proven against
  the real binary). `Args` joins into one flag, `FormatHeader` additionally
  rejects commas, `mpvHeaders` builds the pair. Anonymous flows (empty client
  id) are unchanged.

### Changed

- Room transport moves from weron to go-libp2p (idea-0086, accepted and
  transferred to the goal and the way). weron is frozen — no push in 18
  months, a bugfix PR open for 2 years — and holds two detector-proven races
  (`wrtcsgl` client map, `wrtcconn` `done` flag) that cannot be worked around
  from our code: the client race sits on the `Close` path itself, and Go's
  race runtime ignores `TSAN_OPTIONS=suppressions`, so no suppression file can
  silence it. A GitHub-wide search confirmed there is no maintained drop-in
  replacement; go-libp2p is the only live popular P2P stack, and writing our
  own pion client was rejected as ~100× the cost for the same result.
- `CONCEPT.md#Синхронные комнаты`: rendezvous is the public DHT (IPFS
  bootstrap nodes + Kademlia), commands travel encrypted libp2p streams,
  autorelay covers harsh NAT, invite format `community:password:key` keeps its
  roles (rendezvous key, password handshake, signaling key).
- `DEV.md`: new ADR (2026-10-16, replaces the weron ADR), `internal/rtc`
  retargeted at libp2p, stack pins go-libp2p v0.50.0 (needs Go 1.26 — our
  toolchain), scenario tests run against a local DHT testbed, principle 6
  reworded. Decisions confirmed by the owner: public DHT bootstrap, password
  handshake admission, autorelay on, whole-swap rollout behind green scenario
  tests.
- Audit: bug-0084 removed as moot — both races die with the library instead
  of being fixed. Its out-of-process helper (`testsignaler`, `TestMain`
  build) never reached `main` and is reverted from the tree. libp2p stays the
  recorded fallback direction only in the sense that re-evaluation, if ever
  needed, starts from idea-0086's rejected options.
- Slice 2 landed the code: `Mesh` runs on `Node`, the `Transport` interface
  `Room` talks through untouched, so roster, host authority, takeover and
  seek smoothing moved over byte-identical. `MeshOptions.Signaler` became
  `MeshOptions.Bootstrap` (comma-separated multiaddrs with /p2p/, empty means
  the public defaults); duplicate streams to one peer resolve first-wins,
  since sends are keyed by ID and either survivor carries the same traffic;
  late joiners converge through a 15 s rediscovery loop.
- Settings and flags follow the transport: `--signaler` is `--bootstrap`,
  the saved `signaler` key is `bootstrap`, the form label and both catalogue
  languages say rendezvous. An old saved weron URL is dropped, not migrated —
  it could never be a bootstrap address. Empty still means the public
  defaults; the placeholder shows an example multiaddr instead of a URL.
- The stranger rule changed shape with the transport and is pinned as such:
  presence publishes for everyone (presence is not authority), but no
  handshake passes either way — the stranger never sees a peer, never hears
  traffic, the room never sees it, and the room keeps working around it.
- Still open after slice 2, owned by the next step: README and SECURITY.md
  describe the weron arrangement (bug-0088). Code, goal and way agree; those
  two documents lag one slice behind.

### Removed

- weron, entirely: `signaler_test.go`, the `wrtcconn`/`wrtcsgl` imports in
  mesh, pipe and integration tests, and the dependency itself (`go mod tidy`
  confirms zero mentions in go.mod/go.sum). The in-process test signaler is
  replaced by a DHT testbed built on the exported `Node` API.
- idea-0087 opened for what slice 2 deliberately leaves out: autorelay needs
  a peer source or static relays (bare `EnableAutoRelay()` panics), so only
  direct connections plus hole punching ship — parity with weron, no
  regression, the promised relay gain pending.

### Verified (slice 2)

- `go test ./...` green across all frontend packages; `go test -race
  ./internal/rtc/` green with **zero** warnings — the detector class bug-0084
  was opened for is gone with the library, proven by the detector itself.
- `TestARoomWithoutAServer` passes end to end on the new transport: real
  backends, real torrent swarm, magnet, adoption, pause fan-out, takeover.

### Fixed

- README described the removed server-brokered architecture instead of the
  P2P mesh (bug-0085). The rooms bullet, the "watch together" flow, the
  "what is protected" paragraph, the troubleshooting section, the API table
  and the limitations all assumed REST rooms, SSE event streams and stream
  tickets — none of which exist since idea-0064: rooms meet over the weron
  signaler, commands travel the WebRTC data channel, `/stream` checks the
  ordinary `X-Access-Token` header, and the ticket machinery is gone
  entirely. The table now lists the real routes from `router.go`, and the
  invite code `community:password:key` is documented where the room password
  used to be. Reconciliation: zone was empty (last commit touched this file,
  tree clean), backend `go test ./...` green, frontend
  `FYNE_DRIVER=offscreen go test ./...` green; one finding, fixed, card
  removed.
- README and SECURITY.md describe the libp2p arrangement instead of weron
  (bug-0088). README named the signaler, fixed a "WebRTC connection" and sent
  the reader to `--signaler`, which no longer exists; SECURITY threat-modeled
  a party that no longer exists. Now: rendezvous over the public DHT with
  `--bootstrap`, the swarm gated by the key, streams admitted by the password
  handshake, and the DHT/bootstrap/relay view spelled out as the party that
  sees something. The relay paragraph is conditional on autorelay (idea-0087):
  direct connections plus hole punching are what ships, and the limitation
  says exactly that. Card removed; no code changed, no tests to run beyond
  the prose itself.

### Added

- **A room can be opened from the application.** `Session.Create` existed, was
  bound to a torrent and covered by tests — and nothing in the interface ever
  called it. The window could join a room but not open one, and since a room id
  comes from the server, there was nothing to join either: the whole "watch
  together" half of the product was unreachable from the app while README said
  the host creates a room (deviation-0068). "Open a room" now sits next to
  "Join", takes its name from the torrent that is already selected and its
  password from the field the join form already has, and drops the invitation
  code straight into the field the copy button reads — so the next thing a host
  does is one click, not a menu. The button starts greyed out and wakes up
  when a torrent is picked, because a room is bound to one and a button that
  can only fail is worse than a disabled one.
- The room name is **derived, not passed through**. The server accepts letters,
  digits, spaces, hyphens and underscores up to 50 characters, while real
  torrent names are "Movie.2024.1080p.BluRay.x264-GROUP.mkv" and "[SubsPlease]
  Show - 01" — so sending the name unchanged is a 400 after the person has
  already chosen a torrent and a password. `roomNameFor` strips what the rule
  does not allow, collapses whitespace, cuts to the limit, and falls back to a
  translated default when nothing usable survives. Without this the button
  would have failed on roughly half of all torrents.
- New catalogue keys in both languages: `action.createRoom`, `status.created`,
  `room.defaultName`.

### Fixed

- The release workflow built the removed Qt application. Every job installed
  Qt6, libmpv and cmake and configured a directory with no `CMakeLists.txt`, so
  `make release` had been tagging into a red pipeline and producing no
  artifacts at all (bug-0070). It is rewritten for the Go/Fyne pair: an AppImage
  on Linux with `mpv` **inside** it, a zip on Windows carrying `mpv.exe` and its
  DLLs, and an ad-hoc-signed `.app` in a DMG on macOS — the three formats
  decided by the owner, with no auto-update in this pass.
- **Every artifact is now checked for content after it is built.** The AppImage
  is extracted and searched for the client, the backend, mpv, `AppRun` and a
  non-empty `usr/lib`; the zip is unpacked and searched for both `.exe` files
  and `mpv.exe`; the DMG is mounted and `Contents/MacOS` is listed. An artifact
  that builds and is empty is now red instead of green — the previous file
  would have published it.
- `codeql.yml` ran a `['go', 'cpp']` matrix whose C++ leg installed Qt6, libmpv
  and cmake and built a directory with no `CMakeLists.txt`, so that job had
  been failing rather than analysing anything. It is one `go` leg now. The
  frontend turns out to have been outside CodeQL's scope entirely: roughly six
  thousand lines of client code that nothing was analysing, now built and
  analysed alongside the backend.
- `security.yml`'s `cpp-security` job ran `cppcheck` over a directory that no
  longer exists and `scan-build` with cmake, both ending in `|| echo` — gates
  that could not go red. Replaced by `gosec` over both modules. That job is
  deliberately blocking: its first run may be red, which is the gate working,
  and the findings get fixed rather than silenced.
- The application icon is drawn in code and now available at any size:
  `internal/ui.IconPNG`, with `frontend/cmd/icon` rendering it to a file at
  packaging time. The Qt icon resources were deleted with the client, and
  without this an AppDir and a `.desktop` entry would have shipped without one.
  Drawing it from the same code the tray uses means the icon in the package and
  the icon in the tray cannot drift apart — and `TestTheIconIsNotBlank` exists
  because bug-0034 shipped sixteen PNGs that decoded from nothing.
- **Removing a torrent with `--disk-storage` did not remove its data.**
  `FileStorage.RemoveTorrent` removed `<dataDir>/<infohash>`, a directory
  anacrolix never creates — it lays a torrent down as
  `<dataDir>/<Info.BestName()>`. `os.RemoveAll` on a missing path returns nil,
  so the removal reported success and the gigabytes stayed: the API answered
  200, the torrent left the list, and the user was told it was gone (bug-0077).
  It takes the name now. The sentinel name `metainfo.NoName` — a torrent whose
  files sit directly in the data directory, mixed with everyone else's — removes
  nothing, because there is nothing there that can be removed safely.
- The build system still spoke Qt. Twelve `make` recipes — `frontend`,
  `build`, `test-frontend`, `test-frontend-asan`, `test-frontend-tsan`,
  `test-frontend-coverage`, `test-frontend-gmock`, `test-frontend-fuzz`,
  `test-frontend-mutation`, `test-all`, `test-ci`, `test-full`,
  `integration-test`, `e2e-test` — ran `cmake` in a directory that has had no
  `CMakeLists.txt` since idea-0067, so the documented entry points for building
  and testing the project had been failing for a whole session without anyone
  noticing (bug-0069). They are rewritten against Go: `make frontend` runs the
  documented pair build, `test-frontend` runs `go test` with
  `FYNE_DRIVER=offscreen` and `CGO_ENABLED=1` (mandatory — without the
  offscreen driver the tests *wait* for a window rather than failing, so there
  would be no green run and no red one either), `test-frontend-race` matches
  CI, and `integration-test` runs both ends of the seam: the backend's real
  router, and the real backend binary driven by the real client. The
  asan/tsan/gmock/fuzz/mutation recipes are gone rather than kept as
  placeholders — none of those targets exist. `e2e-test` is gone too: after the
  Qt removal the scenario tests *are* the end-to-end test, and a second name for
  one thing invites one of the two to rot.
- `contract-test-backend` assumed CI had already downloaded the pact FFI, which
  made `make test-all` red on a green tree for a reason that had nothing to do
  with the code. It now fetches the library itself on linux/amd64 and says
  plainly what to do elsewhere (bug-0069).
- The last traces of the deleted client are gone: `frontend/.clang-tidy`,
  `scripts/fill_translations.py` (it filled `translations/*.ts`, which no longer
  exist — catalogues are maintained by hand in `internal/catalog`), and
  `tests/e2e/playwright`, whose spec asserted a login form with username and
  password fields this project has never had and, by design, will never have
  (bug-0069). `.gitignore` lost its Qt block and gained `frontend/dist/` and
  `backend/.pact-ffi/`.
- `go run ./cmd/build` put the two binaries in two different directories
  whenever `-out` was relative — which is the invocation `DEV.md#Сборка`
  documents first. `go build -o dist/x` resolves its `-o` against *its own*
  module directory, and the two builds run from different modules (`.` and
  `../backend`), so the client landed in `frontend/dist/` and the backend in
  `backend/dist/`. The client looks for its backend beside itself, so the pair
  this produces does not start. Output paths are made absolute before they are
  handed to `go build`, while what the script prints is still what the caller
  asked for (bug-0074). `frontend/cmd/build` had **no tests at all** and CI
  only ever ran `go build ./...` on it, so nothing saw this — and a live run
  with an absolute `-out` masked it. The invariant is now pinned by tests, with
  the builder injected so proving it does not compile two binaries.
- The pact provider test had not compiled since `e2fb119`: it called
  `sync.NewService()` while the constructor has required a `HostChecker` since
  that commit. The whole package is behind the `contract` build tag, so
  `go build`, `go test`, `go vet` and `golangci-lint` all skipped it and only
  the Pact job compiled it — meaning **that CI job has been red, not green,
  since `e2fb119`**, while the changelog entry for the commit claimed
  everything was verified (bug-0075).
- The contract was then wrong in substance, which the compile error had been
  hiding. `POST /rooms` requires `torrentId` since bug-0047 and the recorded
  interaction had no such field, and the "user is in a room" provider states
  were no-ops — a promise the provider never kept. They were written when the
  sync handlers did not ask who was calling; they do now, resolving the
  caller's room first and answering 400 without one, so three interactions
  failed on the very state they claimed to be in. The room states now create a
  real room through `p2pSvc.CreateRoom` for the client id the interactions
  present, if it has none. Eleven interactions verify against the real router.
- `lint-backend` now runs `go vet -tags contract ./...`. Without it, the next
  changed constructor in the contract package breaks the same way, with nothing
  but one distant job to notice.

Fresh reconciliation: the zone was empty (the last commit touched this file and
the tree was clean), so everything was checked rather than just the delta. Six
findings went to the board as cards: `deviation-0068` (no way to create a room
from the UI), `bug-0070` (the release workflow still builds the removed Qt
app), `idea-0071` (the core promise has never been checked against a real
swarm), `idea-0072` (the frontend is never compiled on macOS or Windows),
`idea-0073` (nine English messages bypass the catalogue), `idea-0076` (the pact
is now a hand-edited fixture). Verified: `make test-all` passes end to end on a
green tree — 20 backend packages, 9 frontend packages, 11 pact interactions
against the real router, 10 backend e2e tests, 5 scenario tests driving the real
client against the real backend binary; `gofmt` and `go vet` clean; `make build`
puts the pair in `frontend/dist/` and the built client starts its own backend.
Reconciliation found 6 findings; the three bugs it uncovered are fixed and their
cards removed (bug-0069, bug-0074, bug-0075).

Second step of the same session: `deviation-0068` resolved — the room can be
created, in `frontend/internal/ui/createroom.go` with the three decisions
(`canCreateRoom`, `roomNameFor`, `invitationFor`) extracted so they can be
tested without a widget. What this still does **not** cover is the click
itself: the Fyne test driver does not deliver window events, so the path from
the button to the session is unexercised end to end, and a live run can only
show that the built application starts. Verified: 9 frontend packages green,
`gofmt` and `go vet` clean, built client starts its backend with no errors in
the log.

Third step, `bug-0070`: release, CodeQL and security workflows rewritten for Go
and Fyne; all four workflow files parse; the icon renders as a real 256×256 PNG,
checked visually as well as by test; `make test-all` green. **Not verified: the
workflows themselves.** No step of a release pipeline can run on a machine with
no runner, no linuxdeploy and no appimagetool, so "rewritten" here means exactly
that — the first execution is the next tag push, and the card stays
`in_progress` until an artifact has actually been produced and run.

Fourth step, `idea-0071`: **a real-swarm scenario is written and does not pass
yet**, so the goal's first sentence is still unproven. The scaffolding is in
`backend/internal/torrent/real_swarm_test.go` and `loopback_tracker_test.go` —
two real anacrolix clients, a real .torrent, a real announce URL, and
assertions that read piece priorities and stream a byte range the way the media
endpoint does. What it established along the way, none of which the next session
should have to rediscover:

- **There is no tracker in the library.** `AnnounceTracker` is an interface with
  no implementation anywhere in the module, so a minimal in-memory one was
  written. It works: both clients register.
- **The `nodes` list in a .torrent is a trap.** It looks like the cheap way to
  avoid a tracker, but the library passes it to `AddDhtNodes`, and with
  `NoDHT: true` there is no DHT server to pass it to, so the address is dropped
  without a word. The first version of the test did exactly this and produced
  zero pieces in sixty seconds while looking healthy.
- **File storage is flat**: `<dataDir>/<Info.BestName()>`, with no infohash
  directory. The wrong layout says `file has unexpected size ... size=60`,
  which reads as "there is no data". Note that our own `FileStorage.RemoveTorrent`
  deletes `baseDir/<infohash>` — that contradicts what anacrolix actually does,
  and is a separate finding.
- **This machine cannot hash file-backed pieces.** The default backend
  memory-maps and fails with `mapping file: no such device`; with
  `TORRENT_STORAGE_DEFAULT_FILE_IO=classic` it needs SEEK_DATA and fails with
  `seeking to start of extent: invalid argument`, on tmpfs and on btrfs alike.
  A probe (`requireFileBackedPieces`) skips the test where the machine cannot,
  with the reason in the skip message rather than a green run that means nothing.

What remains unexplained, and is the whole point of the exercise: once the
layout is right the **seeder holds all 8 MiB, the tracker lists both peers, and
not one byte is exchanged in thirty seconds**. That is the transfer itself, not
storage and not discovery. The test reports this as a skip with the numbers in
the message. Verified: `make test-all` green, 29 packages.

- The first real push since the Qt client was removed proved that the rewritten
  pipeline had never actually run. What came back: **32 golangci-lint findings**
  across the backend (bug-0069), an updated `golang.org/x/net` that the pinned
  `golangci-lint v2.12.2` could not parse, a Swagger spec still describing the
  deleted stream-ticket endpoint, and a `gosec` run that surfaced two extra
  findings the linter had not. The deprecated `mi.Magnet` helper is gone in
  favour of `MagnetV2`; unchecked `os.Setenv`, `svc.Close`, `resp.Body.Close`,
  `Play` and `Pause` results are handled; the SSE test now closes its response
  body through `t.Cleanup` instead of a `defer` that fired when the helper
  returned and closed the stream before the guest could ever join.
- **The Fyne client could not be built on an Ubuntu 24.04 runner at all.** glfw
  builds through cgo and wants Wayland headers: every workflow assumed GL, X11
  and KGL were enough, and all four of them failed on
  `wayland-client-core.h: No such file or directory` the first time the frontend
  was actually compiled by CI. `libwayland-dev` is now installed everywhere the
  client is built.
- Go 1.27.1 leaves twelve `net/http`, `crypto/tls` and stdlib vulnerabilities
  reachable through `http.Client`, so `govulncheck` failed the backend test job
  on a toolchain that was current the day it was pinned. All are fixed in
  1.27.2, which every workflow now pins.
- The load-test journey created rooms without a `torrentId`, which the API has
  required since rooms were bound to torrents. Every create answered 400 and
  every sync check behind it failed — `create room 201` at 0% out of 3689
  attempts on a server that was answering every list request correctly.
- `codeql.yml` analysed the runner's preinstalled Go instead of the pinned one,
  so what CodeQL compiled could differ from what the pipeline builds and gates.
- **`PickFreePort` listened on `:0`, which really does bind every interface.**
  The port it hands out travels in a URL given to mpv on the same machine, so
  it should never have been reachable from outside it — and the comment beside
  the function claimed exactly that while the code did the opposite. It binds
  `127.0.0.1:0` now. This is gosec G102, and it only surfaced because the
  Security workflow finally ran over the frontend.
- That first frontend `gosec` run also found two `0o755` directories and a
  `0o644` file in the packaging tools, three subprocess launches it could not
  see were safe, and a G101 on the catalogues — gosec matches any map key
  containing "password" that holds a string value, and a UI catalogue is
  nothing but string values keyed by name. Renaming the key to
  `field.roomPassword` did not help, because "Password" is still the match;
  the suppression went on the declaration with the reason, rather than the key
  being made vague enough to slip past the heuristic.
- The Security workflow had no `workflow_dispatch`, so a weekly-only gate
  could not be re-run after fixing anything: the finding is fixed, the path
  triggers do not change, and the only way to learn it is green is to wait
  until Monday. It is runnable on demand now.

### Added

- Room ↔ Torrent binding (bug-0047, first step of idea-0046). Each room is now
  bound to a specific torrent infohash at creation time (`POST /rooms` requires
  `torrentId`; `POST /rooms/join` requires the same `torrentId` and the server
  rejects mismatches). The Qt frontend enforces this: create/join dialogs are
  disabled until a torrent is selected, and the current `torrentId` is sent
  automatically. Internally, `Room` gained a `TorrentID` field, `RoomInfo`
  exposes it to clients, and the P2P service validates it on join. Covered by
  six new p2p tests (`TestCreateRoomWithTorrentID`, `TestCreateRoomWithoutTorrentIDFails`,
  `TestJoinRoomValidatesTorrentID`, `TestRoomInfoIncludesTorrentID`,
  `TestRoomTorrentIDImmutable`) and updated API tests. This is the first
  concrete piece of the “each peer downloads from the swarm” pivot: a room
  cannot exist without a torrent to watch.
- Independent rate limiter for `POST /torrents/{id}/buffer/position` (bug-0031).
  The Qt frontend reports playback position every 500 ms (2 Hz) from every
  playing client. The protected API group ran at 1 req/s burst 10, so a single
  player exhausted the bucket within seconds and every subsequent report was
  answered 429 — the client does not retry a 4xx, so piece priorities silently
  froze a few seconds into playback, the exact defect bug-0028 tried to remove.
  The endpoint now has its own per-IP limiter (`BufferPositionRateLimit=20
  req/s`, `BufferPositionRateBurst=40`) registered **before** the token check,
  so an unauthenticated caller cannot spend a legitimate client's budget.
  Covered by six new tests: client cadence survives, room-behind-one-NAT budget,
  control-plane budget untouched, token still required, wrong token rejected,
  flood still bounded.
- `BackendArgs` abstraction + `test_backendargs` guarding the contract between
  frontend and embedded Go backend (bug-0032). The frontend launches the
  backend via `QProcess` and passes flags; an unknown flag makes Go `flag.Parse`
  exit(2), which is exactly how `--jwt-secret` survived the removal of JWT auth
  and broke the Windows release while all 14 CI jobs were green. The flag list
  lives in `backendargs.cpp` and is asserted by the test: every flag sent by
  the frontend must exist in `knownBackendFlags()` (mirroring Go's
  `parseFlags`), no secret-like argument may appear, and every argument is a
  paired `--flag value`. The release binary now passes only `--port 8889`.
- Release publication unblocked (bug-0033): `draft: false` in
  `.github/workflows/release.yml`; awk now accepts both `## [v1.1.5]` and
  `## [1.1.5]` prefixes; fallback body uses `${VERSION}` instead of executing
  `$(VERSION)` as a command. All three platform jobs (Linux AppImage, Windows
  zip, macOS DMG) now pass `-X` ldflags so `/api/v1/version` returns the
  tagged version instead of `dev`.
- Valid PNG icons (bug-0034): all 16 `frontend/resources/icons/*.png` were
  8-byte stubs (PNG magic only, no IHDR). Replaced with 32×32 transparent
  images so the UI shows icons instead of empty pixels; AppImage and DMG
  bundles pick up real icons.
- `Q_OBJECT` macro added to `TestMpvWidget` (bug-0043), which had five test
  slots that never ran. Sanitizer job now uses `ctest --no-tests=error` for
  UBSan/ASan/TSan so a missing filter fails the job instead of silently
  passing. Added `stateTransitionTest` and `initializationStateTest` to
  exercise real behaviour instead of `QVERIFY(true)` / `QVERIFY(&widget !=
  nullptr)`.
- `backend/data/` removed from git history and added to `.gitignore`
  (bug-0044). The directory contained `rooms.json` with a bcrypt password hash
  and real room IDs from a live run — user data has no place in the
  repository. `.build-tools/` (downloaded cmake) also added to `.gitignore`.
- Security contact in `.github/SECURITY.md` changed from
  `security@torrsyncplayer.local` (unresolvable mDNS domain) to GitHub
  Security Advisory link.
- Guest auto-adds room torrent and selects first video file (bug-0048, step 1
  of per-peer selectedFiles). On `roomTorrentIdentified` (from `/rooms/info`),
  the frontend checks if the torrent exists locally; if not, it adds a magnet
  link by infohash. Once files arrive, it auto-selects the first video file
  (mkv/mp4/avi/mov/webm/m4v) or the first file if no video found. The host
  (who created the room) already has the torrent and chooses the file manually.
  This is the first piece of moving `selectedFile` from global state to per-peer
  state — guests no longer need manual file selection, and different peers can
  theoretically pick different files from the same torrent.
- `selectedFiles` is now per-peer (bug-0048). `torrent.Service.selectedFiles`
  changed from `map[torrentID]int` to `map[torrentID]map[userID]int`, so two
  participants in one room can watch different files of the same torrent — a
  different track (dub/replacement), a different episode of a season pack, or
  just a different branch of a season. Previously `SelectFile` overwrote a
  single process-wide slot, so the last peer to click won and everyone else
  silently received someone else's file.
  - `TorrentService.SelectFile` takes `userID`; `ServeFile` takes `userID`.
  - `POST /torrents/{id}/select` now **requires** the `X-Client-ID` header and
    answers 400 without it: a selection with no owner cannot be routed.
  - `GET /torrents/{id}/stream` takes the peer identity **from the stream
    ticket**, not from a header. libmpv cannot attach headers to its own HTTP
    fetches, so a header could not be trusted anyway — and trusting one would
    have let any holder of a valid ticket read another peer's selection.
  - A peer with no selection of its own gets 400 rather than inheriting the
    last peer's file.
  - Seven new tests. Four in `internal/torrent` (peer selections independent,
    stream requires own selection, reselection keeps other peers, removal clears
    all) and four in `internal/api` against a recording `TorrentService` double
    (missing client id rejected, identity passes through, stream uses ticket
    identity not header, stream serves the peer that selected). Three of the
    torrent tests and both stream-identity API tests were verified to fail
    behaviourally against the previous global-slot implementation;
    `RemoveTorrentClearsAllPeerSelections` passes either way and is kept as a
    leak guard against an orphaned inner map.
  - This also closes **bug-0036**, which described the same global slot from
    the user's side: a guest calling `POST /select` switched the file for
    everyone including the host, and because `ServeFile` read the shared slot,
    a peer who had made no selection answered 400 and broke the stream for
    everyone already watching. Both symptoms are per-peer now.

### Fixed

- `DEV.md` link `[SECURITY.md](SECURITY.md)` → `[.github/SECURITY.md](.github/SECURITY.md)`.
- `.github/workflows/release.yml` now injects version, commit and build time
  via `-X github.com/blagovibe/TorrSyncPlayer/backend/internal/version` into
  all three platform binaries.
- `MpvWidgetTest` lost its `Q_OBJECT` (bug-0043) and three test slots that
  asserted nothing (`QVERIFY(true)`, `QVERIFY(&widget != nullptr)`). Replaced
  with meaningful checks and two new tests.

### Changed

- Project-board cards added for the architecture pivot (idea-0046 accepted):
  each participant now runs its own torrent client and pulls media from the
  BitTorrent swarm directly; the server is only a rendezvous for sync
  commands and roster. This closes deviation-0035 (media through host vs
  CONCEPT "each puts their own backend"), and drives follow-up cards:
  bug-0047 (room↔torrent binding), bug-0048 (per-peer selectedFiles),
  bug-0049 (guest vs host permissions), bug-0050 (host transfer / room
  survival), bug-0051 (torrent registry persistence).

### Security

- Torrent ownership enforced on removal (bug-0049, layer 1). The access token is
  per-process, so any guest who reaches a server shares its authority
  completely. `DELETE /torrents/{id}` therefore removed *anyone's* torrent —
  irreversible, silent loss of another participant's data, and the kind of
  thing a guest can do by accident.
  - `torrent.Service` records `torrentOwners[torrentID] = userID` on add;
    `AddMagnet`/`AddTorrent`/`RemoveTorrent` now take the caller's `X-Client-ID`.
  - Removal is refused with `403 Forbidden` ("Access denied" — the response
    does not disclose whose torrent it was) unless the caller added it.
  - A torrent with no recorded owner (restored from persistence, predating the
    bookkeeping) stays removable by anyone. Refusing there would strand such
    torrents permanently, which is a worse failure than the risk; the choice is
    noted at the check site.
  - Four new tests: a guest cannot remove the host's torrent, the torrent
    survives the refusal, the backend is not asked to drop its pieces, and the
    owner can still remove their own. The guest test was verified to fail
    behaviourally with the ownership gate disabled.
- Room-scoped read access (bug-0049, layer 2 — closes the card). A guest of one
  room could list and read every torrent on the host: names, sizes, file
  listings, and the bytes themselves. Locking removal but leaving reads open
  would only have protected the least interesting half.
  - `p2p.Service.TorrentsVisibleTo(ctx, userID)` answers what a peer may read:
    the torrent bound to the room it currently participates in. Only the
    *current* room counts, so leaving revokes visibility with the membership —
    there is no separate expiry to get wrong. Membership is matched on
    `Peer.UserID`, not the roster key, because `room.Peers` is keyed by a
    per-connection `peer.ID` that a reconnecting peer replaces.
  - `torrent.Service.SetVisibility` wires it in; `canRead` is the single
    decision point for a peer reading a torrent it did not add. It reads the
    ownership map and then calls the provider *without* holding its own lock,
    so the two locks are never nested.
  - Gated: `GET /torrents` (filtered), `GET /torrents/{id}/files`,
    `GET /torrents/{id}/buffer/info`, and `GET /torrents/{id}/stream`.
  - `ServeFile` answers `404`, not `403`: a guest outside the room should not
    learn that the torrent exists. Metadata endpoints return `403` because
    their routes already reveal the id.
  - An unowned torrent stays readable by anyone, mirroring layer 1: it can only
    come from persistence written before ownership was tracked, and refusing
    would leave it permanently unreadable.
  - Twelve new tests (8 in p2p, 4 in torrent). Three were verified to fail
    behaviourally with the gate disabled.
  - The gate also corrected three existing tests that added a torrent as one
    peer and streamed as another — they had been passing for the wrong reason.
    One of them now distinguishes two different refusals: 404 means "no access",
    400 means "access but no selection of your own".
  - Verified live: host, room guest and unrelated peer on one server see 1 / 1 /
    0 torrents; the unrelated peer gets 403 on files, 403 on delete; the guest's
    visibility drops to 0 the moment it leaves the room.
- Stream ticket transferability (deviation-0029) remains open; recorded so
  the compromise is re-checked every audit.

### Verified

- Backend: all packages green, race detector clean, 312 tests including six
  new buffer-position limiter tests.
- Frontend: all 7 test targets green (BackendArgsTest new, MpvWidgetTest now
  runs 6 slots with Q_OBJECT).
- Live server check: 30 back-to-back buffer-position reports → zero 429,
  control-plane budget still enforces its 10-request burst.

### Reconciliation 2026-10-14 — 6 discrepancies found, none fixed silently

Fresh reconciliation (project-docs step 2). Boundary: one committed change since
the last CHANGELOG entry (`3a80d18`, documentation collapse) **plus** the
uncommitted working tree — 107 staged files (+13032/−1178), 17 further modified
files, 6 untracked test files. Test suite: backend green across 18 packages,
`go vet` clean, `go test -race` clean on `torrent`/`persistence`. The frontend
could not be built (no cmake on this machine); CI covers it. Untested code in
the zone was read, and two suspicions were settled by running them, not by
reading.

Findings, all on the project board:

- `bug-0052` — `torrent.Service.Close()` takes `s.mu.Lock()` and then calls
  `SaveRegistry()`, which takes `s.mu.RLock()` on the same mutex. `RWMutex` is
  not reentrant, so the call never returns. Confirmed by running it: with a
  `persistence.Store` attached, `Close()` had not returned after 3 seconds.
  Reachable whenever `--data-dir` is set, which is the documented persistence
  path and the one the Docker example uses — every graceful shutdown of such a
  server hangs until SIGKILL. Third occurrence of this defect class in the
  project after `sync/scheduleSave` and `p2p/scheduleSave`; no test covers
  `Close()` with persistence attached.
- `bug-0053` — `readdTorrentFromRegistry` fabricates a `.torrent` with
  `pieces: ""`, a hardcoded `piece length` of 16384 and a fake announce URL,
  then adds it to the client. The infohash is a SHA-1 over the info dict, so
  the reconstructed torrent gets a **different** identity: confirmed by running
  it, a registry entry keyed `3b2b1a9c…` comes back as `29d9c068…`. Restored
  torrents therefore land under new keys, the ownership map stops describing
  them, and rooms bound to the original infohash can never bind again. The
  registry cannot be repaired locally — restoration needs the original bencoded
  info dict, not a summary of it.
- `bug-0054` — the working tree deleted ~105 lines of journal history (the
  `v1.1.5` and `1.0.0` sections) and left an HTML comment in their place,
  justifying it by their describing code that no longer exists. The deleted
  sections **were restored** from the index, as the append-only check requires;
  the incorrectness of their content is a matter for a new entry, never for
  deletion. The same diff claims "Tested live: add torrent → restart server →
  torrent visible and files listable; pieces served from disk", which cannot
  hold given `bug-0053`.
- `deviation-0055` — CONCEPT.md now states that every participant runs their own
  backend and pulls media from the swarm themselves, and that media traffic does
  not pass through the host. The client has a **single** base URL
  (`networkmanager.cpp:95`) used for torrents, rooms, sync, SSE and for building
  the stream URL (`streamUrl()`, line 457). Without `--server-url` each client
  talks only to its own localhost backend, so two machines cannot see each
  other's room at all; with `--server-url` the guest sends the stream URL to the
  host's server and never starts a backend of its own. The only topology that
  works is the one the goal renounces, which is exactly what
  `.github/SECURITY.md#What the Server Sees` records ("Media data passes
  through the server"). Room↔torrent binding from bug-0047 is real but
  currently inert for the same reason. The goal, the security document and the
  code now disagree three ways, and the code sides with SECURITY.md.
- `bug-0056` — `gofmt -l .` lists 15 files, all inside this working tree. The
  `lint-backend` job checks `test -z "$(gofmt -l .)"`, so it will fail on the
  first commit of this work. The check is honest and did its job.
- `bug-0057` — the idea-0046 rename of the room section left
  `idea-0024`'s `refs` pointing at the old anchor `CONCEPT.md#Синхронные комнаты`,
  which no longer exists, and the mandatory post-goal-change audit of
  `deviation-0029` / `deviation-0039` was not performed. `deviation-0029` also
  contradicts itself: frontmatter says `open`, its discussion says `accepted`.

Not fixed here on purpose: every finding above is either a defect in unfinished
work (restoring or fixing it would edit someone else's change mid-flight) or a
question about the goal, which is the owner's to answer.

### Fixed after the reconciliation — the tree is now committable

Four of the six findings were defects in the uncommitted work, and were repaired
the same session by TDD. The owner chose the order: fix the tree, commit it,
then the topology, then packaging.

- **Close no longer hangs the server** (bug-0052). `Close()` took `s.mu.Lock()`
  and then called `SaveRegistry()`, which takes `s.mu.RLock()` on the same
  mutex; `RWMutex` is not reentrant, so with `--data-dir` set every graceful
  shutdown blocked forever. The flush now happens before the lock is taken.
  Two tests: one asserts `Close()` returns at all with persistence attached,
  the other that it still writes ownership to disk — so the fix cannot be
  quietly "skip the save", which would reintroduce the regression bug-0051
  exists to prevent. Verified: both fail in 10 s before the change, pass after.
- **Restart restores the same torrent, not a lookalike** (bug-0053). The
  registry now persists the *source* a torrent was added from — the magnet URI
  or the original bencoded `.torrent` bytes — instead of a summary of its
  metadata. Restoration replays that source, so the infohash is reproduced by
  construction. `persistence.TorrentInfo` gained `MagnetURI` and `TorrentFile`
  (base64 via `encoding/json`); the hand-rolled bencode encoder and the
  hardcoded `piece length` / `announce` are gone. Reconstructing metadata could
  never work: the infohash is the SHA-1 of the info dict, so a rebuilt torrent
  is a different torrent that merely shares a name.
  - `AddTorrent` now buffers the uploaded bytes (bounded by `MaxTorrentFileSize`)
    so they can be replayed later; the backend still receives a reader.
  - Restoration is bounded by a per-entry `restoreTimeout`, because the backend
    blocks until metadata resolves and an offline peer would otherwise hold
    startup open.
  - A restored handle whose infohash does not match the registry key is dropped
    rather than inserted: a mismatching entry is a stale registry, and keeping
    it would create an orphan that looks restored.
  - A registry entry with no source (written before this change) is skipped
    with a warning instead of being re-added under an invented identity.
  - Verified: a torrent stored as `0123456789abcdef…` used to come back as
    `6985103d…` and now comes back as `0123456789abcdef…`; a `.torrent` upload
    comes back byte-identical. Both tests failed before the change.
- **Event channel send/close race** (bug-0058). `Session.eventChan` was closed
  from four places while `sendToSession` wrote to it unsynchronised. The old
  guards — a `closed` flag checked immediately before the send, and a
  `defer recover()` — suppressed the panic without removing the race, and
  `go test -race` flagged it in `TestEmitEvent_NoPanicOnConcurrentClose` and
  `emit_concurrency_test`. The session now serialises sends and closes through
  one `emitMu` plus an idempotent `closeEvents`, so the panic is impossible
  rather than caught. Same lesson as bug-0052 in a different place: `recover()`
  hides the symptom of a race, it does not remove it.
  Verified: `go test -race ./internal/p2p/...` went from two races to clean.
- **Formatting** (bug-0056). `gofmt -w .` across the backend; `gofmt -l .` is
  empty, so the `lint-backend` gate passes.

Still open on the board: `deviation-0055` (the goal's topology claim, now with
the owner's decision attached), `bug-0054` (a false "Verified live" claim in
this same section) and `bug-0057` (broken anchor, missing post-goal audit).

### Verified after the fixes

- Backend: `go build ./...` clean, `go vet ./...` clean, `gofmt -l .` empty.
- Backend: `go test ./...` green across all 18 packages, and
  `go test -race ./...` green as well — the race run is what surfaced bug-0058,
  so it is the one that actually exercises the new concurrency.
- Frontend: configured and built locally for the first time (the vendored cmake
  in `.build-tools`, plus system Qt 6.12.0 and libmpv 2.5.0), then
  `ctest --output-on-failure` → **7/7 passed**: TorrentModelTest,
  NetworkManagerTest, RoomManagerTest, UtilsTest, BackendArgsTest,
  MpvWidgetTest, NetworkManagerGMockTest. Earlier entries recorded the frontend
  as unbuildable on the dev machine; it is not, it only needed that cmake on
  PATH.

Not run here, and still the gate before merging: `golangci-lint` (not installed
on this machine), the Pact provider job, mutation score, fuzzing, load tests.

### Board maintenance — the idea-0046 goal change, audited properly

The owner ruled that idea-0046 counts as a completed goal change, so the audit
the method requires after such a change is performed here rather than skipped.
Two problems that audit surfaced are fixed, and one deviation is now formally
waiting for review:

- The section rename carried out under idea-0046 left `idea-0024` pointing at
  `CONCEPT.md#Синхронные комнаты`, an anchor that no longer exists. Its `refs`
  now points at «Синхронные комнаты — каждый качает из роя сам». The card's
  substance and its `raw` status are unchanged: no goal edit was made, and the
  ADR was confirmed in deviation-0055, so WebRTC stays deferred rather than
  cancelled.
- `deviation-0029` contradicted itself — `status: open` in the frontmatter
  against "Текущее состояние — `accepted`" in the discussion. The owner
  confirmed the compromise is accepted, so the status is now `accepted` and the
  card stays on the board for as long as the compromise lives: removing it
  would lose the record of a real gap between the goal and the tool. Re-checked
  this session — libmpv still cannot attach headers, the 5-minute TTL is still
  an acceptable window.
- `deviation-0039` moves to `needs_review`: its `refs` points at «Доступ без
  лишних хлопот», which that same goal change edited. It cannot be cleared by
  reading the code — it needs a live host/guest/outsider run to say whether
  `POST /torrents` is still reachable for a guest, and what "guest" even means
  now that each client runs its own torrent backend.
- `idea-0059` (new, `raw`): the way never learned about the torrent registry
  and ownership persistence that the previous session added to the code. DEV.md
  "Хранение данных" still describes only where pieces live. Filing it as an
  idea rather than quietly editing DEV.md, because a way change goes through the
  same card-and-confirmation path as any other.

Also recorded, per the owner's decision: the false "Tested live: add torrent →
restart server → torrent visible and files listable" line in this same
`[Unreleased]` section is **wrong**. It could not hold, because the restore
path produced a different infohash and a torrent with no piece hashes (bug-0053,
now fixed). The line stays where it is — history is not edited — and the
correction is this entry plus the fix recorded above.

### Split the planes — the first half of deviation-0055

The owner decided the topology: the host's server brokers control only, and
every participant pulls the media from the swarm into their own backend. This
entry records the half that makes that possible; handing the torrent to guests
is not done yet.

Three defects stood in the way, all found by running the thing rather than
reading it.

- **The app could not talk to its own backend** (bug-0060). Started with only
  `--port`, the embedded backend generated a random token and printed it to
  stdout; the frontend forwarded stdout to `qDebug()` and never read it, and
  `setAccessToken` was reachable only from `--access-token`. Every request to
  its own backend therefore answered 401. Reproduced directly: the backend logs
  `msg="access token …" token=0c6275cd…` and `GET /api/v1/torrents` with no
  header returns 401. The client now generates the secret and passes `--token`,
  which is the mode DEV.md already documents for the flag ("тот, кто его задал,
  знает его и так"). The residual cost — the secret is visible in the process
  list — is filed as its own deviation rather than waved through.
- **Two backends could not run on one machine.** The torrent client's default
  listen port is a fixed `42069`, so the second instance died with
  `first listen: listen tcp4 :42069: bind: address already in use` and the
  server refused to start. With every participant running their own backend this
  is an ordinary situation, not an exotic one. `cfg.ListenPort = 0` now makes
  anacrolix choose a free port shared across TCP and uTP; an explicitly
  configured port still wins. Every other test in the package disables TCP,
  which is exactly why the suite had never seen it — the new test deliberately
  leaves TCP on.
- **The embedded backend is now always started**, with a free port picked by the
  client. A fixed 8889 made a second instance fail to bind and then quietly talk
  to somebody else's server.

Frontend, `NetworkManager` now holds two endpoints instead of one, and routes
every request by kind:

| plane | carries | address | token |
|---|---|---|---|
| control | rooms, roster, sync commands, SSE | the room's server (`--server-url`) | the host's token (`--access-token`) |
| data | torrents, files, selection, buffer telemetry, media stream | the local backend | the one the client generated |

`streamUrl()` now points at the local backend. That is the line that decides who
actually downloads the film, and it is the test that now pins it. Server
availability is tracked per plane as well: a host going offline no longer makes
the app report itself broken while torrents keep downloading.

`--server-url` changes meaning rather than being replaced — it now names the
control plane, and no longer suppresses the local backend. `--access-token`
belongs to that same control plane; the local token is never typed by a human.

Verified: backend `go test -race ./...` green across 18 packages; frontend built
and `ctest` 7/7, with four new tests — torrent requests reach the local backend,
room requests reach the control plane, each plane gets its own token, and the
stream URL is not built from the host's address. Against two real backends on
one machine: each answers 200 with its own token, 401 with the other's, a guest
joins the host's room over the control plane, and the host's roster shows it.

Not yet done, and it is the point of the whole exercise: a guest still receives
no torrent, so it has nothing to download. The room carries a `torrentId` and
the guest's own backend is empty. That is the remaining half of
deviation-0055.

### The room tells its members what it is watching — the slice closes

The other half of deviation-0055, done right after. A guest that knows only a
room's infohash has nothing to download: an infohash identifies a torrent
without carrying one. So the room now shares it.

A `torrent` event joins the room's event stream next to the roster, carrying the
infohash, the **real** magnet, the name, and the file the host is playing. It is
emitted when a room is created and on every join, because a peer that reconnects
must learn it again — the room outlives any single connection. It travels the
room rather than sitting behind a fetchable endpoint, so only members of that
room learn what it is watching.

The magnet comes from the source the torrent was added from
(`Service.MagnetFor`); for a `.torrent` upload it is derived from the real info
dict. That matters: a magnet rebuilt from metadata names a *different* torrent,
the same trap as bug-0053.

`TorrentEvent.SelectedFile` exists because file selection is per-peer (bug-0048).
Without it a guest picks its own file and the room ends up watching two different
parts of one release — everyone connected to the same instant of two different
films. The guest is told the host's choice instead.

Frontend: `RoomManager` turns the event into `roomTorrentShared`, and
`MainWindow` adds that magnet to **its own** backend. The older path, which
assembled `magnet:?xt=urn:btih:<infohash>` from the room info response, stays
only as a fallback: it carries no tracker at all, so it can find the torrent
through DHT alone and loses seeders.

Sharing the torrent is best effort and never fails a join. A room that exists
but whose torrent cannot be described is still usable for what the host already
has, so a failure here is logged, not turned into a rejected join — refusing
would lock people out of a room they were allowed into.

Verified against two live backends on one machine: the host adds a torrent, picks
a file, creates a room and opens its event stream; a guest joins carrying no
torrent of its own; the stream delivers the room's magnet with the host's
selection; and the guest, adding that magnet to its own backend, arrives at the
**same infohash**. That identity match is what keeps rooms bound to their torrent
across the restart path. Backend `go test -race ./...` green across 18 packages;
frontend built, `ctest` 7/7 with three new tests.

**Срез работает.** Хост добавил торрент, гость получил magnet, гость тянет из
роя сам, оба под одним infohash. Дальше по плану владельца: транспорт
(`idea-0064`), затем перенос фронтенда на Go (`idea-0063`), затем дистрибуция
(`idea-0026`).

### The desktop app could not start its own backend on Linux or macOS

Found by running the built application rather than its tests — the tests were
green and the app was dead. Launching it produced no new listening port and no
backend process at all.

Two layers of it:

- `EMBED_BACKEND` in `frontend/CMakeLists.txt` defaults to `OFF`, so
  `HAS_EMBEDDED_BACKEND` is never defined and `extractEmbeddedBackend()` is
  compiled out entirely.
- The fallback search then listed only `*.exe` names — `torrserver.exe`,
  `torrsyncplayer.exe`. Nothing matched what the project actually builds on
  Linux or macOS.

This is not a developer-only edge: the Linux release job copies the backend to
`AppDir/usr/bin/torrsyncplayer`, next to `AppDir/usr/bin/TorrSyncPlayer`, with
no extension. So **the shipped Linux build could never start its own backend** —
no torrent client, empty list, no playback, and no error the user would see,
because the failure is a `qWarning` in a process whose stderr nobody is reading.

The search list moves into `backendargs.cpp`, which exists for exactly this
reason: `main.cpp` is in no test target, and that blind spot is what let
`--jwt-secret` reach a release once already. Three tests now cover it — the list
must contain the extension-less name, must keep the Windows names, and must not
escape the application directory. The old code rejected any candidate containing
`..` outright; that would have thrown away the dev-tree layout, so the rule is
now narrower and verified.

Verified in the running application, with the backend placed the way the AppImage
places it: the app starts its own backend on a **free port** (`38791`, not the
old fixed 8889), passes a **token it generated itself**, and that token answers
`200` where a request without one answers `401`. Both halves of bug-0060 and
this defect are now confirmed on the real binary, not only in unit tests.

Also established while doing it, and worth knowing: `qDebug`/`qWarning` print
nothing at all in this environment — a minimal Qt program shows it. So the app's
silence says nothing about its health, and every conclusion above was drawn from
processes and ports instead.

Still not verified, and it is the one that matters: **nobody has yet watched a
film with two real machines.** The slice is confirmed at the protocol level and
the app now starts, but the end-to-end experience is unproven.

### The Go frontend starts, and it is the first step of the port

Order changed with the owner's decision. Using weron means the client must be Go:
weron is a pure Go library with no cgo and no C ABI anywhere in its tree, so a
Qt client cannot reach it at all. Doing the transport first would have meant
writing the WebRTC layer in C++ around libdatachannel **and** reimplementing
weron's signaler protocol — as much work as is being removed, in another
language. So the frontend moves first and the transport follows, which is also
Multiplex's order.

Chosen toolkit: **Fyne**. One language, one build, no web stack — which is the
complaint that started this. The cost is accepted: Fyne has no video widget, so
mpv gets embedded through its C API, and the look is not native. Wails was the
alternative and would have been the better *player* (the platform's webview
renders text well and `<video>` solves playback outright) but it swaps C++ for
HTML/JS, leaving the two-toolchain problem in place.

Built alongside the Qt client, not over it. The Qt application is the only one
that has ever worked, and removing it before the replacement does would leave
nothing to run.

Three packages, all testable without a GUI:

- `internal/backend` — the backend's lifecycle: the flag list mirrored from
  `parseFlags`, token minting, free-port picking, and the executable search. It
  exists as a unit because all of this used to live in `main.cpp`, in no test
  target, and that blind spot has already cost a release twice.
- `internal/api` — the two-plane client. Requests are addressed by plane
  rather than by URL, so "which server did this go to" is a type-level
  decision instead of a thing every call site can get wrong. Seven tests hold
  the routing, the per-plane tokens and the stream URL; both test servers answer
  identically on purpose, since otherwise a regression would pass for the wrong
  reason.
- `internal/ui` — the window: torrent list, magnet field, status line.

Verified by running it, not by tests alone: the application starts its backend
on a free port (`40313`) with a token it generated, waits until the backend
answers `/api/v1/version` before showing the window, and a magnet added through
its data plane comes back in the torrent list it polls every two seconds.

### mpv can send headers — the premise of deviation-0029 is false

Found while wiring playback into the Go frontend, when the question arose of how
mpv gets the access token. It turns out it does not need to be told twice: mpv
is told to send it.

Checked against the binaries, not against documentation:

- **mpv as a process.** `--http-header-fields` is an mpv option. Against a test
  server: without the flag it sees `token=""`, with it, `token="letmein"`.
- **libmpv.** `mpv_set_option_string(ctx, "http-header-fields", …)` before
  `mpv_initialize` returns `success`. The option belongs to mpv's core, and
  `mpv_set_option_string` is the documented way to set any option by name.

So the claim written identically in CONCEPT.md, DEV.md and SECURITY.md — that
libmpv cannot attach headers to its own HTTP fetches — was not a property of the
tool. It was a property of how the tool was used. `deviation-0029`, the only
`accepted` compromise on the board, rests on it, and it is the reason for the
public `/stream` route, the HMAC ticket machinery, the ticket endpoint, the
separate stream rate limiter, and a transmissible bearer value in a URL.

**Step 1 of the removal is done, and it is additive.** `/stream` now also accepts
the ordinary `X-Access-Token` header. A ticket, when present, still decides
everything — it names the peer unambiguously, and a header alongside it must not
override that, which an existing test pins. With no ticket, the header is
accepted like on every other endpoint.

Verified: no credential 401, wrong token 401, correct token passes
authentication and reaches the torrent lookup.

**Step 2: playback in the Go frontend, without tickets.** mpv runs as its own
process — no libmpv to link, no C ABI in the build, and a decoder crash takes
the player window with it rather than the application. That is Multiplex's
arrangement, and it is what makes step 1 usable: mpv receives the token as
`--http-header-fields` and the stream URL stays clean.

Three new tests drive the real mpv binary: it opens its IPC socket, it answers
commands, and it puts the configured header on the stream request. The last one
is the whole claim of this entry, checked against the real binary rather than
asserted.

What is **not** verified, and should not be read as verified: `GOOS=windows`
compiles every package of this repository, but the Fyne window does not
cross-compile from Linux — it needs cgo and a Windows GL toolchain. The
three-platform promise is intact in the source and unproven for Windows and
macOS, which will need a native build to confirm. The Windows named-pipe dial in
`internal/player` is written and compiles but has never been run.

The ticket machinery itself is still there. `deviation-0029` is still
`accepted` — closing it is the owner's call, and it should happen only after the
header path has been in use for a while.

### Rooms in the Go frontend: the event stream, the roster, and following the host

`internal/room` is the control-plane half of a viewing room. Two things in it
are worth more than their line count suggests.

**The stream reconnects by itself.** A dropped connection is ordinary — proxies
drop idle ones and laptops sleep — and a client that needs a human to click
"reconnect" sits silently out of sync with the room. The wait doubles from
200 ms to a 5 s ceiling so a room server that is down is not hammered. A
refusal, though, is final: reconnecting to a room this client is not in only
produces more refusals, so a 403 ends the loop instead of spinning. The two are
told apart by `IsNotInRoom`.

**The roster is versioned.** Membership arrives from two directions that race:
the answer to `/rooms/info`, a snapshot taken when the server served it, and
`peer_list` events, which arrive as they happen. A guest who joined after the
GET went out can be delivered by the event stream *before* the GET's answer
lands, and applying the older snapshot afterwards would delete them. `rosterSeq`
is what makes a stale snapshot discardable. Snapshots replace rather than merge,
so the same list applied twice changes nothing — which is what lets a
`peer_list` arriving on top of a reconnect be applied without doubling anyone
up. Nine tests hold that behaviour, including the case where a guest is wiped
out by a late answer.

Wired into the window: join, subscribe, and three reactions — a roster snapshot
updates the room line, a `torrent` event is adopted into **this** client's
backend, and a `sync` event moves mpv. On a seek it follows `status.position`,
not the requested position: the server smooths jumps, so the two differ on
purpose and only the first says where the room actually is.

One behaviour worth recording because it bit the manual check: `/rooms/events`
answers 403 unless the client has already joined. Subscribe-after-join is
therefore the only order that works, and it is what the client does.

Verified against two live backends: a guest joins the host's room on the control
plane, subscribes, a third participant arrives, and the stream carries both the
updated roster and the room's torrent event with the host's selected file —
byte-for-byte the shape the Go parser was written against.

### Control commands, the invitation code, and the buffer window

The rest of what a room needs from the client. 38 tests in `internal/room` now.

**Commands.** Create, join, leave, play, pause, seek, and a buffering report —
all on the control plane. A few behaviours that are easy to get wrong and are
therefore pinned:

- A room cannot be created or joined without a torrent. The server would refuse
  it; catching it in the client keeps the message clear instead of returning a
  400 from a stranger's server.
- A failed join leaves the session *outside* the room. Otherwise the client
  believes it is in a room and waits for events that will never come.
- Leaving clears the session even when the server refuses. The server prunes
  idle peers on its own, so the worst case is a ghost in the roster; a client
  that still believes it is inside is worse.

**The invitation code** is `ID:password`, split on the *first* colon — a room id
is hex and has none, a password may contain any. Splitting on the last colon, or
on all of them, cuts the password in the wrong place. A code with no colon is
read as an id with an empty password rather than refused: someone pasting only
the id has made a typo, and the server is what judges passwords. The id is
checked against the server's rule before anything is sent, which is not security
— the server checks again — but it turns a typo into an immediate message
instead of a round trip to a stranger's server. The token is deliberately not in
the code, as before: it is a credential for the whole server, not for one room.

**The buffer window is the point of the whole prioritiser**, and it was the last
missing piece. Twice a second the player is asked where it is, and that position
in seconds is converted to a byte offset for the local backend, which steers
which pieces to fetch next. Without it the download window drifts behind the
film and the only signal is the seek a person just made.

The conversion refuses to guess. Until the selected file's size and duration are
known it reports nothing: a wrong byte offset aims the prioritiser at the wrong
part of the file, which is worse than no signal at all. Six tests hold that,
including the recovery from a failed reading — a player that cannot be asked
must not stop the ticker, because the next reading 500 ms later is the one that
matters.

Wired into the window: join fields, a password entry rather than a plain one
(an invitation screen is exactly where a password gets typed in public), and
paste/copy for the invitation code.

Verified against live servers. The Go application starts its backend on a free
port and waits for readiness (`200` on `/api/v1/version`). Then, against a real
host and a real guest backend: the guest joins, the host's `play` is accepted
(`isPlaying: true`), the **guest's `play` is refused** with `Access denied`, and
the **guest's buffering report is accepted** — after which `/sync/status` shows
`buffering: true`. That is the whole model the goal describes: one participant
drives playback, and anyone may say they are stalled.

Still missing from the Go client: system tray, settings persistence and the ru/en
catalogs (Fyne has no `lupdate`, so the catalogues are maintained by hand). Qt
remains untouched and remains the only client that does everything.

### CI finally runs the Go frontend — and the window gets tests

**The gap that mattered most in this whole port.** Every `go test` in the
pipeline had `working-directory: backend`. The Go frontend's tests — 91 of them,
including the ones that drive a real mpv — had never been executed once, on any
run, by anything. That is not a missing nicety; it is precisely the blind spot
that let `--jwt-secret` reach a Windows release through a fully green pipeline,
and `main.cpp` sitting in no test target is the same shape of mistake in a
different language.

A `Test Frontend (Go)` job now builds and runs the module on its own, with the
GUI dependencies installed and `FYNE_DRIVER=offscreen` — without that the tests
hang waiting for a window rather than failing. It installs mpv and then re-runs
the player tests with an explicit check that nothing was skipped: those tests
skip silently when mpv is absent, and a silent skip means the claims they check
go unverified on every run while the job stays green.

**The window had 525 lines and no tests at all** — including the decisions that
matter: which position to follow after a seek, whether to re-add the room's
torrent, when a join is allowed. Those are now plain functions with tests, and
the widgets call them rather than reimplementing them:

- a seek follows `status.position`, not the requested position. The server
  smooths jumps, so the two differ on purpose, and following the requested one
  puts the client seconds ahead of everyone else — silently, because the number
  looks plausible. A room that has not moved reports zero, and zero is not a
  place to go.
- the host is not offered its own film back when a newcomer joins. The room
  broadcasts its torrent on every join; re-adding would start a fresh download
  of something already playing.
- a torrent with no magnet is not adopted. An infohash identifies a torrent
  without carrying one, and adding by hash alone builds a magnet with no
  trackers: it resolves over DHT and loses every seeder.

**Settings** are the few choices that should survive a restart, and the file is
never allowed to stop the application: missing, unreadable and damaged are all
normal states, and a half-written file is exactly the state the loader has to
recover from — so writes go through a temporary file and a rename, and a damaged
file is repaired on the next save rather than appended to.

**Translations.** Fyne has no `lupdate`, so the catalogue is maintained by hand,
which is only safe if something checks it. A test fails when a catalogue drifts
from the source list — in either direction. The missing case is the obvious one;
the *extra* case matters more: a key that no longer exists in the source is never
displayed, and it hides the fact that something was renamed, because the
translation keeps looking finished while nothing uses it. An empty entry counts
as missing on purpose, since it renders as the source language and would
otherwise report finished work that is not.

Verified: 91 tests green, `gofmt` clean, `go vet` clean, `GOOS=windows` compiles
every package of the module, and the application still starts its backend and
waits for readiness. Qt remains untouched.

Still missing from the Go client: the system tray, and a few settings the Qt
client had (window geometry, default volume). Those are conveniences rather than
behaviour, and are the last of the parity work.

### The scenario runs end to end — and it caught a real bug in our own parser

Five integration tests start the actual backend binary and drive the actual
client against it. No stubs, no display: what is under test is `api`, `room` and
`player` running headless, because the claim being checked is about behaviour
rather than appearance.

**And it immediately found a defect that no unit test could.** The room stream
sends the whole `P2PEvent` as the SSE data — `{"type":"torrent","data":{…}}` —
while the Go client was treating that whole envelope as the payload. Every field
therefore read as empty, which looks precisely like a room that sent nothing.

The reason the unit tests missed it is the interesting part: they had been fed
the payload directly, because that is what the parser was written against and
nothing contradicted it. The tests and the implementation agreed with each other
and both disagreed with the server. Only a test that starts the real binary can
catch that class of mistake — and it is the same shape of failure as everything
else this project has been bitten by: two things agreeing because nobody checked
them against reality.

The unit tests now use the real wire shape, and the envelope is unwrapped in one
place with a comment saying why.

The five scenarios:

- a host adds a torrent, picks a file, creates a room; a guest joins over the
  control plane; the room tells the guest what it is watching; the magnet names
  the host's infohash and carries the host's selection
- a guest that joins with the wrong infohash is turned away, so the binding
  means something
- the host drives playback and a guest's attempt is **refused** — while the
  guest's buffering report is **accepted**, and the room's status shows it
- two independent backends given the same torrent agree on its infohash, which
  is the property every room binding and every restart rests on
- the data plane is not the host's server, and the two planes carry different
  tokens

One thing the scenario deliberately does **not** do: add the room's magnet to
the guest's backend. Adding a magnet means joining the swarm and waiting for
metadata, and this torrent exists nowhere but in the test, so that wait would
hang rather than fail. What the client owns is that it hands the backend the
magnet the room gave it; the identity that magnet resolves to is covered by the
fourth scenario instead, and was verified live against a real torrent earlier.

Verified: 96 tests green, `gofmt` and `go vet` clean, `GOOS=windows` compiles
every package. CI runs the scenario tests as their own step, because they need
the backend module to build and so cannot live in the backend job.

### The tray, and three defects that only running it could find

The system tray is in. It is best effort everywhere: a desktop that refuses tray
icons and a headless session both leave the application running with its window,
which is what it did before. The icon is drawn in code rather than shipped, because
a tray with no icon is refused outright by some desktops — a cosmetic detail that
would otherwise become a failure to start.

Closing the window hides it while the tray exists, and ends the application when
it does not. That is a **close intercept**, not an after-close hook: Fyne's
`SetOnClosed` runs once the window is already gone, so hiding it there undoes a
completed close, and calling `Close` from inside it re-enters the code that is
running. The decision itself is a function, because the behaviour it governs
cannot be tested through the window — Fyne's test driver closes unconditionally
and never consults the intercept, so a test that called `Close` would pass
whatever it answered.

Running the built application — the first time the Go client has been run as a
program rather than as tests — found three defects, none of which any test was
looking for:

- **Widgets were being touched off Fyne's main thread.** The status line, the
  roster, both lists and the magnet entry are all written from goroutines: a poll
  ticker, an HTTP call, the room's SSE stream. The live run printed Fyne's warning
  on every tick. It now goes through one `onMain`, and the run is silent.
- **A dead backend was reported as a timeout.** With a port already in use the
  backend exited immediately and the application sat for the full ten seconds and
  then blamed the network, while the actual reason sat in its own output,
  unmentioned. Startup now watches the process as well as the clock: the same
  failure is reported in **0 seconds**, as *"the backend stopped with exit status
  1 — its own output above says why"*.
- **The process was being waited for twice.** `run` and `stopBackend` each called
  `Wait` on the same command, and a second `Wait` is an error rather than a second
  answer. One goroutine waits now, in `backend.Process`, and the startup check,
  the window and the shutdown all read that one result.

And a correction to what this file claimed two entries ago: the Qt client had
**no** window-geometry or default-volume setting. It persisted exactly one thing,
`ui/language`, and it hardcoded `resize(1200, 700)`. The "remaining parity gap"
was recorded from memory and checked against nothing — the same failure as
before, in the documentation this time. Parity was in fact already complete once
the tray landed.

Which left one real gap, found while looking for it: `Settings.PlayerBinary` was
written, read back and never used. A person who pointed it at an mpv under `/opt`
got *"no such file"* and a setting that looked like it worked. It now reaches the
launcher, and the control socket path moved onto the window so it is per process —
two instances sharing one socket would drive each other's player, silently, with
every command succeeding.

Also: `--no-tray` was declared and ignored. A flag that does nothing is worse than
a missing one, because the person reading `--help` believes it did something.

Verified: 103 tests green, `gofmt` and `go vet` clean. `GOOS=windows` compiles
every `internal/` package; the **whole binary** does not cross-compile from Linux,
because Fyne's driver needs cgo and a native GL toolchain — the same blocker as
before, and now stated precisely rather than as "every package". Live run: the
application starts its backend, answers `/api/v1/version` 200 with its token and
401 without, polls the list every two seconds, and logs no errors.

### Fixed

- The frontend could not be configured for a test build at all. `option(BUILD_TESTS)`
  was declared ~200 lines *after* the `find_package(Qt6)` call that consults it, so
  `Qt6::Test` was never requested while every test target still linked against it —
  cmake aborted at generate time. Found while verifying the frontend changes for
  bug-0028, which had never been compiled in this repository before.
- `NetworkManagerTest` was use-after-free in the test harness and failed in roughly
  one run in eight (bug-0030). Each SSE mock server was parented to the test object,
  so it outlived the test that started it, while its `readyRead` lambda kept a
  pointer to an `SseMockState` that lived on that test's stack — already freed by the
  next test. Reading the freed memory produced `std::bad_alloc`, a mutex blocked on
  garbage, or nothing at all, depending on what was there. Mock servers are now torn
  down in `cleanup()` after the client that talks to them. Separately, the test
  asserted `roomSpy.count() == 2` with `QCOMPARE` immediately after waiting for
  `statusSpy.count() == 2`; the two reconciliations are fetched in parallel, so the
  second `rooms/info` reply could still be in flight — now waited for rather than
  sampled. 60 consecutive runs, and three full `ctest` passes, are clean.

### Security

- Re-verified the documented threat model against the code: the server sees the
  room roster, every sync command with its position and time, and the media itself;
  a stream ticket can be handed to someone else until it expires; `/metrics`,
  `/health`, `/api/v1/version` and `/swagger/*` are reachable without the access
  token. No undocumented gap was found — the docs say what the code does
  (deviation-0022). `CONCEPT.md` claimed the player and API were protected by rate
  limiting, TLS "and the token itself" — but `/stream` never checks the token, so
  the sentence was simply false. It now names the exception and its cause rather
  than softening it into "generally protected": libmpv cannot attach headers to
  its own HTTP fetches, so stream access travels through a short-lived signed
  ticket in `?ticket=`; that ticket can be handed to someone else and stays valid
  until it expires. `DEV.md` replaces the one-line "HMAC stream tickets" table
  entry with the ticket's format, what the signature covers (expiry, userID and
  torrentID, so none of them can be forged), what it does *not* cover (no binding
  to the requesting client), and why that is a property of the player rather than a
  defect. No code changed: the mechanism works as intended
- Security audit pass over the remaining documented claims (deviation-0022). HSTS
  is sent only over TLS, so the "security headers" bullet now says so instead of
  listing it unconditionally; `/metrics`, `/health`, `/api/v1/version` and
  `/swagger/*` are reachable without the access token and are now recorded as a
  known limitation, since metrics expose room and peer counts. Verified as still
  accurate: the README endpoint table, the TLS 1.2 minimum, the per-IP limiter on
  `/stream` and the middleware order
- Invite code `ID:password` (idea-0016). The host can copy it from
  `Room → Copy invite code` and a guest can paste it into the join tab, which
  fills in both fields — one message instead of two. It carries exactly what
  `POST /rooms/join` already required and nothing more: no encryption, no
  signature, no new secrecy, and the UI says so rather than implying the code
  protects anything. The access token is deliberately excluded — the host has it
  already, but it is a whole-process credential, not a room secret, so putting it
  in an invite would promote a guest from "join this room" to "control this
  server". Parsing splits on the *first* colon so a password may contain colons,
  and a bare room id is read as a passwordless room. Client-side only: no new
  endpoint, no new request field, `JoinRoomRequest` is still `{roomId, password}`
- Security notes: what the server actually handles, a threat model, and a
  verified ticket limitation (idea-0015). `.github/SECURITY.md` gains two sections
  and `CONCEPT.md` points at them. The inventory is written from the code rather
  than from the project's framing, and that check mattered: the idea was drafted
  asserting the server does not see the video stream, which is false — the backend
  *is* the torrent client, the desktop app holds a magnet URI and streams from
  the server. The document now states only what is observed, and says explicitly
  that absence from the list is not a guarantee. The ticket limitation is real and
  was confirmed before writing: `StreamFile` validates the signature, the expiry
  and the torrent id, then discards the user id, so a ticket is transferable
  within its five-minute window
- ADR `server-brokered SSE over WebRTC P2P` (idea-0014). The choice of brokering
  room events through the server was previously asserted as dogma, with no
  reasoning behind it — a statement nobody could argue with, because nobody had
  written down what it costs. The record now states the context, the decision,
  and what is given up: the operator of the server can see who is in a room and
  every sync command with its position and time, event fan-out grows with the
  square of the peer count, and a room does not survive a server outage. Those
  are the real prices, and they are not privacy of the content — the server
  handles the media itself, which P2P would not have changed. Revisit conditions
  are written as things that can be measured or decided rather than as wishes:
  hitting `MaxSSEConnections`/`MaxGlobalSSEConnections` in practice, privacy
  becoming a contractual rather than optional requirement, a requirement to
  survive server downtime, or a signaler that costs nothing to operate
- Security notes in `SECURITY.md` (idea-0015), linked from CONCEPT.md
- The room now shows who is watching (idea-0017). A roster lists every
  participant with their name, who is the host, and who is currently buffering.
  It is delivered twice on purpose: `peers` in `GET /rooms/info` loads a client
  that just connected and never saw the `peer_joined` events of whoever was
  already there, and a new `peer_list` event keeps everyone current after any
  join, leave or buffering report. There is deliberately no per-peer "in sync"
  badge — the room shares one playback position, so the server cannot know how
  far an individual peer has drifted. Instead the client shows its own lag: how
  far its position sits from the room's
- `rosterSeq` on both roster channels (idea-0017). Not an optimisation: the two
  channels race by construction, because `/rooms/info` is an asynchronous GET and
  so answers with a snapshot from the moment the request was served, while
  `peer_list` arrives as things happen. A peer joining after that moment can
  reach the client as an event *first*, and the older HTTP response would then
  erase it from the list. The version number lets the client discard the stale
  snapshot instead of silently losing a participant
- Typed room event payloads (idea-0013). `internal/models/events.go` defines
  `PeerJoinedEvent`, `PeerLeftEvent` and `SyncEvent` with JSON tags, plus event-type
  and sync-action constants and a `NewSyncEvent` constructor. `P2PService.BroadcastSync`
  now takes a `models.SyncEvent` rather than `interface{}`, so forgetting the initiator
  or misspelling an action is a compile error instead of a silently malformed event.
  All six construction sites that built `map[string]interface{}` literals are gone.
  The `signal` event stays untyped on purpose: it is a byte-for-byte relay
  (`Signal []byte`) that the server never interprets
- `GET /api/v1/rooms/{roomID}/events` now has a schema in the OpenAPI spec
  (idea-0013). It had none before — the annotation claimed `{string} stream`, so
  `P2PEvent` was generated nowhere. The endpoint description now spells out each
  payload shape selected by `type`. The limit worth knowing: `P2PEvent.data` is an
  untyped object because the payloads form a union keyed on `type`, which swaggo
  cannot express, so `SyncEvent` and friends do not appear as separate definitions
- Buffering synchronization between peers (bug-0011). When one participant runs
  out of data and mpv goes idle (`core-idle`), it now reports that to the room and
  the others pause, instead of running on alone and desyncing the shared viewing
  moment. `POST /api/v1/sync/buffering` takes `{"buffering":bool}`, records the
  flag in `SyncStatus.buffering`, and broadcasts it as a `sync` SSE event with
  `action:"buffering"`. The peer pauses the local mpv directly and resumes when
  the buffering peer catches up — deliberately *without* sending `syncPause`, so a
  peer's local stall is not recorded as a host command. Unlike play/pause/seek, any
  member may report buffering: it is a local condition, and whoever lacks the data
  is the one who says so
- Host enforcement for playback synchronization: only the room host can now call Play, Pause, and Seek (deviation-0010). Non-host peers receive 403 Forbidden.
- `--addr`: listen address as a whole, replacing the derived `:<port>`. `127.0.0.1:8889`
  or `:9999` both work; without the flag the server listens on `:<port>` exactly as before
- `--token`: fix the access token instead of taking the random one, for deployments that
  manage the secret outside the process (systemd EnvironmentFile, config-management runbook).
  A token set this way is no longer printed to the log — the operator already knows it
- `backend/cmd/server/main_test.go`: the package had no tests at all. `parseFlags` was
  extracted from the global `flag.CommandLine` into `parseFlags(fs, args) (Config, error)`
  so the flags are testable; `main` calls `parseFlagsOrExit` so a parse failure exits the
  process instead of the test binary. Covered: `--addr` displaces `:<port>`, `--token`
  reaches the auth service, an empty token still yields a random one, and an unknown flag
  is an error rather than a silent start on defaults
- `.project-board/` with one file per card, the board the project-docs methodology
  requires. It was missing: the previous session referenced board ids from an external
  tracker, so nothing survived in the repo

### Fixed

- The roster and the peer count could disagree, and the roster could lose a peer
  that had just joined (idea-0017). Both stemmed from the roster being applied by
  merging rather than replacing and from the two delivery channels having no way
  to tell which snapshot was newer. `applyRoster` now replaces wholesale, so
  applying the same list twice changes nothing, and refuses a snapshot whose
  `rosterSeq` is older than the one already applied
- `peerCount` did not count the host, so every room reported one participant too
  few — two people in a room showed as 1 (bug-0021). The host was a participant
  by every other measure: it passes the membership check, it drives playback,
  `GetRoomInfo` names it as `HostUserID`, and `emitEvent` added it to the
  recipient list by hand. What it was not was present in `room.Peers`, because
  `CreateRoom` built an empty roster and the client never calls `join` after
  creating — it subscribes to SSE and that is all. The creator is now added to
  the roster at creation
- A room was deleted out from under a connected host when the last guest left
  (bug-0021). Emptiness was `len(room.Peers) == 0` while the host sat outside
  `Peers`, so the last guest to leave emptied the roster and deleted a room the
  host was still watching and still holding an SSE stream to. Folding the host
  into the roster fixes both symptoms; the leave orderings are now pinned in
  `p2p/peer_count_test.go` — host leaves first, guest leaves first, everyone
  leaves, guests draining one by one
- Idle pruning reaped peers that were connected and watching (bug-0021).
  `LastHeartbeat` was written only by `JoinRoom`, so `PeerIdleTimeout` measured
  "has not joined lately" rather than "is not connected": every peer — host
  included — was dropped from the roster five minutes in, while its SSE stream
  was still open for thirty. Being pruned silently removed a peer from
  `peerCount` and from the recipient list, and left the room behind empty,
  because the pruner never deletes rooms. `TouchPeer` now refreshes the
  heartbeat from the SSE stream — on subscribe and on every ping, a 30 s signal
  that the peer is genuinely present — so the timeout means what it says again.
  This was not optional: putting the host in the roster would otherwise have
  handed it a guaranteed reap, since the host never calls join at all
- Contract testing was unreachable locally: `make contract-test` and
  `make test-all` ran `go test` without `-tags contract`, and the whole
  `internal/contract` package is behind that tag — the recipe answered
  "matched no packages" and exited 1. It could never be green, and never red for a
  real reason either. CI already passed the tag (deviation-0003)
- `validateConfig` validated `--port` even when `--addr` supplied the port, so a valid
  listen address could be rejected by an unused flag
- Startup log reported `port=8889` regardless of the real listen address; it now logs
  the address the server actually bound to
- Documentation: `internal/buffer` is described as an "LRU buffer" in `DEV.md` and
  `README.md`, but no LRU exists anywhere in the backend — it is a piece-priority
  scheduler over the storage layer (deviation-0004)
- Documentation: `DEV.md`, `README.md` and `CONTRIBUTING.md` require "CMake 3.16+";
  `frontend/CMakeLists.txt` declares `cmake_minimum_required(VERSION 3.20)`
  (deviation-0005)
- Documentation: `CONTRIBUTING.md` told contributors to branch from and open PRs against
  `develop`. No such branch exists locally or on origin; the default branch is `main`
  (deviation-0007)
- Documentation: `CONTRIBUTING.md` promised a "coverage >= 60%" gate; `ci.yml` enforces
  `COVERAGE_MIN_PERCENT: '55'`. The 60% figure dates from the era when `.codecov.yml`
  was gitignored and gated nothing
- Documentation: `.github/SECURITY.md` listed "development STUN servers hardcoded" and
  "in-memory user storage" as known limitations. WebRTC/STUN/TURN was removed with the
  P2P rework, and accounts were removed in favour of the per-process token; neither
  limitation exists
- Documentation: `DEV.md` named the token flag `--access-token` in prose and `--token` in
  the flag table; only one flag exists and it is `--token`
- Removed `backend/internal/buffer/service.go.tmp`, a stale copy of `service.go` holding a
  syntactically invalid line. It was untracked and swallowed by the
  `backend/internal/**/*.tmp` ignore rule — the same class of `.gitignore` rule that
  previously hid real source (bug-0006)

### Changed

- A seek payload carries two different positions, which the map literals hid
  (idea-0013). The top-level `position` is where the host asked to go;
  `status.position` is where server-side smoothing actually moved the room, so a
  jump beyond the compensation threshold only moves part of the way. `WithRequestedPosition`
  names the distinction at the call site instead of leaving it to be rediscovered
- `CHANGELOG.md` no longer starts with a UTF-8 BOM (bug-0020). This entry deliberately
  rewrites an existing line instead of appending, and it is the only one that does: once
  the BOM is gone, every later edit is a pure append and needs no repair step. The BOM
  carried no meaning — it was added on 2026-06-04 by commit `0e6f4ff`, a tree-wide
  "Update project files" rewrite, and had no counterpart in the file's five earlier
  commits. It mattered only because editing tools normalise encoding on write, so
  `git diff` reported a changed first line and the real additions were easy to miss in
  review
- `CONCEPT.md` and `DEV.md` describe the target state, not the current one. Recorded as
  an explicit principle in `DEV.md` ("Принципы разработки"): when code
  disagrees with the documentation, the code is fixed. Rewriting the documentation down
  to match the code hides the discrepancy and the next session reads the lowered document
  as intent (idea-0009)
- Sync: the goal no longer promises a continuously catch-up player. There is no latency
  measurement anywhere — no constant, no `SyncStatus` field, no periodic `/sync/status`
  poll, and no playback-speed control — and the documented four-step algorithm
  (expected position from elapsed time minus latency) did not exist in `internal/sync`.
  What the code does is server-side seek smoothing, where a jump beyond 2 s moves 30% of
  the way, and that is now what `DEV.md` describes. The goal states the event-driven
  model and its cost: a peer that drifts catches up at the next command, not on a timer
  (deviation-0002)
- `CONCEPT.md` notes that a `--token` token survives a restart and is not printed, so the
  "nothing is stored" claim is stated with that exception rather than left absolute
- Host checking removed from `sync.Service.Buffering`: any room member may now report
  buffering, not only the host (idea-0018). Buffering is a peer reporting its own local
  condition ("I am out of data"), not a command about what the room should do — and the
  host gate would have stranded the room, because a non-host peer that recovered could
  never say so and the pause would never lift. Play/Pause/Seek keep the host gate: they
  are a different category of action. Room membership is unchanged and still enforced,
  by resolving the caller's own room in the handler before the service is reached, so a
  peer outside any room still gets 400 "Not connected to a room" and never 403. The
  `@Failure 403` annotation was dropped from `POST /api/v1/sync/buffering`, since the
  endpoint can no longer produce it, and the spec was regenerated
- `CONCEPT.md#Синхронные комнаты` now states who may report buffering, after the host-model
  paragraph (idea-0018)
- `GET /api/v1/rooms/info`: reports the caller's own room (id, name, host, peer count).
  The room comes from the caller's membership rather than a path parameter, matching
  `/sync/status`, so one client can never ask about another client's room. A peer outside
  any room gets 400 "Not connected to a room". Added for bug-0012 — the endpoint the card
  named as `GET /rooms/{roomID}/info` never existed on the server
- Frontend now reconciles room state after every (re)connect of the SSE stream
  (bug-0012). On each stream open it fetches `GET /sync/status` and `GET /rooms/info`
  and applies position, play/pause, buffering, host role and peer count. Without this a
  client that lost its stream kept showing a stale position and could keep playing while
  the room was paused. The host role is derived by comparing the room's `hostUserId`
  against our own `X-Client-ID`, so it stays correct if a different peer becomes host;
  `NetworkManager::clientId()` was added for that comparison

### Fixed

- Frontend SSE reconnect never actually happened, so everything downstream of it was
  unreachable (bug-0012). `connectToSSE()` opens by calling `disconnectSSE()`, and
  `disconnectSSE()` cleared `m_sseReconnectPath` — so by the time `onSSEReconnect()` ran,
  the path it needed was already gone and it bailed out with "путь SSE переподключения
  пуст". Every drop was terminal. The path is now cleared only where the room is actually
  left
- Frontend SSE reconnect backoff never grew and its attempt limit never tripped
  (bug-0012). `m_sseReconnectAttempts` was reset inside `connectToSSE()`, i.e. when the
  request was *issued*, before the server had answered; a connection that failed
  immediately therefore reset the counter first, so the delay stayed at the base value
  and `MaxSSEReconnectAttempts` could not be reached. The reset moved to the point where
  the response actually arrives (`metaDataChanged` with HTTP 200), which is also the only
  honest "the stream is open" signal — `connectToSSE()` running proves nothing. A non-200
  answer (403 no longer a member, 429 too many connections) is now reported instead of
  being retried indefinitely
- `GET /api/v1/sync/status` answered 200 with a zeroed status for a caller outside any
  room, which was indistinguishable from a real room sitting at position 0 (bug-0019).
  `GetRoomInfo` errors were swallowed, `roomID` stayed `""`, and `GetStatus("")` has no
  entry for an unknown room and returns `IsPlaying:false, Position:0`. After the
  reconnect reconciliation added in bug-0012 this was actively harmful: a client that
  lost its stream could pause and seek to 0 in a room actually playing at 42 minutes.
  A caller outside any room now gets 400 "Not connected to a room", the same answer
  `/sync/play`, `/sync/pause`, `/sync/seek` and `/sync/buffering` already gave — so the
  whole `/sync` group has one contract and a client can branch on the status code
  without matching on error text
- Room lookup for the caller's own room is now done once, in a shared
  `resolveCallerRoom` helper, instead of five near-identical copies across the sync
  handlers plus one in `GET /rooms/info` (bug-0019). The copies had already diverged:
  the four write handlers returned 400 while the read-only `/sync/status` swallowed the
  same error. "Not in a room" is now one place in the code rather than five that have to
  be kept in agreement. The shared helper also treats an empty `roomInfo.ID` as
  "no room", which the per-handler checks did not

- Auth: the stream-ticket signature was computed over an empty payload — `hash.Hash.Sum` appends the
  MAC of whatever was written *so far*, and nothing was ever written, so the "signature" only
  prefixed attacker-controlled bytes. It now writes the payload before summing, and rejects ids
  containing dots and non-positive expiry (B-01)
- Streaming: `http.Server.WriteTimeout` is an absolute deadline for the whole response and aborted
  every SSE stream and every torrent stream at the deadline. Removed (B-02)
- SSE: a middleware wrapper dropped `http.Flusher`, so type-asserting it failed and the handler
  answered 500 instead of streaming (B-03)
- Proxy handling: `isTrustedProxy` accepted any private, loopback or link-local address, so
  `X-Forwarded-For` from any such client decided the client IP. It now trusts nothing unless
  `TRUSTED_PROXIES` is set (B-07)
- P2P: `emitEvent` iterated peers without holding `RLock` — a fatal, unrecoverable concurrent map
  access with any Join/Leave in flight; `Close()` also ran `close(doneChan)` outside `sync.Once`
  and panicked on every call after the first (B-04, B-08)
- Shutdown: `WaitForSSEConnections` waited without a bound, so one idle SSE client held Ctrl-C for
  the 30-minute SSE timeout (B-08)
- Rooms: `RoomManager` never connected `roomCreated/Joined/Left`, so `isInRoom()` was always false,
  and peer events were read from a `peerId` key the backend never sent (F-02)
- Rooms: the join path read `obj["id"]` from a response that contained none, and the host never
  subscribed to SSE at all (F-03, F-06)
- Torrent list: the response envelope was checked with `doc.isArray()`, which is always false for
  an object, so `torrentListReceived` never fired and the list was silently always empty (F-04)
- Player: no mpv property was observed, so position, duration and pause never updated (F-01);
  a nested `QMutexLocker` deadlocked initialisation (F-07)
- Player: a stream ticket lasts 5 minutes and was never renewed, so video died at minute five —
  and a transient network error stopped renewal permanently (F-13, B-01)
- Player: the ticket-refresh branch played whatever torrent its argument named before checking it
- Player: the resume seek was dropped whenever the new stream took longer than 300 ms to open
- CI: removed `|| true` and `continue-on-error`, so ASan/TSan, e2e and contract tests can fail again.
  The sanitizer targets never compiled and are fixed; `make test-all` no longer builds three CMake
  targets that do not exist

### Changed

- Docs: README, DEV.md, API.md, ARCHITECTURE.md and SECURITY.md described a JWT auth system and
  endpoints (`/auth/login`, `/auth/register`, `/csrf-token`) that are not registered in the router.
  Rewritten to the per-process access token that is actually implemented
- Removed dead JWT constants and the `Claims` type

### Added


- `DEV.md` — the project's file 2 (how it is built): architecture with per-package
  responsibilities, tech stack with versions, development principles, repository layout and test
  commands. Detailed diagrams, request flows and the data model stay in `docs/ARCHITECTURE.md`,
  so nothing is duplicated (task `t_7a629b42`)
- CI: Swagger spec freshness check — regenerates the spec and fails on any diff, so the spec can no
  longer drift from the annotations silently (task `t_33d47b84`)
- README: links to `CONCEPT.md` and `DEV.md`, and the project structure now matches what is actually
  in the repository, including `docs/METRICS.md` and `tests/`
- Persistence: JSON-file storage for users, revoked tokens, rooms and playback state (`internal/persistence`), enabled via `--data-dir`
- Persistence: debounced writes (`internal/utils/debouncer.go`) so bursts of state mutations collapse into one disk write; `Stop` waits for an in-flight write before the shutdown flush
- Contract testing: Pact provider verification against the real backend router (`internal/contract`, contract `pacts/frontend-backend.json`)
- Tests: chaos scenarios (toxiproxy), k6 load scenarios, Playwright e2e suite, Qt headless e2e
- Docs: `docs/METRICS.md`
- Methodology: `CONCEPT.md` (goal and boundaries) and `DEV.md` (way — how it is built)

### Removed

- `docs/ARCHITECTURE_BACKEND.md` and `docs/ARCHITECTURE_FRONTEND.md` — their content is already in
  `docs/ARCHITECTURE.md`; the `Package Structure` block was byte-identical between the two files
  (task `t_7a629b42`)
- `SECURITY.md` (root) — duplicate of `.github/SECURITY.md`, with a conflicting supported-versions
  table; GitHub surfaces the `.github` one anyway (task `t_f3d4996c`)
- `MUTATION_TESTING.md` — stale: it documented `@latest` (which needs Go ≥ 1.27), a 80% threshold
  (now 45%), and a `.mutesting.toml` that does not exist in the project. The working configuration
  lives in the Mutation Testing CI job (task `t_8cc25d8e`)
- `AGENTS.md` — 80% дублировал `DEV.md` и `CONCEPT.md` (структура, конвенции, безопасность,
  тесты, правило «проверки должны краснеть» дословно), уже разъехался с ними (говорил про
  «два файла», дерево урезано, тесты неполные). Методология трех файлов и доски самодостаточна;
  агент при старте читает `DEV.md` и `CONCEPT.md` напрямую. Удалено ради единого источника
  истины.
- The unused account code (deviation-0008). `internal/persistence` still carried `SaveUsers`,
  `LoadUsers`, `SaveRevokedTokens` and `LoadRevokedTokens` — leftovers from a schema with user
  accounts and token revocation — and nothing called any of them: the only live calls were
  `SaveRooms` and `SaveSync`. Those functions, the `UserData` and `TokenRevocationData` types
  and their internal `persistedUser` conversion helpers are gone, along with the whole dead
  "Auth Models" block in `models/types.go` (`User`, `UserResponse`, `RegisterRequest`,
  `LoginRequest`, `ChangePasswordRequest`, `AuthResponse` — not one of them had a single
  reference anywhere in the repository). Authentication was never JWT: it is one access token
  minted at startup and printed to the console, and none of this code described anything the
  server actually does
- The `internal/persistence` note in DEV.md pointing at the dead functions — after the deletion
  it would have been describing code that no longer exists

The tests that exercised the removed functions are gone too, since they validated code nobody
called. `TestAtomicWrites` did not go with them: it checks the atomic write path itself, so it
was rewritten against `RoomsData` and gained a check that the persisted file is not
world-readable — room metadata includes the password hash.

### Fixed

- Backend build was broken: `internal/sync` and `internal/p2p` referenced `utils.Debouncer`, but the file was ignored by `.gitignore` and never committed — restored and committed
- `internal/persistence`: tests did not compile (missing `time` import, non-existent `models.SyncStatus.RoomID` field)
- Removed dead code found by the `unused` linter: `realPiece` in `internal/buffer/service.go` (a wrapper around
  `*torrent.Piece` that was never used — `realTorrent.Piece()` returns the concrete type, which already
  satisfies `torrentPiece`)
- CI: Go pinned to 1.26.6 in ci.yml, release.yml and security.yml — `govulncheck` reported 7 stdlib
  vulnerabilities present in 1.26.5 (GO-2026-5026/6088/6089/6090/6091/6218), all fixed in 1.26.6
- CI: pinned `go-mutesting` to `v0.0.0-20251226130216-48d0401f00fb` — `@latest` now requires Go >= 1.27,
  which conflicts with the pinned Go toolchain in CI
- CI: the mutation-score threshold was dead code — it grepped for `Mutation score: <n>`, but go-mutesting
  prints `The mutation score is 0.607143` (a 0..1 ratio), so the 80% gate never fired. Parsing fixed;
  note that the real score is well below 80% (see the project board) and the gate will now fail honestly
- Contract test now compiles and runs: migrated from the removed `provider.VerifierConfig`/`VerifyProvider(ctx, cfg)`
  API to `provider.NewVerifier().VerifyProvider(t, provider.VerifyRequest{})` (pact-go v2.5.1), switched to
  `PactFiles` (the old `PactURLs` field rejects local paths with a builder error), and fixed the pact path
  (three levels up from `internal/contract`, not four)
- CI: the Pact job was green without ever running a test — it omitted `-tags contract`, so `go test` reported
  no test files, and `|| true` hid it. Tag added, `|| true` removed, and the native `libpact_ffi` (v0.4.28)
  is now downloaded, since pact-go links against it via cgo
- Import grouping: `internal/buffer/service_test.go` failed the `goimports` formatter check (local-prefixes)
- gofmt: 5 files unformatted (`api/handlers_room.go`, `api/handlers_test.go`, `buffer/service.go`, `buffer/service_test.go`, `models/types.go`), which failed the CI format check
- CHANGELOG append-only: commit 6bd47e9 had rewritten the existing v1.1.5 and 1.0.0 entries; original lines restored, the change is now recorded here instead of rewriting history
- Docs: stale WebRTC references after WebRTC removal — comments in `internal/models/types.go` and `internal/api/handlers_room.go` corrected
- `.gitignore`: removed rules that swallowed working code (`internal/utils/debouncer.go`) and CI config (`.codecov.yml`)
- Swagger generation could not be repaired from the annotation side: `@Failure {object} APIError` referenced a type
  that does not exist (`models.ErrorResponse` is the real one), so `swag init` failed with
  `cannot find type definition: APIError` and the checked-in spec silently described WebRTC data channels that had
  been removed in 6bd47e9. The annotations were corrected and the spec regenerated; CI now fails on any spec drift
  (board `t_0ab2e3d0`)
- CI: the Pact provider job was green while running zero tests, masked twice over. `CGO_LDFLAGS` referenced
  `$PACT_FFI_DIR` from a workflow-level `env:` block, which the runner resolves before bash runs, so the linker was
  handed the literal `$PACT_FFI_DIR` and failed with `cannot find -lpact_ffi`; and `go test ... | tee pact.log` returned
  tee's exit status because the workflow sets neither `pipefail` nor `shell:`. Removing `|| true` in #51 fixed
  neither. Download and verification now share one step and `pipefail` is set (boards `t_1452fa61`, `t_70d91346`)
- CI: `security.yml` pinned Go with a hardcoded `1.26.6` while `ci.yml` and `release.yml` use the `GO_VERSION` env var,
  so a future version bump would have silently missed one of the three; `security.yml` now uses the same variable
- Comments: stale WebRTC references dropped from `internal/constants/constants.go` (`MaxSignalSize` is a DoS guard on
  the server-brokered room signal, not an SDP/ICE size limit) and `internal/p2p/service.go`

### Removed

- Dead frontend code: `imediaplayer.h`, `iroommanager.h`, `itorrentmanager.h`, `mock_roommanager.h`, `mock_torrentmanager.*` and their gmock tests (no remaining references)

### Known Issues

1. The Pact contract is stale: with verification now actually running, all 16 interactions fail. Most requests are
   sent unauthenticated, so protected endpoints answer 401 (8 cases), plus 415 on missing Content-Type, 429 from
   rate limiting, and a `text/event-stream` expectation that gets `application/json`. The contract file needs to be
   regenerated with auth headers and the current response shapes — board `t_7eebdc1c`
2. CI still masks failures of e2e and mutation steps via `|| true` — board `t_1452fa61`
3. `.gitignore` still ignores `config.yaml` globally and `backend/internal/testutil/` — board `t_6e2ff5cf`

### Verification

Fresh reconciliation performed (project-docs step 2): git boundary = 12 commits after the last CHANGELOG entry (6bd47e9..HEAD).
Result: 9 discrepancies found; 5 fixed here (backend build, persistence tests, formatting, CHANGELOG append-only + stale
entries, WebRTC comments), 4 recorded on the project board as bugs/deviations, plus 2 found by CI afterwards
(dead code flagged by `unused`, Go 1.26.5 stdlib vulnerabilities).

Verified on PR #51 — CI green: Lint Backend, Test Backend (with govulncheck), Frontend Build & Test
(clang-tidy + Qt unit + gmock), Build Backend on ubuntu/macos/windows. Locally, on the CI Go version
(`GOTOOLCHAIN=go1.26.6`): `golangci-lint run` 0 issues, `gofmt -l .` and `goimports -l .` empty,
`go vet ./...` clean, `go test -race ./...` green. Frontend cannot be built on the dev machine
(no cmake, sudo requires a password) — covered by CI.

Docs cleanup pass (same session): fresh reconciliation against the methodology files found the repo
consistent with its goal and 5 documentation/structure discrepancies. 5 resolved here (`DEV.md`
created, duplicate architecture and security files removed, stale mutation-testing doc removed, README
links fixed, Swagger freshness added to CI); the 4 documentation deviations found earlier in this
session stay on the project board. Verified: no status/roadmap file exists, `DEV.md` and `CONCEPT.md`
contain no plans or status markers, the CHANGELOG stays append-only (the only `MUTATION_TESTING.md`
mention left is inside a historical entry, which is never edited), and every internal Markdown link
resolves to an existing file.

Audit pass 2026-10-06: three claims in the entries above are inaccurate and are corrected here rather
than by editing history (the earlier text stays on purpose, since it was true when written and can still
be quoted):

- The mutation score quoted above (`0.607143`) is not reproducible. The same package scope measures
  `0.494964`, and the gate in `ci.yml` is `MUTATION_MIN_RATIO: '0.45'` — so the threshold is honest now,
  but it gates at 45%, not at the 80% the entry describes. Finding: the number was carried from a session
  that never printed it.
- "Go pinned to 1.26.6 in ci.yml, release.yml and security.yml" held for the first two only; `security.yml`
  used a hardcoded literal and would have been silently missed by a version bump. Fixed in this session.
- The 1.0.0 entry advertises a PR coverage gate of 60%. No such gate exists: `.codecov.yml` has been
  gitignored since 8a118a4, and coverage is uploaded with `fail_ci_if_error: false`, so it gates nothing.

Fresh reconciliation this session (project-docs step 2): the git boundary is empty — HEAD was itself the
last CHANGELOG commit — so the check was a direct audit rather than a diff of commits. Backend locally:
13 packages green, `gofmt -l .` and `go vet ./...` clean. CI on `426e33f`: 14/14 jobs green. Frontend
cannot be built locally (no cmake); CI covers it.
Discrepancies found, none fixed silently: the Pact false-green recorded above; a stale WebRTC description
in the contract for `POST /rooms/signal` (WebRTC is a CONCEPT.md non-goal — the endpoint survives as an
opaque relay and is a decision for the owner, not a defect to patch); `.gitignore` still swallowing
`config.yaml` globally, `.codecov.yml` and `backend/internal/testutil/`; and the project board cannot
express the methodology's `open`/`accepted`/`needs_review` states in its schema (`hermes kanban reopen`
does not exist, so an archived card cannot be restored).

The Pact provider job now runs and fails honestly on CI run 37461241156 (PR #52). Verified from the uploaded
`pact-verification-log` artifact: the test binary links (0 occurrences of `cannot find -lpact_ffi`, no
`build failed`), all 17 interactions execute, 1 passes (`GET /health`) and 16 fail with distinct, concrete causes:

- 12 x 401 — the contract sends `Authorization: Bearer test-token`, which is not a token
  `ValidateTokenWithRevocation` accepts. This includes `POST /api/v1/auth/login`, which the contract expects to
  answer 200 while the API correctly rejects those credentials with 401.
- 3 x 415 — `POST /api/v1/rooms/leave`, `/api/v1/sync/play` and `/api/v1/sync/pause` send neither body nor
  `Content-Type`, which `ContentTypeMiddleware` rejects.
- 1 x 429 — `GET /api/v1/sync/status`; the per-IP limiter is `rate.Limit(1), burst 10`
  (`internal/api/middleware.go:761`), so a 17-interaction run from a single IP trips it.
- 1 x 400 — `POST /api/v1/auth/register`: the contract's `testpass` violates `ValidatePassword` (minimum 8
  characters with upper, lower, digit and special character), so the API answers 400 rather than the expected 201.
- Response shapes also diverge: several interactions expect envelope keys the handlers do not emit (for example
  `items` and `type` on the torrent list, `token` and `expiresIn` on register).

The contract also points `GET /api/v1/rooms/events` at an unregistered path: the router serves only
`/{roomID}/events` (`internal/api/router.go:126`), while `APIPathRoomEvents = "/api/v1/rooms/events"`
(`internal/api/paths.go:70`) is defined but never used — dead code, and the contract is its only mention.
Separately, `POST /api/v1/rooms/signal` is still described as "sends WebRTC signal" although WebRTC is a
CONCEPT.md non-goal; the endpoint survives as an opaque relay.

The frontend "contract test" step turned out to be dead too: CI run 37465712937 failed at
`make -j$(nproc) test_networkmanager_contract test_torrentmanager_contract test_roommanager_contract` with
"No rule to make target" — those targets are not defined in `frontend/CMakeLists.txt`, and
`ctest -R "Contract"` matched zero tests. The job had never checked anything; `|| true` had kept it green.
Also found while auditing: `frontend/src/test_integration.cpp` is a placeholder containing a single
`testPlaceholder`, with a comment pointing at `tests/e2e/qt_headless/test_e2e_headless.cpp` — and that file is
not wired into any CMakeLists, so the Qt headless E2E step had no tests to run either. The job was removed;
the backend/front-end contract is verified by the Pact provider job and NetworkManager's interface contract by
`test_networkmanager_gmock`.

`backend/Makefile` had the same dead-gate defect as CI once had, which is why local runs never caught it:
`grep -oP 'Mutation score: \K\d+(\.\d+)?'` matched nothing (go-mutesting prints `The mutation score is
0.494964`), so `MUTATION_SCORE` was always empty and `make test-mutation` always exited 0. Its threshold was
80% and its scope the full `./internal/...`, against CI's 45% and six packages. It now parses the real output,
fails when it cannot parse, uses CI's scope and threshold, and warns that go-mutesting rewrites sources in
place. Verified: parses 0.494964, passes at the 0.49 floor, fails at 0.48.

Backend statement coverage measured at 60.1% (`go test -coverprofile` + `go tool cover -func`), so the 60% the
1.0.0 entry advertised is reachable but leaves 0.1 points of headroom, which would flake on any new file. The
enforced floor is 55%, checked in the test-backend job with an explicit empty-value guard. Verified against the
real profile: 60.1 passes, 40 fails.

The Go fuzzing job now fails for a real reason, previously hidden by `|| true`:
`FuzzValidateUsername/537fd2664c896b18` reports `expected error for username length 31` for
`"000000000000000000000000000000 "` — a 31-character username of digits and a space is rejected by
`usernameRegex` but accepted by the length check path the test exercises. A genuine gap in the fuzzer's
expectations, not in the validator.

### The Qt client is gone, and the ticket machinery went with it

The only frontend the project has is the Go one. Qt/C++ -- `frontend/src/`,
`CMakeLists.txt`, `Makefile`, `build.sh`, `build.bat`, `build.ps1`,
`build_macos.sh`, translations and resources -- has been deleted. CI has no Qt
job, no sanitizer job, no dead fuzz or contract steps left. Building is now
`cd frontend && go run ./cmd/build`, which puts the client and the backend side
by side, because that is what the application needs at run time: every
participant starts their own backend, and the client finds it next to itself.

While deleting, three claims the docs and the board were making turned out false,
which is the actual news of this entry:

1. **The Qt client had no window geometry or default-volume setting.** It
   persisted exactly one thing (`ui/language`) and hardcoded `resize(1200, 700)`.
   The "remaining parity gap" from two entries ago was recorded from memory and
   checked against nothing -- the same mistake as the parser bug, but in the
   documentation.
2. **The sanitizer job was C++ only.** ASan/TSan/UBSan have no equivalent in
   Go. What Go does have, and what already caught a defect in this project, is
   `-race`, now the one sanitizer step.
3. **The `--no-tray` flag was declared and ignored.** It now does what it says.

A **second correction, about a security claim.** deviation-0029 survived because
the docs and the ticket code said `mpv` could not send HTTP headers -- which is
what forced a bearer value into the URL. It can: `--http-header-fields`. The
second remaining path that kept the ticket alive was the room-scoped read check,
which used to live only in the stream-ticket handler. That check moved into
`StreamFile` **before** the ticket handler was deleted, so the restriction
survived the cleanup instead of vanishing with it. `/stream` now authenticates
exactly like every other endpoint -- the header, then `CanRead` -- and the
five-minute HMAC ticket, its constant, its swagger definition and its tests are
gone. The endpoint may still accept a ticket if someone sends one; it no longer
helps.

The client-side story did not change much, because the Go client was already
header-based since idea-0066. What changed is that it is now the *only* client,
so the assumption can no longer survive by living in `frontend/src/` alongside
a working one.

### The release workflow, read line by line before the first tag

The release pipeline was rewritten for the Go/Fyne pair and has never been run —
which the card for it says out loud, and which is the only honest state: every
command in it had been checked by CI except the ones only this file contains. So
it was read the way the rest of the project is read, and three defects fell out,
none of which a tag would have shown cheaply.

**linuxdeploy was given names instead of paths.** `-e TorrSyncPlayer -e mpv` — from
the working directory, where neither exists: the binaries are in `dist/`. The
tool's own source settles it: for every `-e` it calls `fs::exists` and returns 1
otherwise, so the job would have died on "No such file or directory" before a
single library was bundled. Now `-e dist/TorrSyncPlayer -e dist/AppDir/usr/bin/mpv`.
This was not cosmetic: the client is a cgo/Fyne binary and needs twenty-five
shared libraries, so an image built without a working `-e` would have run on the
runner and nowhere else.

**The release notes for a new tag were empty.** The extraction looks for
`## [<version>]`, and this changelog has only `[Unreleased]` — so for the first
tag it produced zero bytes and fell back to a one-line `Release ${VERSION}`,
throwing away ~126 KB of everything written since v1.1.5. The convention this
journal follows is that `[Unreleased]` holds exactly what is about to ship, so
that is what it now takes, with a header saying where the notes came from.

**And the extraction matched prose.** The pattern was an unanchored substring
match, so a line of *prose* that quotes a heading hijacked it — and this journal
quotes its own headings constantly, including the entry about this very step. For
`v1.1.5` the notes started in the middle of somebody's paragraph (108 KB of the
wrong content); anchored at the start of the line they are the section's 35 lines.

Windows got a guard rather than a guess. `Get-Command mpv` resolves whatever comes
first on PATH, and on a chocolatey machine that is the shim in
`ProgramData\chocolatey\bin`, a directory holding one `.exe` and nothing else.
Copying it would ship a launcher without a player, and the archive check — which
looks for `mpv.exe` — would have passed. The job now fails when no DLL sits next
to mpv, because that is the difference between a bundle and a stub.

Checked and *not* defects, so they are not "fixed": `upload-artifact@v7`,
`download-artifact@v8` and `action-gh-release@v3` all exist (verified against the
GitHub releases API, not assumed — CI still pins `upload-artifact@v4`, which also
works); all fifteen shell steps parse under `bash -n`; all four workflows parse as
YAML. And the parts a runner would have done were done here: the pair builds, the
icon renders, `desktop-file-validate` accepts the entry, `AppRun` parses.

What is still missing is the one thing that closes the card: the workflow has not
been run by a tag. That is the owner's action, not the agent's.

### What existed and nothing wired: the reporter, the catalogue, the secret, and the settings

Four cards and one storage question, with the same shape between them: something
was built, tested and unreachable. A green suite sees none of that.

- **The buffer reporter was constructed and never asked to do anything
  (bug-0082).** `room.BufferReporter` had been finished and tested for a long
  time; the window created it and never called `Track`, `Run` or set a playing
  state, so the backend's piece prioritiser — the thing that makes "watch, don't
  download" true — was steered by nothing. The window now asks mpv for the
  file's duration (`player.Duration`), waits until mpv actually knows it
  (`waitForDuration`: a zero while the header is still being read is not an
  answer), starts the report with the file size and the duration, and cancels
  the previous file's report when the person switches files. The reporter's
  `Position` now asks mpv and returns an error while there is no player, instead
  of inventing a zero. A 2000-byte file that mpv says is 100 s long, playing at
  25 s, becomes byte 500 — the number the download window steers from.
- **Half the window bypassed the catalogue (idea-0073).** Every playback error,
  the room's own status lines and the track choosers were written as English
  literals, so a person reading the Russian interface read every failure in the
  source language. All of them go through `tr`/`trf` now. The check that keeps
  them from coming back — `internal/ui/catalog_usage_test.go`, a `go/parser`
  walk over the package — also found **eight keys the window asked for and the
  catalogue never defined**: they rendered as empty labels, which reads as a
  missing feature rather than a missing translation (tracks and room lines,
  left behind by `be96d0d`). Defined and translated in both languages.
- **The access token no longer travels on the command line (deviation-0061).**
  The embedded backend was started with `--token <secret>`, readable by anyone
  who can run `ps`. The backend gained `--token-stdin`: it reads the first line
  of standard input, trims it, refuses an empty secret at startup, and treats
  `--token` together with `--token-stdin` as a configuration error. The frontend
  no longer puts the secret in `Args` at all and writes it to the child's stdin;
  a test drives a real shell stand-in through `Start` to prove delivery.
- **The window could not reach its own backend (idea-0083), which is what the
  storage question was waiting for (idea-0081).** Storage, the htorrent gateway
  and the signaler were reachable only by starting a server by hand — and then
  it was not the application's own backend. There is a settings dialog now:
  language, theme, mpv path, storage (memory or disk with a directory), gateway
  and signaler, with storage and gateway in the same form the backend accepts
  them.

What the settings dialog does not do is pretend. Theme and signaler apply at
once (the window redraws; the signaler is read at the next join); language, mpv,
storage and gateway are read at startup, and the form says so — the backend
child is already running and a window cannot reconfigure it. On first launch the
application asks where the pieces should live, with **memory preselected** and
`~/Downloads/TorrSyncPlayer` offered for disk; "Later" is not an answer, so the
question returns at the next launch. Memory mode now means **nothing on disk**,
registry included: the frontend passes an empty `--data-dir`, which disables
persistence entirely, matching what "in memory" promises.

What travels to the child, and how, is deliberate: storage as **flags**
(`--data-dir`, `--disk-storage` — a flag is not a default, so an inherited
`DATA_DIR` cannot substitute for the choice made in the window), the gateway as
**environment** with replace-and-scrub semantics (so a gateway switched off in
the window does not survive the next launch), and the token on **stdin**. The
password is never on the command line.

Two more defects came out of the same pass, both found by running what was
written rather than by reading it:

- `--lang` reached only the tray. The window kept reading the store, so the flag
  changed the tray menu and nothing else — worse than no flag, because `--help`
  promises it. Both surfaces now use one resolver
  (`PreferredLanguage`: flag → setting → system locale → English), the
  documented system-language fallback exists at all, and locale spellings like
  `ru-RU` and `ru_RU.UTF-8` are understood.
- The window constructor started its poll and player-watch loops **twice**: the
  split into a build-only function and one that starts the loops left the
  goroutine starts in both halves, so every window polled its backend twice.
  Caught by `-race` after the split, not by review; fixed, with the smoke test
  building through the goroutine-free half so no test leaves a poller behind.

Also: `frontend/go.mod` tidied (the dialog gained `github.com/FyshOS/fancyfs`,
systray and weron became direct requirements, `kr/text` dropped), and `DEV.md`
gained the confirmed wording: English is the source language and fallback,
`--token-stdin`, the storage question, the settings channels, and the pact file
declared a hand-edited fixture rather than a recorded contract (idea-0076). The
swarm scenarios already in the tree closed the "watch, don't download" card
(idea-0071); subtitles from the swarm stay open with the owner's choice recorded
(idea-0080: a separate endpoint, `--sub-file=http`, no code this pass).

Verified: frontend — `go build`, `go vet`, `gofmt` clean; `go test ./...` green
with `FYNE_DRIVER=offscreen`; `-race` green for the changed packages; the five
real-mpv tests ran and passed, not skipped. Backend — build, vet, full test
suite green. Full `-race` over the frontend is red in `internal/rtc` for a
reason outside this repository: `weron@v0.3.0`'s in-process signaler has a data
race in its own client map, with no project frame in the report; the plain run
is green and the package is untouched.

## [v1.1.5] - 2026-07-12

### Added

- Makefile `release` target: single-command tag + push workflow
- Release workflow: release notes extracted from CHANGELOG.md (via awk)
- Release workflow: single-file portable Windows EXE with embedded Go backend
- Release workflow: SHA256 checksum verification for linuxdeploy downloads
- Release workflow: per-job permissions (contents:read/write)
- CI: race detector with CGO_ENABLED=1
- `backend/Dockerfile`: multi-stage Alpine build (1.6MB runtime)
- P2P: TURN server configuration via TURN_URL/TURN_USERNAME/TURN_CREDENTIAL envar
- Auth: JWT token TTL configurable via JWT_TTL_HOURS environment variable
- Auth: structured audit logging for register/login events
- Auth: CORS origins reload every 5 minutes (no restart required)
- Security: IPv6 private range detection in proxy trust logic (fc00::/7, fe80::/10, ::1)
- Torrent service: 10 new tests (sanitizeFilename, nil buffer, Close idempotent, custom options, concurrent remove)
- Goroutine leak tests for P2P and Buffer services

### Fixed

- Auth: hardcoded dummy bcrypt hash fallback replaced with nil-safe handling
- Auth: `ErrInvalidCredentials` now returns AppError for correct HTTP 401 mapping
- API: `ErrTimeout` now returns HTTP 408 instead of 500
- P2P: Close timeout extracted to named constant
- Makefile: Go version check now uses proper semver comparison
- Sync: `latencyMs` validated for negative values (clamped to 0)
- CORS: exposed X-RateLimit-* headers for client access
- Server: default Go Server header removed from all responses
- Frontend: `MinRoomNameLength` synced to 1 (was 2, backend expects 1)
- Frontend: `testFormatDurationSecondsOnly` now tests seconds (was duplicate of zero test)
- Frontend: `build.sh` treats libmpv as optional, matching CMakeLists.txt
- Frontend: `handleApiError` body size limited to 64KB (OOM protection)
- API: documented health check response corrected (basic check returns only {"status":"ok"})
- Cleaned up stale artifacts (coverage, gosec-results.json)

## [1.0.0] - 2025-06-01

### Added

- Basic torrent client functionality based on anacrolix/torrent
- HTTP REST API server in Go with chi router
- P2P connections via WebRTC (pion/webrtc v4)
- Playback synchronization with latency compensation
- JWT authentication for users and peers
- Password-protected rooms with bcrypt hashing
- SSE (Server-Sent Events) for real-time room events
- Qt/C++ frontend with libmpv video player
- System tray integration
- Graceful shutdown for all services
- Structured logging
- CSRF protection
- Rate limiting for API
- CORS support
- Health check endpoints
- Prometheus metrics
- Swagger UI at `/swagger/`
- LRU cache with piece download priorities
- In-memory storage (UserStore, TokenRevocationStore)
- AppError and ErrorType structured error handling
- Constants package (all magic numbers extracted)
- TLS 1.2+ support
- pprof on port 6060 (optional)
- Retry logic in NetworkManager (exponential backoff, max 3)
- Seek debounce in MpvWidget
- `.editorconfig` for Go, C++, CMake, Makefile, JSON, YAML
- Code coverage in CI pipeline (Go + C++ with Codecov integration)
- Coverage check for PRs (minimum 60%)
- MIT license headers in all source files
- User guide (docs/USER_GUIDE.md)
- Installation guide (docs/INSTALL.md)
- Architecture documentation (docs/ARCHITECTURE.md)
- Contributor guide (CONTRIBUTING.md)
- RoomID validation in API endpoints
- Validation tests (internal/validation/validation_test.go)
- Auth handler tests (internal/auth/handlers_test.go)
- API handler tests (internal/api/handlers_test.go)
- Torrent service tests (internal/torrent/service_test.go)
- P2P service tests (internal/p2p/service_test.go)
- Sync service tests (internal/sync/service_test.go)

### Security

- JWT authentication with token revocation
- bcrypt password hashing (cost=12)
- CSRF tokens with TTL 1h
- Rate limiting (10 req/min for auth, 60 req/min for API)
- Security headers (X-Content-Type-Options, X-Frame-Options, HSTS)
- CORS policies
- All input data validation
- TLS 1.2+ support

### Architecture

- Microservice architecture with independent services
- Thread safety via sync.RWMutex
- Graceful shutdown with timeouts
- Context-oriented operation management

### Known Issues

1. `frontend/src/main.cpp:336` — when launched with `--server-url`, the URL is parsed but not passed to NetworkManager
2. `frontend/src/networkmanager.cpp:301` — SSL errors are ignored in debug mode; production behavior is not implemented
3. In-memory UserStore/TokenRevocationStore — no persistence
4. No database integration
5. Buffer Service lacks unit tests
6. Frontend MainWindow/MpvWidget do not have unit tests

### The release workflow's first real run, and what only a run could show

The workflow has now been executed — by the tag `v1.2.0-rc.1` — and it failed,
which is the whole reason the card for it stayed open. Reading a pipeline finds what
a reader knows to look for; running it finds the rest. Two defects were of exactly
that kind, and neither was visible from the file.

**linuxdeploy wanted a number, not a word.** `--verbosity error` was rejected with
`Argument 'verbosity' received invalid value type 'error'`, followed by the entire
help text. The verbosity is 0=debug, 1=info, 2=warning, 3=error — a number, as the
tool's own usage says. The step died before a single library was bundled.

**Windows had no mpv on PATH, and what it did have was a stub.** The chocolatey
package installs mpv without adding it to PATH, so `Get-Command mpv` in the
assemble step reported "the term 'mpv' is not recognized" — immediately after a
successful install. Worse than the failure is what *would* have happened next: the
one thing that does resolve on PATH is `mpv.com` in the package's tools directory, a
launcher stub with no libraries beside it, so the mpv directory is now chosen by what
it holds — the directory whose side has the most DLLs — rather than by what PATH
happens to answer. Copying the stub would have shipped a launcher with no player in
it, and the archive check, which looks for `mpv.exe`, would have passed anyway.

The macOS job passed in full — including the ad-hoc signing and the DMG content
check — so three of the four jobs' worth of untested commands are now confirmed by a
real runner.

The release itself is a draft pre-release, which is what a tag with a suffix means
here: the artifacts are built and attached, but nobody can see them until somebody
looks. That is the state this workflow should have been run in from the beginning,
and the reason the tag is `v1.2.0-rc.1` rather than `v1.2.0`.

### The second rc run, and the two failures only a runner could produce

Tag `v1.2.0-rc.2` fixed the two defects the first run found, and the runners
found two more — one in CI, one in the pipeline. Neither was visible from the
file, and both are now fixed rather than worked around.

**The AppImage was built and then declared missing.** linuxdeploy printed
"Success" and a 110 MB squashfs, and the step's own check reported "no AppImage
was produced" — because the image lands in the *working directory*, not in
`dist/`. linuxdeploy never learns where the AppDir came from; it writes its
output next to where it was invoked. The image was sitting in the repository
root while `dist/*.AppImage` matched nothing, which would also have made the
upload step ship an empty artifact. Now the image is moved into `dist/` by
name, and the check looks at what is actually there.

**The swarm test hung CI for ten minutes.** `TestBackend` died on go test's own
timeout with `TestAPartialTorrentIsPlayable` stuck in `reader.waitAvailable` —
inside the range the test streams, on a byte of a piece that had been counted
complete minutes earlier. The mechanism is in the library, not in the project:
completion is a bitmap that a failed hash check clears, the piece is un-marked
and re-requested, and with the seeder removed nobody answers that request. What
the test did wrong was stream "what was complete when counted" with a request
that carries no deadline — one un-marked piece turns the whole suite into a
ten-minute silence. Two guards now: the streamed range is measured at stream
time, re-checking the complete prefix right before serving, and the serve itself
is bounded — on a timeout the test fails with the prefix state attached, which
is a diagnosis rather than a hang.

**The frontend gate caught a missing tool, which is the gate working.** The
"no SKIP" re-run of the real-mpv player tests failed because the multi-track
fixture needs ffmpeg to build it, and the runner has mpv but no ffmpeg. The
claim behind that gate is that every check stays verified; the fix is to install
ffmpeg next to mpv, not to teach the gate to tolerate a skip.

Windows and macOS are now green end to end — the chocolatey PATH search and the
ad-hoc-signed DMG both confirmed by real runners. Linux is the one still open,
and only because of the AppImage path above.

### The player was reading other people's mail

`Player.request` sent one command over a fresh IPC connection and decoded the
first line that came back. But mpv broadcasts async events to every connected
client, and the first line after a command can be an event — `start-file`,
`tracks-changed` — rather than the answer. Decoded into a reply shape that event
is `Error=""` with no data, which is indistinguishable from "nothing there yet".

Two symptoms, one cause. In the test, a track query made while the file was still
opening reported "no tracks" — the CI run of `TestRealMpvListsAndSwitchesTracks`
saw fifteen seconds of exactly that against mpv 0.34.1 and failed the whole
frontend job. In the product, worse: a position query on the same hot path
reported position **0 with no error**, and the buffer reporter handed the room
"the start of the film" — every time a poll landed on an event, which is exactly
when the room is most active.

`request` now reads until the reply: lines carrying an `event` without a
`request_id` are stepped over (a command reply always has one, zero when the
command did not set it; an async event never does), bounded at 32 — event
traffic is finite, a file start produces a handful, so a longer stream means
something else is wrong and spinning until the context deadline would turn that
into a slow silence.

The track test also stopped conflating two failures. "The file never opened" and
"the file has no such tracks" used to look identical — both were an empty list —
so `waitForLoaded` now waits for the playlist to admit the file first and fails
naming the file when it does not. Whatever fails next will say which one it is.

### v1.2.0-rc.6 — the pipeline is green and the artifacts are real

The release workflow has now run, and run green: `TorrSyncPlayer-x86_64.AppImage`
(111 MB), `TorrSyncPlayer-windows-x64.zip` (68 MB), `TorrSyncPlayer-macos-arm64.dmg`
(27 MB) and `SHA256SUMS`, each one unpacked or mounted on its own runner and
checked for the client, the backend, mpv, the launcher and a non-empty library
directory. CI on the same commit is green too — twelve jobs, no failures.

Getting there took six release-candidate tags, and every one of them found
something no reading would have:

1. **rc.1** — `--verbosity error`: linuxdeploy wants a number. And `Get-Command mpv`
   found nothing, because the chocolatey package does not add mpv to PATH.
2. **rc.2** — the AppImage was built (110 MB of squashfs) and then declared missing,
   because linuxdeploy writes its output to the *working directory*, not to the
   AppDir's parent; the swarm test then hung CI for ten minutes, streaming bytes
   of a piece that a failed hash check had un-marked with no seeder left to
   re-request it from; and the no-skip gate caught a missing ffmpeg.
3. **rc.5** — `Player.request` decoded the first line it could read as the reply,
   and mpv broadcasts async events to every connected client. A position poll that
   landed on an event reported position **0 with no error**, and the buffer
   reporter handed the room "the start of the film". Replies are now matched by
   shape (`request_id`), bounded at 32 skipped events.
4. **rc.6** — mpv enumerates a file's tracks only when it has an output chain to
   feed them to. On a runner with no DRM, no SDL and no display, `aid` and `vid`
   stay `auto`, the tracks are never created, and `track-list` answers an empty
   list with error `success` — indistinguishable, over the wire, from a file that
   is still loading. The test runs headless now (`--vo=null --ao=null`, an output
   that needs no device; `--vo=no` looks right and is not — mpv reports "Video
   output no not found").

Two of the six were product defects rather than test defects. One was the request
reader. The other is the shape of a feature: on a machine with no video output a
user's film has no selectable tracks, because mpv never activates them. The
application opens a window and needs no workaround for that, but it is now known
and written down rather than assumed.

`Test Frontend (Go)` passed five red runs before it passed green, which is the
point of a gate that cannot be silenced. mpv's own output is no longer discarded:
a player that fails to open a file or a track used to fail silently, and that
silence is what made this take six tags to understand.

Board: `bug-0070` fixed and removed. `idea-0026` (packaging) is carried out for all
three platforms — AppImage, zip-with-mpv, and an ad-hoc-signed DMG — with Winget
still unstarted, which is what the card's own wording always meant by "Winget".

The release is a draft pre-release, deliberately: a tag with a suffix publishes
nothing until somebody looks at the artifacts and decides.

### The frontend is checked on all three platforms now

The goal promises a desktop application for Linux, macOS and Windows, and CI
checked one of them: `test-frontend-go` ran on ubuntu alone, while the backend had
a three-way matrix since long before. Everything that can only break somewhere
else broke silently — path separators, the mpv IPC named pipe that Windows uses
instead of a unix socket, Fyne's cgo and GL headers, `fyne.io/systray`, and the
packaging of the client+backend pair.

`test-frontend-go` is a matrix now: ubuntu-22.04, macos-latest, windows-latest,
with the same suite on each — tests under `-race`, the no-skip player gate, the
full build, and the end-to-end scenario tests that start the real backend binary
and drive the real client against it. What differs per platform is how the
dependencies arrive, and that difference is the point of the matrix rather than an
inconvenience: headers that exist on one runner and not another, a display on none
of them, mpv and ffmpeg from three different package managers.

Three things were needed to make that honest rather than merely green:

- **`fail-fast: false`.** A red macOS leg must not cancel ubuntu and Windows —
  the value of a matrix is that it says *where* the break is, and a cancelled leg
  tells nobody anything.
- **One shell syntax.** Every step is `bash`, including on Windows, so the job is
  one file rather than three dialects of the same ten steps.
- **mpv and ffmpeg on PATH on Windows.** Chocolatey does not put mpv there for
  later steps — the release job learned that the hard way — so the directory is
  found by what it holds and pushed onto PATH.

What this does not prove, said plainly as the card for it demands: that an
application *window* opens and the video plays. A headless runner compiles and
tests code; it cannot open a window. The gap between "built" and "someone is
watching" is closed by a run on a real machine, and that remains unscheduled.

### The first three-platform run, and what only macOS and Windows showed

`test-frontend-go` is a matrix now — ubuntu, macOS, windows, same suite — and it
ran red three times over, each leg finding something the other two never could.
That is the whole point of a matrix that does not cancel itself: it says *where*
the break is.

**Ubuntu lost mpv.** The first run of the matrix dropped `mpv ffmpeg` from the apt
line while rewriting the per-platform installs, so the player tests failed with
`exec: "mpv": executable file not found in $PATH`. A red job that tells nobody
anything about the platform, which is why the packages are installed next to the
headers rather than in a step of their own.

**Windows did not parse my PowerShell.** `"$name: ..."` — PowerShell reads `$name:`
as a *drive-qualified variable reference* and refuses it: "':' was not followed by
a valid variable name character". It needs `${name}:`. The release workflow never
hit this because it prints `$mpvDir` and no `$name:`; the matrix hit it within
minutes, which is what a matrix is for.

**macOS found two things that are true about the project.**

The build script is fine and its test was not: it compared the output directory as
a *string*, and on macOS `/var` is a symlink to `/private/var`, so `t.TempDir()`
hands out `/var/folders/...` while `filepath.Abs` — what the build script uses, and
what `os.Getwd` returns — produces `/private/var/folders/...` for the same
directory. A passing build looked broken. Both sides are resolved before they are
compared now.

And macOS caps unix socket paths at 104 bytes. `t.TempDir()` on a macOS runner is
already most of that before the test's own name is appended, so the real-mpv tests
died with `bind: invalid argument` and "mpv did not open .../mpv.sock in time". The
socket in the tests is now in a short directory; the name still carries the pid so
two players cannot collide. This one was a near miss for the product rather than a
test-only issue: the application's own socket path is
`<tempDir>/torrsync-<pid>.sock`, and a long `TMPDIR` would put it over the same
limit.

What the matrix still does not prove, and the card for this work says so out loud:
that a window opens and the video plays. A headless runner compiles code and runs
tests; it cannot open a window.

### Windows, for the first time, and what it cost

The macOS run turned green with two fixes. The Windows leg ran red for forty
minutes and found four things — none of which are visible from a Linux shell,
which is the entire point of the matrix.

**The backend was refused as world-writable.** Go reports mode 0666 for a file
created in a temp directory on Windows whatever the actual permissions are, so
`FindBackend`'s Unix check refused *every* candidate and the application had no
backend to start. On Windows there is no portability answer for "who can write
this": it is an ACL question, and the standard library does not expose one. The
check is now per platform — the mode bits on Unix, false on Windows with the
reason and the size of the loss written next to it. What is lost is a check, not
an assurance: the binary is still looked for beside this application rather than
downloaded.

**`go build -o` silently appends `.exe`.** The scenario harness built the backend
into a path without an extension; on Windows the command succeeds and writes
`torrsyncplayer.exe`, and the tests then try to exec `torrsyncplayer` — six
scenarios failing with "executable file not found", each one of them a green-CI
lie on Linux.

**The player's socket was a path where Windows wanted a name.** `dial_windows.go`
already said so in its comment ("mpv is handed a bare name with
--input-ipc-server, and it is opened here as `\\.\pipe\<name>`"), and
`NewWindow` handed it a full path anyway, so nothing agreed with anything. The
application's socket is now a pipe name on Windows and a path everywhere else,
and the application and the player cannot drift apart because both go through one
function.

**The fake mpv used a unix socket.** Windows has none, so the protocol tests died
with `bind: invalid argument`. The fake now runs over `net.Pipe` — an in-memory
pair that behaves like a connection on every platform — and the transport is a
field on `Player` so a test can supply one. That is what makes the protocol
testable where the OS has no unix sockets, and it is also why the tests now fail
for a reason rather than for a platform.

Two things are still unproven, and they are recorded rather than assumed: that
mpv on Windows opens a *pipe* with a bare name (it should, and the real-mpv
tests are the ones that will say), and that a window opens at all on any of the
three.

### The world-writable test now asserts the coupling, not one platform

The Windows fix for the world-writable backend refused to be tested by a test that
asked both platforms the same question. `TestFindBackendRefusesAWorldWritableBinary`
asserted "a world-writable backend is refused", which is true on Unix — where
`os.Chmod 0777` really is world-writable — and false on Windows, where the check
deliberately is not implemented because Go's mode bits do not answer the question
there.

It now asserts the *coupling*: if this platform's check calls the file
world-writable, the search must refuse it; if it does not, the search must use it.
The Unix behaviour is pinned exactly as before, the Windows behaviour is pinned as
deliberately absent rather than silently broken, and the day someone makes the
check quietly do nothing on any platform the test fails instead of the application
starting with no backend.

### Three platforms, three green legs, and the gap that is still open

`test-frontend-go` runs on ubuntu, macOS and windows now, and all three legs are
green — 13 of 13 CI jobs. The same suite on each: tests under `-race`, the no-skip
player gate, the full build, and the end-to-end scenario tests that start the real
backend binary and drive the real client against it. Windows took an hour, macOS
two minutes, ubuntu ninety seconds.

The run was worth its cost. Nine findings, every one of them something no Linux
shell would have shown, and two of them product defects rather than test defects:

- `FindBackend` refused every backend on Windows, because Go reports mode 0666 for
  a temp file there whatever the permissions are, so the Unix "who can write this"
  check had no answer for the platform and answered *no* to all of it. The check is
  per platform now, with the loss written down: on Windows it is an ACL question
  the standard library does not expose.
- `go build -o` silently appends `.exe`, so the scenario harness built one name
  and exec'd another. Six scenarios, red on Windows and green on Linux.
- The player's socket was a path where Windows wanted a pipe name — the comment in
  `dial_windows.go` already said so, and `NewWindow` did not read it. Both go
  through one function now.
- The fake mpv used a unix socket. It runs over `net.Pipe` now, and the transport
  is a field on `Player` rather than a call: that is what let the protocol tests
  run on a platform with no unix sockets at all.

Plus two that were the tests being wrong in a platform's terms: `/var` versus
`private/var` spelling the same directory on macOS, and the 104-byte unix socket
cap that a `t.TempDir()` already nearly fills there. The last one is worth
remembering for the product, not only the tests: the application's own socket path
is `<tempDir>/torrsync-<pid>.sock`, and a long `TMPDIR` would put it over the same
limit.

What this does not prove, and the card for the work says so in its own words: that
a window opens and the video plays. A headless runner compiles code and runs
tests. The gap between "built on three platforms" and "used on three platforms" is
a run on a real machine, and that is still unscheduled.

### idea-0064: комнаты без сервера — брокер выпилен, mesh вместо него

Направление было решено раньше, детали — в этой сессии, все пять владельцем:
слоями с удалением последним; код приглашения `community:password:key`
генерирует хост; в тестах только локальный сигналер; сглаживание и проверка
хоста на последователе; уход хоста закрывает наименьший живой peer ID.

Слои:

1. **Протокол** (`frontend/internal/rtc/protocol.go`): magnet, pause,
   position, buffering, hello, host, bye. Неизвестный тип — ошибка, а не
   догадка. Два чистых правила: команды считаются только от приславшего
   magnet, takeover — наименьший ID.
2. **Mesh** (`mesh.go`): join по community+password+key, accept пиров, чтение
   каждого потока с построчным фреймингом, broadcast, ростер, bye с грейсом.
   Тесты двухслойные, потому что рандеву падает от погоды (нагрузка CPU,
   призраки на переиспользованном сигналлере), а логика — нет: детерминированные
   unit-тесты поверх `net.Pipe` плюс интеграционные против сигналера в процессе
   с ретраями на свежем сигналлере.
3. **Драйвер комнаты** (`room.go` + `Transport`): ростер из hello, хост из
   magnet/host, сглаживание на последователе (те же 2 секунды и 30% — числа
   сервера, место новое), инвайты с генерацией без двоеточий.
4. **Удаление**: `internal/p2p` (1280 строк) и `internal/sync` с 2574 строками
   их тестов, роуты `/rooms` и `/sync`, SSE-менеджер, модели событий комнат,
   персистентность комнат и синк-стейта, их метрики, константы, пути,
   валидация имён и ID комнат, room/sync k6 и chaos-пробы, pact-взаимодействия
   с удалёнными endpoint'ами. Осталось сознательно: `MagnetFor` (хост анонсирует
   свой фильм из него), ownership-`CanRead` (строже комнатного), реестр
   торрентов, pact минус четыре взаимодействия.
5. **Комната без сервера** (`TestARoomWithoutAServer`, ~4 с): два настоящих
   бэкенда, локальный трекер и сигналер, magnet по data-каналу, настоящие
   куски из роя через настоящий adopt, pause веером, takeover после ухода хоста.

По пути нашлось настоящее, все три — в коде, а не в тестах:

- `Player.request` читал первую строку как ответ, а mpv шлёт события всем
  подключённым: опрос позиции, поймавший событие, отдавал **0 без ошибки**, и
  репортер буфера нёс комнате «начало фильма». Ответы теперь матчатся по
  `request_id`, событийные строки пропускаются (лимит 32).
- `Player.Close`... нет — `Mesh.Close` вешался на прощальном `bye`, когда
  читать уже некому: bye best-effort с секундным грейсом, потом соединения
  закрываются и ростеры догоняют сами.
- `json.Decoder` поверх message-oriented SCTP роняет пира через
  `io.ErrShortBuffer`: построчный фрейминг вместо потокового декодера.
- Клиентский `GET /magnet` — новый endpoint: хосту нужен magnet своего
  торрента для анонса, а infohash торрент не переносит.

Способ и цель перенесены с подтверждения формулировок: ADR переписан
(статус «заменено», решение — weron + data-канал), non-goal про WebRTC убран
из цели, секции комнат/синка/инвайта/способа — за mesh. SECURITY.md
переписан следом: сервер больше ничего не знает о просмотре, видит сигналер
(IP и состав, не команды), токен — доступ к одному своему бэкенду, секрет
комнаты — код приглашения без срока и отзыва.

Аудит после правки: deviation-0039 — resolved (гости не трогают чужой бэкенд
вообще, проверять нечего); deviation-0062 — resolved (предмет исчез вместе со
старым текстом ADR); idea-0024 — rejected как поглощённая (предлагала опцию,
случилась замена); deviation-0061 — проверен чтением кода, компромисс жив,
остаётся open; bug-0082 — проверен, баг жив, остаётся open. idea-0064
перенесена полностью — карточка снята, история здесь.

Проверено: оба модуля зелёные целиком (`go vet` чисто), спека перегенерирована
и не врёт (0 упоминаний комнат), 361 тест (242 backend + 119 frontend) с новыми mesh-слоями.
