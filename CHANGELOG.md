# Changelog

All significant changes to the project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Persistence: JSON-file storage for users, revoked tokens, rooms and playback state (`internal/persistence`), enabled via `--data-dir`
- Persistence: debounced writes (`internal/utils/debouncer.go`) so bursts of state mutations collapse into one disk write; `Stop` waits for an in-flight write before the shutdown flush
- Contract testing: Pact provider verification against the real backend router (`internal/contract`, contract `pacts/frontend-backend.json`)
- Tests: chaos scenarios (toxiproxy), k6 load scenarios, Playwright e2e suite, Qt headless e2e
- Docs: `docs/METRICS.md`, `MUTATION_TESTING.md`
- Methodology: `CONCEPT.md` (goal and boundaries); `docs/ARCHITECTURE.md` serves as the way (how)

### Fixed

- Backend build was broken: `internal/sync` and `internal/p2p` referenced `utils.Debouncer`, but the file was ignored by `.gitignore` and never committed — restored and committed
- `internal/persistence`: tests did not compile (missing `time` import, non-existent `models.SyncStatus.RoomID` field)
- Removed dead code found by the `unused` linter: `realPiece` in `internal/buffer/service.go` (a wrapper around
  `*torrent.Piece` that was never used — `realTorrent.Piece()` returns the concrete type, which already
  satisfies `torrentPiece`)
- CI: Go pinned to 1.26.6 in ci.yml, release.yml and security.yml — `govulncheck` reported 7 stdlib
  vulnerabilities present in 1.26.5 (GO-2026-5026/6088/6089/6090/6091/6218), all fixed in 1.26.6
- Import grouping: `internal/buffer/service_test.go` failed the `goimports` formatter check (local-prefixes)
- gofmt: 5 files unformatted (`api/handlers_room.go`, `api/handlers_test.go`, `buffer/service.go`, `buffer/service_test.go`, `models/types.go`), which failed the CI format check
- CHANGELOG append-only: commit 6bd47e9 had rewritten the existing v1.1.5 and 1.0.0 entries; original lines restored, the change is now recorded here instead of rewriting history
- Docs: stale WebRTC references after WebRTC removal — comments in `internal/models/types.go` and `internal/api/handlers_room.go` corrected
- `.gitignore`: removed rules that swallowed working code (`internal/utils/debouncer.go`) and CI config (`.codecov.yml`)

### Removed

- Dead frontend code: `imediaplayer.h`, `iroommanager.h`, `itorrentmanager.h`, `mock_roommanager.h`, `mock_torrentmanager.*` and their gmock tests (no remaining references)

### Known Issues

1. Swagger cannot be regenerated: `@Failure {object} APIError` annotations reference a type that does not exist (the type is `models.ErrorResponse`), so `swag init` fails — board `t_0ab2e3d0`
2. Contract test does not compile against pact-go v2.5.1 (`provider.VerifierConfig` no longer exists) — board `t_7eebdc1c`
3. CI masks failures of contract, e2e and mutation tests via `|| true` — board `t_1452fa61`
4. `.gitignore` still ignores `config.yaml` globally and `backend/internal/testutil/` — board `t_6e2ff5cf`

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
