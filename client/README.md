# TorrSyncPlayer Client

The main TorrSyncPlayer application: React, TypeScript, Vite, Electron, PeerJS, and WebTorrent.

## Quick Start

```bash
cd client/
npm ci
npm run electron:dev
```

## Scripts

| Script | Description |
|--------|-------------|
| `npm run dev` | Start Vite dev server |
| `npm run electron:dev` | Start Electron with hot reload |
| `npm run build` | Web build (TypeScript + Vite) |
| `npm run electron:build` | Package Electron app |
| `npm run electron:build:linux` | Build Linux AppImage |
| `npm run electron:build:win` | Build Windows EXE (portable + NSIS) |
| `npm run lint` | ESLint |
| `npm run type-check` | TypeScript check |
| `npm run test` | Unit tests (Vitest) |

## Build Output

- **Linux**: `client/.electron-app/dist/*.AppImage`
- **Windows**: `client/.electron-app/dist/*.exe`

## Recommended IDE Setup

- [VS Code](https://code.visualstudio.com/) + [ESLint](https://marketplace.visualstudio.com/items?itemName=dbaeumer.vscode-eslint)
