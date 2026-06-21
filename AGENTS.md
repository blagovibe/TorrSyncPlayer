# AGENTS.md — AI Agent Guide for TorrSyncPlayer

## Project Overview

TorrSyncPlayer is a desktop torrent player with P2P synchronization. It consists of:
- **Backend:** Go HTTP API (torrent management, P2P rooms, sync, auth)
- **Frontend:** Qt/C++ desktop application (libmpv video player)

## Project Structure

```
TorrSyncPlayer/
├── backend/           # Go backend
│   ├── cmd/server/    # Entry point (main.go)
│   ├── internal/      # Packages (api, auth, buffer, metrics, p2p, sync, torrent, etc.)
│   ├── pkg/           # Shared packages (logger, response)
│   └── docs/          # Swagger docs
├── frontend/          # Qt/C++ frontend
│   ├── src/           # Source files
│   └── resources/     # Icons, etc.
├── docs/              # Documentation (API.md, ARCHITECTURE.md, INSTALL.md, USER_GUIDE.md)
├── .github/           # CI/CD workflows

└── Makefile
```

## Code Conventions

### Go
- All magic numbers go in `internal/constants/constants.go`
- Use structured errors from `internal/errors/errors.go`
- Interface definitions go in `internal/interfaces.go`
- Use `errors.As` for error type checking (not direct type assertion)
- Normalize usernames to lowercase in auth store

### C++/Qt
- Use `m_` prefix for member variables
- Use camelCase for methods
- Sanitize all user-controlled values before URL construction

## Key Security Considerations
- JWT tokens are parsed once per request (JTI extracted from already-parsed claims via middleware context, not re-parsed)
- CSRF protection is skipped for requests with valid JWT Bearer tokens (API clients not vulnerable to browser-based CSRF)
- Self-signed certs use random serial numbers
- Auto-generated temp cert files are cleaned up on shutdown; user-provided certs are preserved
- Usernames are case-insensitive (stored lowercase)
- Metrics endpoint (/metrics) is per-IP rate limited but not JWT-protected (for Prometheus scraping)
- MemoryStorageCapacity has an upper bound of 256GB (MaxMemoryStorageCapacity)

## Testing
- Backend: `cd backend && make test`
- Frontend: `cd frontend/build && ctest --output-on-failure`
- Race detection: `cd backend && make test-race`
