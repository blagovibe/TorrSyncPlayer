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
- There is no user store. Auth is a per-process access token, so there are no usernames to normalize

### C++/Qt
- Use `m_` prefix for member variables
- Use camelCase for methods
- Sanitize all user-controlled values before URL construction

## Key Security Considerations
- Auth is a single access token generated at startup and compared with crypto/subtle.ConstantTimeCompare. No JWT, no accounts, no CSRF store: a cross-origin page cannot read the token and cannot set the X-Access-Token header without a preflight the server rejects
- Self-signed certs use random serial numbers
- Auto-generated temp cert files are cleaned up on shutdown; user-provided certs are preserved
- Metrics endpoint (/metrics) is per-IP rate limited but not token-protected (for Prometheus scraping)
- MemoryStorageCapacity has an upper bound of 256GB (MaxMemoryStorageCapacity)

## Testing
- Backend: `cd backend && make test`
- Frontend: `cd frontend/build && ctest --output-on-failure`
- Race detection: `cd backend && make test-race`
