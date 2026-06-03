# AGENTS.md

This file provides guidance to agents when working with code in this repository.

## Stack

- **Backend:** Go 1.24+, anacrolix/torrent v1.58.1, pion/webrtc v4, go-chi/chi/v5, golang-jwt/jwt/v5
- **Frontend:** C++17, Qt 6.5+, libmpv, CMake 3.16+
- **Build:** Make (backend), CMake (frontend)
- **CI/CD:** GitHub Actions (golangci-lint, clang-tidy, tests, coverage ≥60%)
- **Docker:** multi-stage build (golang:1.25-alpine → alpine:3.19)

## Project Structure

```
TorrSyncPlayer/
├── backend/           # Go backend (HTTP API server, port 8889)
│   ├── cmd/server/    # Entry point (main.go, 408 lines)
│   ├── internal/
│   │   ├── api/       # HTTP API (router, handlers, middleware, tests)
│   │   ├── auth/      # JWT auth (HS256, bcrypt, token revocation)
│   │   ├── buffer/    # LRU cache, piece priorities
│   │   ├── constants/ # All magic numbers extracted
│   │   ├── errors/    # AppError, ErrorType
│   │   ├── metrics/   # Prometheus metrics
│   │   ├── models/    # Data models
│   │   ├── p2p/       # WebRTC peer connections, rooms
│   │   ├── storage/   # In-memory storage
│   │   ├── sync/      # Playback sync with latency compensation
│   │   ├── torrent/   # Torrent management + HTTP streaming
│   │   ├── validation/# Input validation
│   │   └── version/   # Version info
│   ├── pkg/logger/    # slog-based logger
│   └── docs/          # Swagger specification
└── frontend/          # Qt/C++ frontend
    ├── src/           # Source files
    ├── resources/     # Icons and resources
    └── CMakeLists.txt # Build configuration
```

## Build & Dev Commands

```bash
# Backend
cd backend
make build           # Build Go backend
make run             # Run backend server
make test            # Run Go tests
make clean           # Clean build artifacts

# Frontend
cd frontend
./build.sh           # Build Qt frontend (Linux/macOS)
build.bat            # Build Qt frontend (Windows)

# Root
make all             # Build both backend and frontend
make clean           # Clean both
```

## Architecture

### Backend (Go)
- HTTP REST API server on port 8889 with go-chi/chi/v5
- Middleware pipeline: SecurityHeaders → Recovery → CORS → Logger → CSRF → RateLimit → Auth
- Graceful shutdown (30s timeout)
- TLS 1.2+ support
- pprof on port 6060 (optional)
- `TorrentService` — torrent management + HTTP streaming (always in-memory storage)
- `P2PService` — WebRTC peer connections, room management
- `SyncService` — playback sync with latency compensation
- `BufferService` — LRU cache with piece priorities
- Services are independent (no DI container)

### Frontend (Qt/C++)
- `MpvWidget` — libmpv video player widget (OpenGL, QMutex, seek debounce)
- `NetworkManager` — HTTP client with retry logic (exponential backoff, max 3)
- `TorrentModel` — QAbstractListModel for torrent list
- `TorrentManager` — torrent operations manager
- `RoomManager` — P2P room management
- `MainWindow` — main application window
- `RoomDialog` — create/join room dialog
- `SystemTray` — system tray integration
- SSE for real-time room events

### Communication
- HTTP REST API (JSON)
- Server-Sent Events (SSE) for real-time room events
- WebRTC DataChannel for P2P sync

## API Endpoints (22 routes)

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/health` | Health check | No |
| GET | `/api/v1/version` | Server version | No |
| GET | `/metrics` | Prometheus metrics | No |
| GET | `/api/v1/csrf-token` | Get CSRF token | No |
| GET | `/swagger/` | Swagger UI | No |
| POST | `/api/v1/auth/register` | Register | No |
| POST | `/api/v1/auth/login` | Login | No |
| POST | `/api/v1/auth/logout` | Logout | No |
| GET | `/api/v1/torrents` | List torrents | JWT |
| POST | `/api/v1/torrents` | Add torrent | JWT |
| DELETE | `/api/v1/torrents/{id}` | Remove torrent | JWT |
| GET | `/api/v1/torrents/{id}/files` | List files | JWT |
| POST | `/api/v1/torrents/{id}/select` | Select file | JWT |
| GET | `/api/v1/torrents/{id}/stream` | Stream file | JWT |
| POST | `/api/v1/torrents/{id}/buffer/position` | Set buffer position | JWT |
| GET | `/api/v1/torrents/{id}/buffer/info` | Buffer info | JWT |
| POST | `/api/v1/rooms` | Create room | JWT |
| POST | `/api/v1/rooms/join` | Join room | JWT |
| POST | `/api/v1/rooms/leave` | Leave room | JWT |
| POST | `/api/v1/rooms/signal` | WebRTC signal | JWT |
| GET | `/api/v1/rooms/{roomID}/events` | Room events (SSE) | JWT |
| POST | `/api/v1/sync/play` | Sync play | JWT |
| POST | `/api/v1/sync/pause` | Sync pause | JWT |
| POST | `/api/v1/sync/seek` | Sync seek | JWT |
| GET | `/api/v1/sync/status` | Sync status | JWT |
| GET | `/api/v1/health/detailed` | Detailed health check | JWT |

## Security

- JWT authentication (HS256, 24h TTL, JTI for revocation)
- bcrypt password hashing (cost=12)
- CSRF protection (token store with TTL 1h)
- Rate limiting (10 req/min for auth, 60 req/min for API)
- Security headers (X-Content-Type-Options, X-Frame-Options, HSTS)
- CORS policies
- Input validation
- TLS 1.2+ support

## Code Conventions

- **Go:** Standard `gofmt`/`go vet`. Use `logger.Info/Warn/Error/Debug` from `pkg/logger`.
- **Error wrapping:** Always use `fmt.Errorf("...: %w", err)`.
- **C++:** Qt naming conventions, camelCase for methods.
- **Comments:** Code comments in Russian.
- **Constants:** Extract magic numbers to `internal/constants` package.

## Key Files

- `backend/cmd/server/main.go` — Backend entry point
- `backend/internal/api/router.go` — API route definitions
- `backend/internal/api/paths.go` — API path constants
- `backend/internal/constants/constants.go` — All constants
- `backend/internal/auth/auth.go` — JWT authentication logic
- `backend/internal/buffer/service.go` — LRU buffer cache
- `backend/internal/metrics/metrics.go` — Prometheus metrics
- `frontend/src/main.cpp` — Frontend entry point
- `frontend/src/mainwindow.cpp` — Main window implementation
- `frontend/src/mpvwidget.cpp` — Video player widget
- `frontend/src/networkmanager.cpp` — HTTP client
- `frontend/CMakeLists.txt` — Frontend build configuration

## Known Issues

1. `frontend/src/main.cpp:336` — `--server-url` flag parses URL but doesn't pass it to NetworkManager
2. `frontend/src/networkmanager.cpp:301` — SSL errors ignored in debug mode, production behavior not implemented
3. In-memory UserStore/TokenRevocationStore — no persistence
4. No database integration
5. Buffer Service has no unit tests
6. Frontend MainWindow/MpvWidget have no unit tests
