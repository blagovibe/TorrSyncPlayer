# TorrSyncPlayer

The active application lives in `client/`. The repository root is a command shim so local commands and CI entry points do not accidentally build the older root Vite/Tauri scaffold.

## Local Commands

Run these from the repository root:

```sh
npm run dev
npm run lint
npm run type-check
npm run test
npm run build
```

The root scripts delegate to the matching `client/` scripts. Install dependencies with `npm ci` inside `client/`; CI also uses `client/package-lock.json`.

## Desktop Bundles

Tauri bundle commands are also delegated to `client/`:

```sh
npm run tauri:build:linux
npm run tauri:build:win
```

Rust and the Tauri platform dependencies are required for these bundle commands.

## Signaling Status

The current client runtime uses PeerJS signaling through `client/src/services/P2PService.ts`. There is no active in-repository signaling server required for local client build, test, or dev.

The old Node WebSocket server is not the active path and should not be restored with committed `node_modules`. If self-hosted signaling is added later, implement it as a Go module under `server/` and update the client wiring and tests at the same time.
