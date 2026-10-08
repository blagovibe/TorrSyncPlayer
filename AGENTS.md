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

## Checks must be able to fail

A green build is only worth something if a broken build can turn it red. For
this reason the following are banned outside a deliberate, commented exception:

- `|| true` at the end of a Makefile recipe
- `continue-on-error: true` on a CI step or job
- trailing `-` on a shell command in a recipe
- a test that only asserts a non-nil pointer, or that reproduces the logic it
  is meant to check

Two bugs reached main with a fully green pipeline precisely because of this:
the SSE handler answered 500 because a middleware wrapper dropped
`http.Flusher`, and the stream ticket signature was computed over an empty
string, which made it authenticate nothing. Both were invisible because no
check could fail.

When a check looks flaky, fix the flake. Do not silence the check.

Note that `test-frontend-fuzz` and `test-frontend-mutation` are honest
placeholders that only echo; they promise nothing, which is correct. The rule
above is about checks that *look* real and are not.
