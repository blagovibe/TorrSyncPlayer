# TorrSyncPlayer

[![CI](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/ci.yml/badge.svg)](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/ci.yml)
[![Release](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/release.yml/badge.svg)](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Desktop torrent player with P2P playback synchronization.

## Features

- **Streaming playback** — instant viewing without full download
- **Sync rooms** — server-brokered synchronized viewing with friends (REST + SSE; no direct peer-to-peer data channel)
- **Security** — a per-process access token compared in constant time, per-IP rate limiting, bcrypt-hashed room passwords. No accounts, no JWT
- **Metrics** — Prometheus metrics for monitoring
- **Buffering** — LRU cache with piece download priorities
- **CI/CD** — GitHub Actions with golangci-lint, clang-tidy, tests, coverage ≥60%
- **Swagger** — interactive API documentation at `/swagger/`

## Tech Stack

- **Backend:** Go 1.26+, anacrolix/torrent v1.61.0, go-chi/chi/v5
- **Frontend:** C++17, Qt 6.5+, libmpv, CMake 3.16+
- **Build:** Make (backend), CMake (frontend)
- **CI/CD:** GitHub Actions

## Documentation

- [Concept](CONCEPT.md) — what the project is for and its boundaries
- [Dev](DEV.md) — architecture, stack, code conventions, test commands
- [API documentation](docs/API.md) — complete REST API reference (22 routes)
- [Architecture details](docs/ARCHITECTURE.md) — diagrams, request flows, data model
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
│   │   ├── auth/      # Per-process access token, constant-time compare, stream tickets
│   │   ├── buffer/    # LRU cache, piece priorities
│   │   ├── constants/ # All magic numbers extracted to constants
│   │   ├── errors/    # AppError, ErrorType
│   │   ├── metrics/   # Prometheus metrics
│   │   ├── models/    # Data models
│   │   ├── p2p/       # Sync rooms + SSE event relay (server-brokered)
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
├── CONCEPT.md         # What the project is for, and its boundaries
├── DEV.md             # How it is built: architecture, stack, conventions, tests
├── docs/              # Documentation
│   ├── API.md         # API documentation
│   ├── ARCHITECTURE.md # Diagrams, request flows, data model
│   ├── INSTALL.md     # Installation guide
│   ├── METRICS.md     # Prometheus metrics
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
├── tests/             # Chaos (toxiproxy), load (k6), and e2e suites
└── LICENSE            # MIT license
```

## API

### Main Endpoints

| Method | Path | Description | Authentication |
|--------|------|-------------|----------------|
| GET | `/health` | Health check | No |
| GET | `/api/v1/version` | Server version | No |
| GET | `/metrics` | Prometheus metrics | No |
| GET | `/swagger/*` | Interactive API docs | No |
| GET | `/api/v1/torrents/{id}/stream` | Stream file | Stream ticket (`?ticket=`) |
| GET | `/api/v1/torrents` | List torrents | Access token |
| POST | `/api/v1/torrents` | Add torrent | Access token |
| DELETE | `/api/v1/torrents/{id}` | Remove torrent | Access token |
| GET | `/api/v1/torrents/{id}/files` | List files | Access token |
| POST | `/api/v1/torrents/{id}/select` | Select file | Access token |
| POST | `/api/v1/torrents/{id}/stream-ticket` | Mint a short-lived stream ticket | Access token |
| POST | `/api/v1/torrents/{id}/buffer/position` | Set buffer position | Access token |
| GET | `/api/v1/torrents/{id}/buffer/info` | Buffer info | Access token |
| POST | `/api/v1/rooms` | Create room | Access token |
| POST | `/api/v1/rooms/join` | Join room | Access token |
| POST | `/api/v1/rooms/leave` | Leave room | Access token |
| POST | `/api/v1/rooms/signal` | Relay signal to room peers (SSE) | Access token |
| GET | `/api/v1/rooms/{roomID}/events` | SSE events | Access token |
| POST | `/api/v1/sync/play` | Sync play | Access token |
| POST | `/api/v1/sync/pause` | Sync pause | Access token |
| POST | `/api/v1/sync/seek` | Sync seek | Access token |
| GET | `/api/v1/sync/status` | Sync status | Access token |
| GET | `/api/v1/health/detailed` | Detailed health check | Access token |

There is no registration and no login. The server generates one random access
token at startup, prints it to the console, and forgets it — nothing is stored,
and nothing survives a restart. Send it in `X-Access-Token` on every request
above.

`/api/v1/torrents/{id}/stream` is the one exception: media players (libmpv)
cannot attach headers to their own HTTP fetches, so that endpoint takes a
short-lived HMAC-signed `?ticket=` minted via `/stream-ticket` instead.

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

1. **In-memory storage** — UserStore and TokenRevocationStore are not persistent (data is lost on restart) unless `--data-dir` is set
2. **No database integration** — a production deployment requires a database
3. **Frontend tests** — MainWindow and MpvWidget do not have unit tests
4. **SSE client disconnect detection** — backend detects client disconnect via context cancellation but does not immediately remove the peer from the room (relies on prune loop with 5-minute timeout)

## License

[MIT](LICENSE)
