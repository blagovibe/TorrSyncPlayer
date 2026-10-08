# Changelog

All significant changes to the project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

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
