# TorrSyncPlayer Client

This is the active TorrSyncPlayer application: React, TypeScript, Vite, Tauri, PeerJS, and WebTorrent.

From the repository root, `npm run <script>` delegates to this package. CI also runs from this directory and uses `client/package-lock.json`.

## Recommended IDE Setup

- [VS Code](https://code.visualstudio.com/) + [Tauri](https://marketplace.visualstudio.com/items?itemName=tauri-apps.tauri-vscode) + [rust-analyzer](https://marketplace.visualstudio.com/items?itemName=rust-lang.rust-analyzer)

## Build Windows EXE (portable)

1. Build from a Windows environment with the Tauri prerequisites installed, or use CI on `windows-latest`.
2. From project root run:
   - `npm run tauri:build:win`
3. Output portable EXE:
   - `client/src-tauri/target/release/client.exe`
