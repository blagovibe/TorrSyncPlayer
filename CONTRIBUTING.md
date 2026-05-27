# Contributing to TorrSyncPlayer

Thank you for your interest in contributing! This document provides guidelines for setting up the project, making changes, and submitting pull requests.

## Prerequisites

- Node.js >= 20 (see `client/package.json` engines field)
- npm 9+

## Setup

```bash
cd client/
npm ci
```

## Development

```bash
npm run dev          # Start Vite dev server
npm run electron:dev # Start Electron with hot reload
```

## Build

```bash
npm run build                    # Web build
npm run electron:build:linux     # Linux AppImage
npm run electron:build:win       # Windows portable EXE
```

## Quality Checks

Run all checks before submitting a PR:

```bash
npm run lint         # ESLint
npm run type-check   # TypeScript (strict — no unused locals/parameters)
npm run test         # Unit tests (Vitest)
npm run test:electron # Electron-specific tests
```

All checks must pass. CI enforces lint, type-check, and tests on both Ubuntu and Windows.

## Project Structure

```
TorrSyncPlayer/
├── client/                  # Main application (React + Electron)
│   ├── src/
│   │   ├── components/      # React UI components
│   │   │   ├── __tests__/   # Component tests
│   │   ├── services/        # Business logic (P2P, Sync, Torrent)
│   │   │   ├── __tests__/   # Service tests
│   │   ├── hooks/           # Custom React hooks
│   │   ├── contexts/        # React context providers
│   │   ├── utils/           # Helpers and utilities
│   │   │   ├── __tests__/   # Utility tests
│   │   ├── shims/           # Browser polyfills/shims
│   │   ├── types/           # TypeScript type declarations
│   │   ├── config.ts        # Application configuration
│   │   ├── App.tsx          # Root component
│   │   └── main.tsx         # Entry point
│   ├── electron/            # Electron main process
│   │   ├── main.cjs         # Main process entry
│   │   ├── preload.cjs      # Preload script
│   │   └── torrent-bridge.cjs # Torrent bridge for Electron
│   └── package.json
└── README.md
```

## Architecture Overview

TorrSyncPlayer is an Electron desktop app for P2P synchronized torrent video streaming.

### Service Layer
- **P2PService** — Manages PeerJS connections, state machine (`disconnected` → `connecting` → `connected` → etc.), reconnection with exponential backoff, and rate limiting.
- **TorrentService** — Wraps WebTorrent (browser) or Electron torrent backend. Supports both backends but never mixed in the same instance.
- **SyncService** — Master-slave playback sync via heartbeat (2s interval). Latency-compensated position alignment with configurable tolerance.

### Key Patterns
- **Cleanup pattern**: All services use `createCleanup()` from `src/utils/cleanup.ts` for resource management. Call `cleanup.abort()` on destroy.
- **State machine**: P2PService enforces valid state transitions via `VALID_TRANSITIONS` map.
- **Error boundaries**: React error boundaries at room and chat levels prevent total UI crashes.

## Coding Standards

- **TypeScript**: Strict mode is enabled with `noUnusedLocals` and `noUnusedParameters`. Avoid `any` types.
- **ESLint**: Follow the project's ESLint configuration. Run `npm run lint` before committing.
- **No console statements**: Use the structured logger from `src/utils/logger.ts` instead.
- **Type imports**: Use `import type` for type-only imports.
- **JSDoc**: Add JSDoc comments to all public methods in service classes.
- **Testing**: All new features must include unit tests. Bug fixes must include a regression test.

## Branch Naming

Use the following branch naming conventions:

- `feature/<short-description>` — new features
- `fix/<short-description>` — bug fixes
- `docs/<short-description>` — documentation changes
- `refactor/<short-description>` — code refactoring
- `test/<short-description>` — test additions or fixes

## Commit Conventions

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat: add audio track selection`
- `fix: resolve memory leak in torrent bridge`
- `docs: update README with new build instructions`
- `refactor: extract P2P rate limiter into separate module`
- `test: add unit tests for SyncService`

Keep commits small and focused on a single change. Each commit should pass all quality checks.

## Pull Request Process

1. Fork the repository and create a feature branch.
2. Make your changes, ensuring all quality checks pass.
3. Add tests for new functionality. Bug fixes must include regression tests.
4. Update documentation if needed (README, IPC_API.md, SECURITY.md, SPEC.md).
5. Submit a PR with a clear description of the changes. Keep PRs focused — avoid mixing unrelated changes.
6. Reference any related issues in the PR description (e.g., `Closes #123`).

### PR Checklist
- [ ] `npm run lint` passes
- [ ] `npm run type-check` passes
- [ ] `npm run test` passes
- [ ] `npm run test:electron` passes
- [ ] New functionality has tests
- [ ] Documentation updated (if applicable)

## Release Process

Releases are automated via GitHub Actions. Push a tag `v*` to trigger:
1. CI checks (lint, type-check, tests)
2. Build for Linux (AppImage) and Windows (portable EXE)
3. GitHub draft release with artifacts

Format follows semantic versioning: `v<major>.<minor>.<patch>`.

## Security

- Never commit secrets, API keys, or credentials.
- All IPC inputs are validated at the preload boundary.
- CSP headers are generated dynamically based on configured PeerJS server.
- Report security vulnerabilities privately to the maintainers.

## Formatting

The project uses `.editorconfig` and `.prettierrc` for consistent formatting. Configure your editor to respect these files.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
