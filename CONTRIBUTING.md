# TorrSyncPlayer Contributor Guide

Thank you for your interest in the TorrSyncPlayer project! This guide will help you get started with the codebase and contribute.

## Contents

1. [Environment Requirements](#environment-requirements)
2. [Setting Up the Workspace](#setting-up-the-workspace)
3. [Build Process](#build-process)
4. [Code Style](#code-style)
5. [Running Tests](#running-tests)
6. [Pull Request Process](#pull-request-process)
7. [Commit Conventions](#commit-conventions)
8. [Project Structure](#project-structure)
9. [Branching Strategy](#branching-strategy)
10. [Rollback Procedure](#rollback-procedure)

---

## Environment Requirements

### Backend (Go)

| Component | Minimum Version | Recommended |
|-----------|----------------|-------------|
| Go        | 1.25+          | Latest stable |
| Make      | 4.0+           | Latest       |

### Frontend (C++/Qt)

| Component | Minimum Version | Recommended |
|-----------|----------------|-------------|
| C++       | C++17          | C++20        |
| Qt        | 6.5+           | Latest LTS   |
| CMake     | 3.16+          | Latest       |
| libmpv    | 0.35+          | Latest       |

### Operating Systems

- **Windows:** Windows 10+ with MSVC 2022 or MinGW
- **Linux:** Ubuntu 20.04+ / Fedora 35+ / Arch Linux
- **macOS:** macOS 12+ with Xcode Command Line Tools

---

## Setting Up the Workspace

### 1. Clone the repository

```bash
git clone https://github.com/blagovibe/TorrSyncPlayer.git
cd TorrSyncPlayer
```

### 2. Install Go (Backend)

**Ubuntu/Debian:**
```bash
wget https://go.dev/dl/go1.25.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

**macOS:**
```bash
brew install go@1.24
```

**Windows:**
Download and install from the [official site](https://go.dev/dl/).

### 3. Install Qt and libmpv (Frontend)

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install -y \
    build-essential \
    cmake \
    ninja-build \
    qt6-base-dev \
    qt6-multimedia-dev \
    libmpv-dev \
    libgl1-mesa-dev
```

**macOS:**
```bash
brew install qt@6 mpv cmake ninja
```

**Windows:**
1. Install Qt 6.5+ from the [official site](https://www.qt.io/download)
2. Install libmpv via vcpkg or download binaries

### 4. Verify installation

```bash
# Go
go version

# CMake
cmake --version

# Qt (check via qmake)
qmake --version
```

---

## Build Process

### Build Backend

```bash
cd backend
make build
```

The executable will be in `backend/build/` or `backend/bin/`.

### Build Frontend

**Linux/macOS:**
```bash
cd frontend
mkdir -p build
cd build
cmake .. -G Ninja -DCMAKE_BUILD_TYPE=Release
ninja
```

**Windows:**
```bash
cd frontend
mkdir build
cd build
cmake .. -G "Visual Studio 17 2022" -A x64
cmake --build . --config Release
```

### Build Everything

```bash
# From project root
make all
```

### Clean

```bash
# Backend
cd backend
make clean

# Frontend
cd frontend/build
ninja clean

# Everything
make clean
```

---

## Code Style

### Go (Backend)

#### Formatting

- Use `gofmt` for code formatting
- Use `go vet` for static analysis
- Editor settings in [`.editorconfig`](../.editorconfig)

```bash
# Format
gofmt -w .

# Check
go vet ./...
```

#### Conventions

1. **Naming:**
   - CamelCase for exported functions and types
   - camelCase for unexported
   - Acronyms in uppercase: `HTTPClient`, `URLParser`

2. **Error handling:**
   ```go
   // Always wrap errors
   if err != nil {
       return fmt.Errorf("context description: %w", err)
   }
   ```

3. **Logging:**
   ```go
   // Use structured logger
   logger.Info("Operation completed", "key1", value1, "key2", value2)
   logger.Error("Error", "error", err)
   ```

4. **Comments:**
   - Code comments in English
   - Exported functions must have godoc comments
   ```go
   // AddMagnet adds a torrent by magnet link.
   // Parameter ctx is the context for operation cancellation.
   // Returns torrent info or an error.
   func (s *Service) AddMagnet(ctx context.Context, magnetURI string) (*models.TorrentInfo, error) {
   ```

5. **Constants:**
   - Extract magic numbers to named constants in the `constants` package
   ```go
   const (
       gracefulShutdownTimeout = 30 * time.Second
       dataDirPermissions     = 0755
   )
   ```

### C++ (Frontend)

#### Formatting

- Editor settings in [`.editorconfig`](../.editorconfig)
- Use clang-format for automatic formatting

#### Conventions

1. **Naming (Qt style):**
   - camelCase for methods: `addTorrent()`, `onPlayPause()`
   - camelCase for variables: `m_torrentList`, `m_network`
   - PascalCase for classes: `MainWindow`, `NetworkManager`

2. **Qt macros:**
   ```cpp
   class MainWindow : public QMainWindow
   {
       Q_OBJECT  // Always at the top of classes with signals/slots
   public:
       // ...
   signals:
       void torrentAdded(const QJsonObject &torrent);
   private slots:
       void onAddTorrent();
   };
   ```

3. **Comments:**
   - Code comments in English
   - Use Doxygen style for class documentation
   ```cpp
   /**
    * @brief Add a torrent by magnet link
    * @param magnetUri Magnet link for the torrent
    */
   void addTorrent(const QString &magnetUri);
   ```

4. **Memory management:**
   - Use Qt parent objects for automatic deletion
   - For non-Qt objects use `std::unique_ptr` or `std::shared_ptr`

---

## Running Tests

### Backend Tests

```bash
cd backend

# Run all tests
make test

# Run tests with coverage
go test -cover ./...

# Run tests for a specific package
go test ./internal/torrent/...
go test ./internal/p2p/...
go test ./internal/sync/...
go test ./internal/auth/...
go test ./internal/validation/...
go test ./internal/api/...

# Run with verbose output
go test -v ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Frontend Tests

```bash
cd frontend/build

# Run tests via CTest
ctest --output-on-failure

# Or directly
./test_networkmanager
./test_torrentmodel
```

### Integration Tests

```bash
cd backend

# Run integration tests
go test -tags=integration ./internal/api/...
```

---

## Pull Request Process

### 1. Create a branch

```bash
# Create a branch from develop
git checkout -b feature/your-feature-name
# or
git checkout -b fix/your-bug-fix
```

### 2. Make changes

- Write clean, readable code
- Follow code style conventions
- Add tests for new functionality
- Update documentation as needed

### 3. Before submitting

```bash
# Make sure all tests pass
make test

# Check formatting
gofmt -l .
go vet ./...

# Verify the build
make all
```

### 4. Create a Pull Request

1. Push the branch:
   ```bash
   git push origin feature/your-feature-name
   ```

2. Create a Pull Request on GitHub

3. Fill in the PR template:
   - **Description:** What was changed and why
   - **Type of change:** Feature / Bug fix / Documentation / Refactor
   - **Testing:** How the changes were tested
   - **Related issues:** Links to related tasks

### 5. Code Review

- Wait for review from maintainers
- Make corrections based on comments
- Make sure CI passes

---

## Commit Conventions

We use the [Conventional Commits](https://www.conventionalcommits.org/) format:

### Format

```
<type>(<scope>): <short description>

<detailed description (optional)>

<footer (optional)>
```

### Commit Types

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation changes |
| `style` | Formatting (does not affect code) |
| `refactor` | Code refactoring |
| `test` | Adding/changing tests |
| `chore` | Maintenance changes |
| `perf` | Performance improvements |
| `ci` | CI/CD changes |

### Examples

```
feat(torrent): add magnet link support

- Implemented magnet URI format validation
- Added metadata fetch timeout handling
- Added tests for new functions

Closes #123
```

```
fix(p2p): fix DataChannel close bug

DataChannel was not closed correctly when leaving a room,
which caused resource leaks.

Fixes #456
```

```
docs(readme): update installation guide

- Added Windows instructions
- Fixed documentation links
```

```
refactor(sync): extract magic numbers to constants

- maxPositionJump = 2.0
- smoothAdjustmentRatio = 0.3
- msPerSecond = 1000.0
```

---

## Project Structure

```
TorrSyncPlayer/
├── backend/                    # Go backend
│   ├── cmd/server/             # Entry point
│   │   └── main.go
│   ├── internal/               # Internal packages
│   │   ├── api/                # HTTP API (router, handlers, middleware, tests)
│   │   ├── auth/               # JWT authentication (HS256, bcrypt, revocation)
│   │   ├── buffer/             # LRU cache, piece priorities
│   │   ├── constants/          # Constants (magic numbers)
│   │   ├── errors/             # AppError, ErrorType
│   │   ├── metrics/            # Prometheus metrics
│   │   ├── models/             # Data models
│   │   ├── p2p/                # WebRTC P2P service
│   │   ├── storage/            # In-memory storage
│   │   ├── sync/               # Playback synchronization
│   │   ├── torrent/            # Torrent service
│   │   ├── validation/         # Validation
│   │   ├── version/            # Version
│   │   └── interfaces.go       # Service interfaces
│   ├── pkg/logger/             # slog-based logger
│   ├── docs/                   # Swagger spec
│   │   ├── docs.go
│   │   ├── swagger.json
│   │   └── swagger.yaml
│   ├── Makefile
│   └── go.mod
│
├── frontend/                   # Qt/C++ frontend
│   ├── src/                    # Source files
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
│   ├── resources/              # Resources
│   ├── CMakeLists.txt
│   └── build.sh / build.bat
│
├── docs/                       # Documentation
│   ├── API.md                  # API documentation
│   ├── ARCHITECTURE.md         # Architecture overview
│   ├── ARCHITECTURE_BACKEND.md # Backend architecture
│   ├── ARCHITECTURE_FRONTEND.md # Frontend architecture
│   ├── ARCHITECTURE_P2P.md     # P2P/WebRTC architecture
│   ├── INSTALL.md              # Installation guide
│   ├── USER_GUIDE.md           # User guide
│   └── METRICS.md              # Metrics reference
│
├── .github/                    # GitHub Actions
│   └── workflows/
│       ├── ci.yml              # CI pipeline
│       └── release.yml         # Release pipeline
│
├── .editorconfig               # Editor settings
├── .gitignore
├── CHANGELOG.md
├── CONTRIBUTING.md
├── LICENSE
├── Makefile
├── README.md
├── Dockerfile
└── docker-compose.yml
```

---

## Branching Strategy

- `main` — stable release branch
- `develop` — integration branch for features
- `feature/*` — feature branches (branched from `develop`)
- `hotfix/*` — hotfix branches (branched from `main`)

### Code Review Process
1. Create a feature branch from `develop`
2. Open a PR to `develop` when ready
3. At least 1 approval is required to merge
4. All CI checks must pass
5. Squash merge to keep history clean

---

## Rollback Procedure

1. Identify the problematic release tag
2. Revert the merge commit on `main`: `git revert -m 1 <merge-commit>`
3. Create a hotfix branch if needed
4. Tag a new release after verification

---

## Useful Links

- [Architecture documentation](docs/ARCHITECTURE.md)
- [API documentation](docs/API.md)
- [User guide](docs/USER_GUIDE.md)
- [Installation guide](docs/INSTALL.md)
- [Changelog](CHANGELOG.md)

---

## Contact

If you have questions, create an [Issue](https://github.com/blagovibe/TorrSyncPlayer/issues) on GitHub.
