# AGENTS.md

This file provides guidance to agents when working with code in this repository.

## Stack

- **Backend:** Go 1.25, Wails v2 (desktop framework)
- **Frontend:** React 18 + TypeScript, Vite 8, Vitest, Playwright
- **Torrent:** anacrolix/torrent
- **P2P:** pion/webrtc v4
- **DI:** Custom container (`container/container.go`)

## Build & Dev Commands

```bash
make install          # Install Go + npm deps
make dev              # Wails dev mode (hot reload)
make build            # Build for current platform
make build-windows    # Cross-compile Windows
make test             # Go tests (./...)
make test-verbose     # Go tests with -v
make test-coverage    # Go tests + HTML coverage report
make lint             # go vet + frontend lint
make build-prod       # Optimized build (-ldflags "-s -w")
```

Frontend tests: `cd frontend && npm run test` (vitest watch) / `npm run test:run` (single run) / `npm run test:e2e` (playwright).

## Architecture

All Go backend code is in the project root (single `package main`). Three core services:

- **`TorrentService`** (`services.go`) — torrent management + HTTP streaming server (port 8888, auto-increments)
- **`P2PService`** (`p2p_service.go`) — WebRTC peer connections, room management, heartbeat
- **`SyncService`** (`sync_service.go`) — playback sync with latency compensation

Services are registered as singletons in a custom DI container (`container/container.go`) and wired together in `main.go`. `SyncService.SetP2PService()` is called after all services are initialized to break the circular dependency.

Interfaces for all services are defined in `interfaces.go`. Compile-time interface checks use `var _ Interface = (*Impl)(nil)` pattern.

## Code Conventions

- **Go:** Standard `gofmt`/`go vet`. Use `logger.Info/Warn/Error/Debug` from `logger/` package (slog-based, respects `LOG_LEVEL` and `LOG_FORMAT` env vars).
- **Error wrapping:** Always use `fmt.Errorf("...: %w", err)`.
- **Mutex:** Embed `sync.RWMutex` directly in service structs; use `mu.Lock()`/`mu.RLock()` (not defer for RLock in hot paths).
- **Comments:** All exported functions and types must have Go doc comments. Code comments are in Russian.
- **Security:** Magnet URIs are sanitized via `SanitizeLogValue()` before logging. File paths validated via `validateFilePath()` (no null bytes, no path traversal, `.torrent` extension only). Room passwords hashed with bcrypt.
- **Testing:** Mocks for all three services live in `mocks_test.go`. Tests use `testify/assert` and `testify/require`. Test files are co-located with source files (e.g., `services_test.go`).

## Key Gotchas

- `SyncService` depends on `P2PService` but is initialized first — `SetP2PService()` is called manually in `app.startup()` after all `Init()` calls complete.
- HTTP stream server port auto-increments (up to +9) if the default port 8888 is occupied.
- `TorrentService.emitEvent()` and `P2PService.emitEvent()` silently skip if `ctx` is nil — they won't crash but events will be lost.
- `SyncService.Close()` uses `sync.Once` — safe to call multiple times.
- Wails bindings must be regenerated with `wails generate module` after changing Go method signatures exposed to frontend.
- Frontend Wails JS bindings live in `frontend/wailsjs/` — do not edit manually.
