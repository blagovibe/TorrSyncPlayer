# TorrSyncPlayer

[![CI](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/ci.yml/badge.svg)](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/ci.yml)
[![Release](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/release.yml/badge.svg)](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Desktop torrent player with P2P playback synchronization.

## Features

- **Streaming playback** — instant viewing without full download
- **P2P rooms** — synchronized viewing with friends via WebRTC
- **Security** — JWT authentication (HS256), CSRF protection, rate limiting, bcrypt passwords
- **Metrics** — Prometheus metrics for monitoring
- **Buffering** — LRU cache with piece download priorities
- **CI/CD** — GitHub Actions with golangci-lint, clang-tidy, tests, coverage ≥60%
- **Swagger** — interactive API documentation at `/swagger/`

## Tech Stack

- **Backend:** Go 1.26+, anacrolix/torrent v1.61.0, pion/webrtc v4, go-chi/chi/v5, golang-jwt/jwt/v5
- **Frontend:** C++17, Qt 6.5+, libmpv, CMake 3.16+
- **Build:** Make (backend), CMake (frontend)
- **CI/CD:** GitHub Actions

## Documentation

- [API documentation](docs/API.md) — complete REST API reference (22 routes)
- [Architecture](docs/ARCHITECTURE.md) — backend, frontend, and P2P architecture
- [User Guide](docs/USER_GUIDE.md) — usage instructions
- [Installation Guide](docs/INSTALL.md) — installation and configuration
- [Changelog](CHANGELOG.md) — version history
- [Swagger UI](http://localhost:8889/swagger/) — interactive API docs (when server is running)

## Quick Start

### Backend

```bash
cd backend
make build
make run
```

The server will start on port 8889.

### Frontend

```bash
cd frontend
./build.sh  # Linux/macOS
build.bat   # Windows
```

## Running

```bash
# Terminal 1
cd backend && make run

# Terminal 2
cd frontend/build && ./TorrSyncPlayer
```

## Project Structure

```
TorrSyncPlayer/
├── backend/           # Go backend (HTTP API + P2P + Torrent)
│   ├── cmd/server/    # Entry point (main.go, 408 lines)
│   ├── internal/
│   │   ├── api/       # HTTP API (router, handlers, middleware, tests)
│   │   ├── auth/      # JWT authentication (HS256, bcrypt, token revocation)
│   │   ├── buffer/    # LRU cache, piece priorities
│   │   ├── constants/ # All magic numbers extracted to constants
│   │   ├── errors/    # AppError, ErrorType
│   │   ├── metrics/   # Prometheus metrics
│   │   ├── models/    # Data models
│   │   ├── p2p/       # WebRTC P2P service, rooms
│   │   ├── storage/   # In-memory storage
│   │   ├── sync/      # Playback sync with latency compensation
│   │   ├── torrent/   # Torrent management + HTTP streaming
│   │   ├── validation/# Input validation
│   │   └── version/   # Version info
│   ├── pkg/logger/    # slog-based logger
│   ├── docs/          # Swagger spec (swagger.yaml, swagger.json, docs.go)
│   ├── Makefile
│   └── go.mod
│
├── frontend/          # Qt/C++ frontend
│   ├── src/           # Source files
│   │   ├── main.cpp
│   │   ├── mainwindow.h/.cpp
│   │   ├── mpvwidget.h/.cpp
│   │   ├── networkmanager.h/.cpp
│   │   ├── torrentmodel.h/.cpp
│   │   ├── torrentmanager.h/.cpp
│   │   ├── roommanager.h/.cpp
│   │   ├── roomdialog.h/.cpp
│   │   ├── systemtray.h/.cpp
│   │   ├── utils.h/.cpp
│   │   ├── inetworkmanager.h
│   │   ├── test_torrentmodel.cpp
│   │   └── test_networkmanager.cpp
│   ├── resources/     # Resources (icons, etc.)
│   ├── CMakeLists.txt
│   └── build.sh / build.bat
│
├── docs/              # Documentation
│   ├── API.md         # API documentation
│   ├── ARCHITECTURE.md # Architecture (backend, frontend, P2P)
│   ├── INSTALL.md     # Installation guide
│   └── USER_GUIDE.md  # User guide
│
├── .github/           # GitHub Actions workflows
│   └── workflows/
│       ├── ci.yml     # CI pipeline (lint, test, build, coverage)
│       └── release.yml # Release pipeline
│
├── CHANGELOG.md       # Version history
├── CONTRIBUTING.md    # Contributor guide
├── AGENTS.md          # AI agent guide
└── LICENSE            # MIT license
```

## API

### Main Endpoints

| Method | Path | Description | Authentication |
|--------|------|-------------|----------------|
| GET | `/health` | Health check | No |
| GET | `/api/v1/version` | Server version | No |
| GET | `/metrics` | Prometheus metrics | No |
| GET | `/api/v1/csrf-token` | Get CSRF token | No |
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
| GET | `/api/v1/rooms/{roomID}/events` | SSE events | JWT |
| POST | `/api/v1/sync/play` | Sync play | JWT |
| POST | `/api/v1/sync/pause` | Sync pause | JWT |
| POST | `/api/v1/sync/seek` | Sync seek | JWT |
| GET | `/api/v1/sync/status` | Sync status | JWT |
| GET | `/api/v1/health/detailed` | Detailed health check | JWT |

Full API documentation is available in [docs/API.md](docs/API.md) and in Swagger UI at `/swagger/`.

## Testing

```bash
# Backend tests
cd backend
make test

# Backend tests with coverage
go test -cover ./...

# Frontend tests
cd frontend/build
ctest --output-on-failure
```

## Known Limitations

1. **In-memory storage** — UserStore and TokenRevocationStore are not persistent (data is lost on restart)
2. **No database integration** — a production deployment requires a database
3. **Buffer Service** — lacks unit tests
4. **Frontend tests** — MainWindow and MpvWidget do not have unit tests
5. **SSL in NetworkManager** — SSL errors are ignored in debug mode; production behavior is not implemented
6. **--server-url flag** — when launching frontend with `--server-url`, the URL is parsed but not passed to NetworkManager

## License

[MIT](LICENSE)
