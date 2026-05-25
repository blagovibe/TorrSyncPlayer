# Contributing to TorrSyncPlayer

Thank you for your interest in contributing! This document provides guidelines for setting up the project, making changes, and submitting pull requests.

## Prerequisites

- Node.js 18+
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
npm run electron:build:win       # Windows EXE (portable + NSIS)
```

## Quality Checks

Run all checks before submitting a PR:

```bash
npm run lint         # ESLint
npm run type-check   # TypeScript
npm run test         # Unit tests (Vitest)
```

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

## Coding Standards

- **TypeScript**: Strict mode is enabled. Avoid `any` types.
- **ESLint**: Follow the project's ESLint configuration. Run `npm run lint` before committing.
- **No console statements**: Use the structured logger from `src/utils/logger.ts` instead.
- **Type imports**: Use `import type` for type-only imports.
- **Cleanup pattern**: Use `createCleanup()` from `src/utils/cleanup.ts` for resource management in services.
- **JSDoc**: Add JSDoc comments to all public methods in service classes.

## Pull Request Process

1. Fork the repository and create a feature branch.
2. Make your changes, ensuring all quality checks pass.
3. Add tests for new functionality.
4. Update documentation if needed.
5. Submit a PR with a clear description of the changes.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
