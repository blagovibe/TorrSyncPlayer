# Changelog

All significant changes to the project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
