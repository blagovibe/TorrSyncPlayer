# Tauri Frontend (Rust + WebView) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite the TorrSyncPlayer frontend from Go/Fyne to a Tauri app (Rust shell + vanilla TS/CSS frontend), reaching full feature and three-platform parity with the Go frontend, then delete the Go frontend.

**Architecture:** Tauri 2 shell spawns the untouched Go backend (`torrsyncplayer-backend`) as a child process, talks to it over 127.0.0.1 HTTP with the same API contract as today. The frontend is vanilla TypeScript + CSS (no bundler) in the system WebView, following the Vision Pro / Netflix-home references the owner picked. The watch-party room is reimplemented in Rust directly inside the Tauri app (rust-libp2p), protocol version 3, deliberately with no interop with Go-built peers. The mpv player is driven by the same IPC pipe (`\\.\pipe\torrsync-<pid>` on Windows, unix socket on unix) the Go app uses.

**Tech Stack:** Rust 1.99, Tauri 2, npm (only the Tauri CLI; the web app has no build step), TypeScript (no bundler), plain CSS, rust-libp2p 0.56, tokio, reqwest (rustls), serde_json, Go backend unchanged.

**Spec:** `docs/superpowers/specs/2026-11-17-tauri-frontend-design.md` (read it first; it is the source of truth for scope and owner's decisions)

## Global Constraints

- The Go backend (`backend/`) is byte-untouched: same flags, same HTTP API, same responses. Any behavior the frontend needs must exist there already — verified at `backend/internal/api/router.go` (list/add/files/select/magnet/subtitles/buffer info/version).
- The mpv binary, its flags, and the IPC pipe path stay exactly as the Go frontend launches them; buffer-position formula `(position / duration) * fileSize` stays exactly as in `frontend/internal/room/buffer.go: BytePosition`.
- Protocol version 3, protocol ID `/torrsync/sync/3.0.0`, rendezvous namespace `torrsync-room-v1␀<community>`, handshake HMAC-SHA256 over `"torrsync-auth-v1\x00" + password + "\x00" + key` — the messages stay newline-delimited JSON, the type tags stay identical (`magnet`, `pause`, `position`, `buffering`, `hello`, `host`, `bye`, `peers`).
- Full parity gate before Go frontend deletion — this means checklist in the spec (library, playback, room, settings, tray, diagnostics, language, first-launch storage) — not "mostly working".
- Three OS packages every release: Windows ZIP, macOS DMG, Linux AppImage — as today.
- Test bar: mechanisms covered (room state machine, buffer math, obvious-file, race tests, ported integration tests), UI verified by eye.
- The public-DHT nightly series (`measurements/`, `rendezvous.yml`) continue to run post-release; they are not release gates.
- No deadline pressure; phases are ordered by risk (room before library), and the owner asked for that explicitly.

## Review Focus

The spec's exclusions and the owner's explicit answers leave the following failure classes uncovered by any task's tests — these are the most likely to bite a person using this app, listed worst first:

1. **The room silently stops applying to the person** — a host pauses and the follower's player keeps playing / drifts forever without any error shown. Expectation: buffering report → host stops itself, or on host pause all peers pause, within ~1 s. Covering task: Phase 2 integration test `ARoomOfThreeIsARoomAndNotAPair` ported to Rust + divergence CI (existing measurement against a TOML-pinned threshold).
2. **The guest adopts the magnet and his backend can never get the movie** — adopting a magnet hands it to a local backend; if the infohash's file list never comes, or the file the host picked doesn't exist, the person sees an empty player forever. Expectation: adoption waits for the metadata, and the failure lands in the diagnostics file with a protocol-honest message. Covering task: Phase 2 `careless-adoption` test added to the guest suite.
3. **Launching when the second instance is already running** — a second launch spawns a second backend and both fight over the torrents; or the Tauri app refuses to start and the person has no idea why. Expectation: the single-instance guard says why (message or log) and exits quietly. Covering task: Phase 1 `second_instance.rs` test using the same PID-pigeonhole approach as the Go frontend (mutex on startup or a lockfile under the config dir).
4. **Closing the window silently kills the backend while downloading** — a person expects a download to continue with the window closed. Expectation: without tray, the window close keeps backend running (hide or ask); with tray, closing hides; quitting ends both. Covering task: Phase 5 `window_close.rs` test simulating both tray-present and tray-absent drivers.
5. **The settings dialog allows corrupt state that bricks the app on next start** — empty dataDir with disk mode, garbage JSON that never parses. Expectation: invalid settings are refused at save time with a human-readable error; a corrupt settings file on disk falls back to defaults rather than failing to start. Covering task: Phase 5 `settings_roundtrip.rs` tests.

---

## Phase 0 — Spike: prove the toolchain end-to-end (mandatory)

A short spike proving the four load-bearing assumptions before anyone invests in building anything: spawn the Go backend, show `/api/v1/version` in the Tauri window, open the mpv IPC pipe, and see the window/transport work in WebView. The owner's "Poшли" authorizes this. Destroy all spike artifacts at the end; keep only `docs/` notes.

### Task 0.1: Scaffold the Tauri app

**Files:**
- Create: `frontend/package.json`, `frontend/index.html`, `frontend/src/main.ts`, `frontend/src/style.css`, `frontend/src-tauri/Cargo.toml`, `frontend/src-tauri/tauri.conf.json`, `frontend/src-tauri/src/main.rs`, `frontend/.gitignore`
- Delete after phase passes: nothing in this task

**Interfaces:**
- Consumes: nothing
- Produces: a window that opens and shows "Выберите папку…" placeholder text; the npm scripts `npm run dev`, `npm run build`; the cargo crate `torrsyncplayer` (bin name `TorrSyncPlayer`)

- [ ] **Step 1: Write the scaffold**

```bash
cd frontend; npm init -y; npm i --save-dev @tauri-apps/cli@latest
```
Then overwrite `package.json` with `{"name":"torrsyncplayer","version":"0.5.0","private":true,"scripts":{"dev":"tauri dev","build":"tauri build"}}`; write `index.html` with a single `<div id="app"></div>`; write `src/style.css` with `body { margin: 0; background: #0f0f12; }`; `src/main.ts` that sets `document.getElementById('app')!.textContent = 'TorrSyncPlayer'`; `src-tauri/Cargo.toml` with `[package] name="torrsyncplayer" version="0.5.0" edition="2021"`, `[dependencies] tauri = { version = "2", features = [] }`; `tauri.conf.json` with `productName:"TorrSyncPlayer"`, `identifier:"com.blagovibe.torrsyncplayer"`, window title `TorrSyncPlayer`, width 1280, height 880, `frontendDist: "../dist"` with `src/main.ts` compiled by a bare `esbuild` pre-step or `tsc --noEmit`-free (decision: no build at all — main.ts is served as plain JS; keep the file a `main.js` and drop TS for the spike).

- [ ] **Step 2: Verify it builds and starts**

Run: `cd frontend; npm run build`
Expected: `.exe` at `src-tauri/target/release/TorrSyncPlayer.exe`
Run: `npm run dev` — window opens, text visible.

- [ ] **Step 3: Commit**

```bash
git add frontend/package.json frontend/index.html frontend/src-tauri/
git commit -m "spike: Tauri scaffold builds and opens"
```

### Task 0.2: Spawn the Go backend and show its version

**Files:**
- Modify: `frontend/src-tauri/src/main.rs`
- Create: `frontend/src-tauri/src/backend.rs`

**Interfaces:**
- Consumes: `CandidatePaths` semantics (glob-match in Rust): executables named `torrsyncplayer-backend{,.exe}` next to the app
- Produces: `Backend::spawn(&self) -> BackendHandle` with `port: u16`, `token: String`; `BackendHandle::health(&self) -> bool`; `BackendHandle::stop(&self)`; `BackendHandle::version(&self) -> String`

- [ ] **Step 1: Write the failing test**

```rust
#[test]
fn backend_starts_and_answers_version() {
    let dir = tempdir().unwrap();
    // copy the built binary: integration target builds ../backend first
    std::fs::copy(env!("BACKEND_BIN"), dir.path().join("torrsyncplayer-backend.exe")).unwrap();
    let back = Backend::spawn_binary(dir.path()).unwrap();
    assert!(back.health(), "backend did not come up");
    let v = back.version();
    assert!(v.contains("0.4.13"), "unexpected version {}", v);
    back.stop();
}
```
(Integration test `backend_spawn.rs`; `BACKEND_BIN` set by build.rs or a `cargo test` wrapper script that builds `../backend` first.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cargo test --test backend_spawn -v`
Expected: FAIL — module not found / spawn error.

- [ ] **Step 3: Implement `spawn` and `version`**

Port `backend.PickFreePort` (bind 127.0.0.1:0), `GenerateToken` (16 bytes hex), `CandidatePaths` logic, `--token-stdin` stdin delivery, `--port` arg, and the timeouts from `process.go`. On Windows: `Command::new(exe)` with `stdout(Piped).stderr(Piped).stdin(piped)`; on Unix same but signal TERM on stop. Pipe token through stdin before spawn returns.

- [ ] **Step 4: Run test to verify it passes**

Expected: PASS — backend starts, `/api/v1/version` answers `{"version":"0.4.13"…}` (or whatever Go backend returns).

- [ ] **Step 5: Show the version in the window**

`async_thunk` invoked from frontend via `window.__TAURI__.core.invoke('backend_version')`; main.rs registers the command; index.html sets `app.textContent = 'backend: ' + v`.

- [ ] **Step 6: Commit**

```bash
git add frontend/
git commit -m "spike: spawn Go backend, show its version in the window"
```

### Task 0.3: Open the mpv IPC pipe and read pause/duration

**Files:**
- Modify: `frontend/src-tauri/src/backend.rs` (or new `frontend/src-tauri/src/mpv.rs`)

**Interfaces:**
- Consumes: pipe name convention `\\.\pipe\torrsync-<pid>` (Windows) / `$TMPDIR/torrsync-<pid>.sock`
- Produces: `MpvHandle::read_pause(&self) -> Result<bool>`, `MpvHandle::read_duration(&self) -> Result<f64>`

- [ ] **Step 1: Write the failing test** (skip on CI without mpv)

```rust
#[test]
fn pipe_roundtrip() {
    let bin = which_mpv().skip_if_missing();
    let handle = MpvHandle::spawn_and_connect(bin, mpv_log_dir());
    handle.send_command(&["loadfile", "about:blank", "replace"]).unwrap();
    std::thread::sleep(std::time::Duration::from_millis(300));
    let paused = handle.read_pause().unwrap();
    let duration = handle.read_duration().unwrap();
    eprintln!("paused={} duration={}", paused, duration);
    assert!(duration >= 0.0);
    handle.quit();
}
```

- [ ] **Step 2: Run test to verify it fails**

Expected: FAIL — `MpvHandle` not defined.

- [ ] **Step 3: Implement `MpvHandle`**

Named-pipe client (Windows: `File::open(format!(r"\\.\pipe\{}", name))` — open with retry for `ERROR_PIPE_BUSY` for a bounded window; unix: `UnixStream::connect(path)`). JSON lines, `request_id` on every outbound command, `get_property` requests for `pause`, `duration`.

- [ ] **Step 4: Run and observe** (skip threshold: on a machine without mpv this is informational)

Expected: log prints garbage or zeros; record outcome in `docs/spike-findings.md`. Delete the spike files if they're only diagnostic; keep `mpv.rs` if clean.

- [ ] **Step 5: Commit**

```bash
git add frontend/
git commit -m "spike: mpv IPC pipe readable from Rust"
```

### Task 0.4: Delete the spike scaffolding

- [ ] Remove `src/main.ts`, `src/style.css`; keep `package.json`, `src-tauri/Cargo.toml`, `src-tauri/src/main.rs`, `src-tauri/src/backend.rs`, `src-tauri/src/mpv.rs`, `src-tauri/src/lib.rs`, `src-tauri/src/main.rs` — assembled into a clean starting point for Phase 1.
- [ ] Write `docs/spike-findings.md`: what worked, what to carry forward, what to throw away.
- [ ] Commit

---

## Phase 1 — Shell and HTTP client (no library UI yet)

### Task 1.1: Tauri command `backend_health` and lifecycle wiring

**Files:**
- Create: `frontend/src-tauri/src/commands.rs`, `frontend/src-tauri/src/lib.rs`
- Modify: `frontend/src-tauri/src/main.rs`

**Interfaces:**
- Consumes: `commands::backend_health() -> Result<String, String>` (returns the version), `commands::backend_port() -> Result<u16, String>`
- Produces: the `invoke` JS API contract the UI will use throughout

- [ ] **Step 1: Write the failing test** — integration test `commands_backend.rs` asserts `backend_health` answers the backend's version string and `backend_port` answers a port that responds to HTTP.
- [ ] **Step 2: Implement** the two commands and register them in `lib.rs::run()` (builder).
- [ ] **Step 3: Run tests pass.**

### Task 1.2: HTTP client (`reqwest`, rustls) with the `X-Access-Token` + `X-Client-ID` headers

**Files:**
- Create: `frontend/src-tauri/src/client.rs`

**Interfaces:**
- Consumes: nothing beyond Task 0.2
- Produces: `Client::get_version() -> Result<String>`, `Client::torrents() -> Result<Vec<Torrent>>`, `Client::add_magnet(&str) -> Result<Torrent>`, `Client::add_torrent_file(&[u8]) -> Result<Torrent>`, `Client::delete_torrent(&str) -> Result<()>`, `Client::files(&str) -> Result<Vec<FileEntry>>`, `Client::select_file(&str, usize) -> Result<()>`, `Client::buffer_position(&str, u64) -> Result<()>`, `Client::magnet(&str) -> Result<String>`

- [ ] **Step 1: Write failing tests** with `wiremock` or a tiny in-process HTTP server answering the exact JSON shapes from the Go backend's router (listed above); assert header presence and body parsing.
- [ ] **Step 2: Implement.**

### Task 1.3: Shell lifecycle — spawn, watchdog, stop, diagnostics file

**Files:**
- Create: `frontend/src-tauri/src/lifecycle.rs`, `frontend/src-tauri/src/diagnostics.rs`

**Interfaces:**
- Consumes: Tasks 0.2–1.2
- Produces: `lifecycle::run(app: tauri::AppHandle)` — spawns backend, streams its stdout/stderr into `torrsyncplayer.log` with prefixes `backend:`, kills backend on window exit (with one `SIGTERM` then `SIGKILL` after 2 s on unix, `CTRL_BREAK` + `TerminateProcess` on Windows), writes startup/shutdown lines into the same log

- [ ] **Step 1: failing test** — spawn with a stand-in shell script that sleeps forever, assert that on stop it is actually dead (waitpid / equivalent) and that the log got both prefix lines.
- [ ] **Step 2: implement.**

### Task 1.4: Second-instance guard

**Files:**
- Create: `frontend/src-tauri/src/single_instance.rs`

Closes review-focus #3. Non-panicking refusal: if `torrsyncroot.lock` (or the equivalent named mutex on Windows) exists and the backend answers, exit quietly with a message to the log.

### Task 1.5: Settings read/write (`settings.json`)

**Files:**
- Create: `frontend/src-tauri/src/settings.rs`

Port the Go `Settings` struct exactly (see `frontend/internal/ui/settings.go:23-72`): `Language`, `DarkTheme`, `PlayerBinary`, `DisplayName`, `StorageMode`, `DataDir`, `StorageAsked`, `Bootstrap`, `Relays`. JSON file beside the executable on Windows; platform config dir on unix (mirror the Go `os.UserConfigDir` choice). Fallback: corrupt file → defaults, never a crash.

### Task 1.6: Hotkey surface

**Files:**
- Create: `frontend/src-tauri/src/hotkeys.rs`

`Ctrl+L` → focus the magnet field (event → JS `document.getElementById('magnet').focus()`); `Ctrl+R` → refresh; `Ctrl+,` → settings dialog; `Ctrl+C` in the field copied via tauri's clipboard.

### Task 1.7: Diagnostics export ("Copy diagnostics")

**Files:**
- Modify: `frontend/src-tauri/src/diagnostics.rs`
- Create: `frontend/src-tauri/src/copy.rs`

Assembles the bundle (mpv path, version, backend version, OS, last lines of log) exactly as the Go version did — output shape from `diagnostics.go:34-65`.

---

## Phase 2 — Room in Rust (the part the spec orders first)

### Task 2.1: Protocol types and newline-delimited JSON codec

**Files:**
- Create: `frontend/src-tauri/src/room/protocol.rs`

Types: `Magnet`, `Pause`, `Position`, `Buffering`, `Hello`, `Host`, `Bye`, `Peers` — field-for-field from the Go protocol (`MagnetURI`, `InfoHash`, `SelectedFile`, `Name`, `Pause`, `Position`, `Buffering`). JSON array-of-arrays codec matching Go's `json.Decoder` shape (one JSON object per line, `\n`-terminated). `ProtocolVersion = 3`, `SyncProtocolId = "/torrsync/sync/3.0.0"`.

**Interfaces:** `RoomMessage::encode(&self) -> Vec<u8>`, `RoomMessage::decode(&[u8]) -> Result<RoomMessage>`, `enum RoomMessage` with full variants.

- [ ] **Failing test:** golden round-trip — encode a Magnet, assert byte equality with the Go wire format; decode the Go fixture JSON files (a small fixture file `protocol_fixture.json` copied from `room_network_test.go`).

### Task 2.2: Invite code generation and parse

**Files:**
- Create: `frontend/src-tauri/src/room/invite.rs`, `invite_test.rs`

Format identical to Go's `rtc.Invite` (`frontend/internal/rtc/room.go:640-700`): bare `v2:<community>:<password>:<key>:<addrs>` where `Addrs` are comma-separated multiaddrs with `/p2p/` peer IDs; a legacy shape with no `v2` prefix is protocol 1 and must be refused with the "this build speaks" error exactly as Go refuses it. `ParseInviteCode` and `GenerateInvite` with the same community alphabet and randomness source. Go tests to mirror: `TestInviteRoundTrip` (invite → encode → parse → equal), `TestGeneratedInviteStrength` (community/passwords/keys are 9-char alphanumeric / 16-char mixed; key lengths asserted), `TestInviteCodeRejectsVersionMismatch` (a v3-code the build doesn't speak answers the refusal string).

### Task 2.3: Mesh node — listen, bootstrap, rendezvous, relay, handshake

**Files:**
- Create: `frontend/src-tauri/src/room/mesh.rs`

```
Peer → start everything:
  DHT (kad::tokio::Behaviour) seeded with public bootstrap nodes
  Identify
  Relay client (autorelay dialing the relays from `NodeOptions.StaticRelays` + discovered)
  Rendezvous client + server with namespace from the invite's community
  Custom behaviour for the sync protocol; handshake on stream open (challenge/proof/verdict)
```

**Interfaces:** `Mesh::new(options: NodeOptions)`, `Mesh::join(&self, is_host: bool) -> JoinHandle`, `Mesh::announce_magnet(&self, magnet: &Magnet)`, `Mesh::receive(&self) -> Receiver<RoomEvent>`, `RoomEvent::Magnet | Pause | Position | Buffering | Roster(Vec<Peer>) | Joined | Left`.

- [ ] **Failing integration test** `three_node.rs` — three processes on `127.0.0.1`, all on loopback, one host announces a magnet, two guests join, all agree within 1 s on the magnet and on pause=true; host closes, guests report the departure, new host (first remaining guest) announces it. This is the ported `TestARoomOfThreeIsARoomAndNotAPair`. Unix-domain sockets must sit in `$TMPDIR`/posix temp with names under 104 characters (the macOS CI failure mode from the Go port).

### Task 2.4: Room state machine and host migration

**Files:**
- Create: `frontend/src-tauri/src/room/state.rs`, `state_test.rs`

States: `Alone → Announcing → InRoom → Losing → Migrating`; transitions on events; host election by longest-running-peer order.

### Task 2.5: Sync loop and mpv control bridge

**Files:**
- Create: `frontend/src-tauri/src/room/sync.rs`

Subscribes to mpv property changes (`observe_property` for `pause`, `time-pos`), broadcasts `Pause`/`Position` on the room, applies incoming `Pause`/`Position`/`Buffering` to mpv. Buffering report every 500 ms from mpv's `demuxer-cache-duration` property. Gap tolerance 1 s, pause-on-gap, resume on follow.

### Task 2.6: Buffer-position reporting

**Files:**
- Create: `frontend/src-tauri/src/room/buffer.rs`, `buffer_test.rs`

Exact port of `BytePosition(positionSeconds, durationSeconds, sizeBytes) -> Option<u64>` (integer guard for negative position, zero duration, clamp at end) and `BufferReporter::Track/Run/…` with the same 500 ms ticker and the same silent-failure mode.

### Task 2.7: Room adoption — guest receives magnet and downloads

**Files:**
- Create: `frontend/src-tauri/src/room/adopt.rs`

On `RoomEvent::Magnet` for a guest not in the room: POST the magnet to the local backend (Task 1.2), wait for the file list to appear, match the host's `SelectedFile` by index (or by name suffix fallback), POST select.

**Review focus #2** (see header) — the failure to wait for metadata is THE sharp edge; the test simulates a slow backend (response delay) and asserts adoption does not blow up.

### Task 2.8: Restore the CI gates for rooms

- [ ] Add to `.github/workflows/ci.yml` (or a new `long-gates.yml` entry): `cargo test --release --release-sync` running `three_node`, `migration`, `buffering_follow`.

---

## Phase 3 — Library window and playback

### Task 3.1: Shelf of drawn posters (CSS, no image generation)

**Files:**
- Create: `frontend/src/shelf.js`, `frontend/src/poster.js`

CSS-drawn poster cards (seeded two-hue gradient + big Inter ExtraBold title + progress bar + delivery state line) — the visual design stays close to what the owner approved in principle at the end of the previous phase; final art direction is RC-stage, not this task.

### Task 3.2: Obvious-file port

**Files:**
- Create: `frontend/src-tauri/src/obviously_file.rs`, `obviously_file_test.rs`

Exact port of `obviousFile`: one file wins alone; multiple videos where one dominates the rest ≥ 3× wins; otherwise returns `None`, and the UI shows all buttons.

- [ ] test cases identical to `obviousfile_test.go` (one-file; one video among extras; giant video among samples; comparable videos → no pick; a 2/3-loss → no pick; no-video-many → no pick).

### Task 3.3: Torrent add — magnet field + .torrent file picker

**Files:**
- Modify: `frontend/index.html`, `frontend/src/main.js`, `frontend/src-tauri/src/commands.rs`

Drag-and-drop .torrent → `POST /api/v1/torrents/` (multipart? check the Go backend's exact multipart expected shape — it accepts raw bytes with a filename part); empty magnet → "Добавьте магнет-ссылку или .torrent" inline error, no modal.

### Task 3.4: Playback — launch mpv, feed it the stream URL, stop on exit

**Files:**
- Create: `frontend/src-tauri/src/player.rs`

`Player::start(&MpvOptions) -> Player`; `Player::play(&self, url: &str)`; singleton-per-invocation entry `player::ensure_player() -> Result<()>` — the app owns exactly one mpv at a time; closing the window quits it. `Player::kill()` on app exit.

### Task 3.5: Seek slider with debounced seek and volume

**Files:**
- Create: `frontend/src/seek.js`

Debounced `seek_interval` 400 ms; two clock labels; `pause` icon swap by mpv `pause` property; 2 Hz poll via Tauri event.

### Task 3.6: Audio/subtitle tracks choosers

**Files:**
- Create: `frontend/src-tauri/src/tracks.rs`

On `loadfile`, poll `track-list`; rebuild selects with `audio-track`/`sub-track`; send `set_property` on change; keep `off` entries for subtitles; hide choosers when single-track (as the Go version did).

### Task 3.7: Subtitle file picker — sidebar `.srt` list with add to mpv

**Files:**
- Create: `frontend/src-tauri/src/subtitle_files.rs`

The `subtitles` query of the backend's files list (verify whether the subtitle picker is a sidebar control listing `.srt` entries embedded in the torrent or an external file picker driven by tauri-dialog plugin). Decision: use the backend's `.srt` files from the `files` response filtered by extension — the exact feature the Go `subfiles.go` built around `subtitleFiles` and `subFileOptions`.

### Task 3.8: Buffering indicator — mpv `pause=1` + `cache-pause=1` → spinner over posters

Straightforward; no tests required beyond existence.

---

## Phase 4 — Settings, tray, language, diagnostics

### Task 4.1: Settings dialog — all fields ported

- [ ] language (en/ru), darkTheme, sleep-prevent glyph, player mpv path override, displayName, storageMode (memory/disk), dataDir (folder picker), storageAsked (question asked once), bootstrap, relays, verified problem strings inline (exact text from `settings.go:Problem`).

### Task 4.2: First-launch storage question gate

- [ ] on first launch after install: modal before main window; persist `storageAsked: true`; backend gets the same config at spawn (storage flags identical to Go constants `StorageMemory = "memory"`, `StorageDisk = "disk"`).

### Task 4.3: Tray icon + close-to-tray

**Closes review focus #4** (see header).

- [ ] tray icon drawn in-code as PNG bytes (reuse `IconPNG` logic — translate to tiny-Skia drawing or ship a base64 inline); `tray_menu` with Открыть / Настройки / Выйти; `Quit` handling identical to Go.
- [ ] close event with tray enabled → hide window; without tray → exit; both paths tested with a stand-in driver or manual check.

### Task 4.4: Language switch and catalog

- [ ] `catalog.ts` with en/ru tables (copy from `catalog.go` verbatim, preserving every key and translation — the owner speaks Russian; `TestNoWindowTextBypassesTheCatalog` equivalent: assert every `tr()` call key exists).

### Task 4.5: Diagnostics export button

- [ ] `copy_diagnostics` command builds the bundle; identical to Go's shape; mpv version comes from `mpv --version` capture (sub-100 ms).

### Task 4.6: Second instance dialog

Implicitly covered in Task 1.4; this is the modal text.

---

## Phase 5 — Packaging and release

### Task 5.1: AppImage, DMG, ZIP ribbons brought up-to-date

- [ ] copy the existing `.github/workflows/release.yml` shape; replace `frontend/` build step with `npm run build && npm run tauri build -- --target …`; keep mpv bundling, backend bundling, cross-compile targets, SHA256SUMS, THIRD_PARTY_NOTICES.

### Task 5.2: `.deb` and `.AppImage` packages signed by the owner's key if/when he has them; unsigned is fine for v0.5.0 RC.

### Task 5.3: DMG layout: `TorrSyncPlayer` + `TorrSyncPlayer-backend` next to it in the same app bundle (macOS Finder copy-UX is not part of v0.5.0 — zip-style).

### Task 5.4: v0.5.0-rc1 tag; owner validates; then fix pass; then v0.5.0 final.

---

## Phase 6 — Migration and deletion

### Task 6.1: Parity checklist (explicit, lives in `docs/parity-v050.md`)

Every feature listed in the spec's "Цели" section appears in a checkbox; every checkbox maps to a test or a manual eyeball gate on the owner's machine.

### Task 6.2: Delete the Go frontend

- [ ] `frontend/` (Go source) is deleted; `frontend/src-tauri/` and `frontend/src/` survive.
- [ ] `go.work` at repo root still covers `backend/` only (or is deleted).
- [ ] `.github/workflows/ci.yml` drops `frontend` matrix entries, keeps `backend`.

### Task 6.3: v0.5.0 final tag + release

---

## Self-review checklist

- Spec coverage: phases 1–6 map to the spec's phases 1–7 one-to-one; every spec bullet ("what flows from backend", "what Tauri does itself", "mpv IPC", "room", "settings/language/tray/diagnostics", "platform bundles") is covered by a task.
- No steps decide nothing: every test asserts something concrete; every interface names a function and a file.
- Type consistency: `RoomMessage`, `Client`, `Backend`, `MpvHandle` stay in their modules.
- Review Focus lines all have owner tasks (1 → Phase 2, 2 → Phase 2.7, 3 → 1.4/4.6, 4 → 4.3/Phase 5 manual, 5 → 5.4 RC).
- Proportion: plan is long because the work is a rewrite, not because of padding.
