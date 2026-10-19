# Contributor Guide

Thank you for your interest in TorrSyncPlayer!

Everything below is written for the project as it is today: a Go backend and a
Go/Fyne desktop client. `DEV.md` is the reference for how the code is built,
run and tested — this file is the short version for someone starting out.

## Requirements

| Component | Minimum |
|-----------|---------|
| Go | 1.26+ (pinned as `GO_VERSION` in every workflow) |
| Fyne build headers | `build-essential pkg-config libgl1-mesa-dev libx11-dev libxkbcommon-dev libfontconfig1-dev` |
| mpv | installed separately — it is an external process, not a library |

Without a display, Fyne tests hang rather than fail, so `FYNE_DRIVER=offscreen`
is mandatory locally and in CI.

See DEV.md, "Зависимости", for macOS and Windows specifics.

## Quick Start

```bash
# Build backend
cd backend
make build

# Build the client together with its backend (they are one pair)
cd ../frontend
go run ./cmd/build          # → ./dist/TorrSyncPlayer + ./dist/torrsyncplayer
./dist/TorrSyncPlayer
```

The application starts its own backend on a free port and waits for it, so
there is nothing to launch separately for normal use.

## Running Tests

The authoritative list of targets and what each one covers is in
[DEV.md, "Соглашения по тестам"](DEV.md#Соглашения-по-тестам). The short version:

```bash
cd backend
make test                    # All backend tests
make test-race               # With race detector
make test-coverage           # With coverage report
make test-ci-contract        # Checks CI describes this repo correctly (-count=1)

cd ../frontend
FYNE_DRIVER=offscreen go test ./...
FYNE_DRIVER=offscreen go test -race ./...            # as in CI
FYNE_DRIVER=offscreen go test ./internal/integration  # scenario tests, real binaries
```

Two conventions are worth knowing before you are surprised by them:

- The scenario tests under `frontend/internal/integration` build and launch a
  real backend binary and drive the real client against it. They are the only
  tests that can catch a disagreement between two peers, and they are slow.
- `internal/cicontract` reads `.github/workflows/ci.yml` from disk, so it must
  be run with `-count=1`: a file the test reads is not an input Go knows about,
  and the result cache would happily return a pass measured against yesterday's
  workflow.

## Code Style

### Go (both modules)

- `gofmt` for formatting, `go vet ./...` before pushing
- CamelCase for exported names, camelCase for unexported
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Comments explain why, not what — and they say the rule the code enforces, so
  the next reader does not have to re-derive it
- Magic numbers live in named constants next to the code that uses them
  (`backend/internal/constants/constants.go` for the backend's shared ones);
  a bare literal in a condition is a smell
- Anything visible to a person goes through the translation catalog
  (`frontend/internal/catalog`), never a string literal in a widget

### Platform code

Platform-specific code lives in `*_unix.go` / `*_windows.go` files with a
build tag and a comment saying what the platform cannot do, not what the code
works around.

## PR Process

1. Branch from `main`:
   ```
   git checkout -b feature/your-feature
   ```
2. Make changes, add tests
3. Verify: `make test && go vet ./...` (backend), `FYNE_DRIVER=offscreen go test ./...` (frontend)
4. Push and open a PR to `main`
5. CI must pass; backend statement coverage must stay at or above the floor
   enforced in `ci.yml` (currently 55%)

## Commit Format

```
<type>(<scope>): <description>

<optional body>
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `ci`

## Questions?

Open an [issue](https://github.com/blagovibe/TorrSyncPlayer/issues).