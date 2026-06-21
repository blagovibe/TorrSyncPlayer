# Contributor Guide

Thank you for your interest in TorrSyncPlayer!

## Requirements

### Backend
| Component | Minimum |
|-----------|---------|
| Go | 1.26+ |

### Frontend
| Component | Minimum |
|-----------|---------|
| C++ | C++17 |
| Qt | 6.5+ |
| CMake | 3.16+ |
| libmpv | 0.35+ |

## Quick Start

```bash
# Build backend
cd backend
make build

# Build frontend (Linux/macOS)
cd frontend/build
cmake .. -G Ninja -DCMAKE_BUILD_TYPE=Release
ninja

# Run backend
cd backend && make run

# Run frontend
cd frontend/build && ./TorrSyncPlayer
```

## Running Tests

```bash
cd backend
make test                    # All backend tests
make test-race               # With race detector
make test-coverage           # With coverage report

cd frontend/build
ctest --output-on-failure    # Frontend tests
```

## Code Style

### Go
- Use `gofmt` for formatting
- Run `go vet ./...` before pushing
- CamelCase for exported names, camelCase for unexported
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Extract magic numbers to `internal/constants/constants.go`

### C++
- Qt-style naming: `camelCase` for methods, `m_` prefix for members, `PascalCase` for classes
- Use clang-format for formatting
- Use Qt parent objects or `std::unique_ptr` for memory management

## PR Process

1. Branch from `develop`:
   ```
   git checkout -b feature/your-feature
   ```
2. Make changes, add tests
3. Verify: `make test && go vet ./...`
4. Push and open a PR to `develop`
5. CI must pass, coverage >= 60%

## Commit Format

```
<type>(<scope>): <description>

<optional body>
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `ci`

## Questions?

Open an [issue](https://github.com/blagovibe/TorrSyncPlayer/issues).
