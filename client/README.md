# Tauri + React + Typescript

This template should help get you started developing with Tauri, React and Typescript in Vite.

## Recommended IDE Setup

- [VS Code](https://code.visualstudio.com/) + [Tauri](https://marketplace.visualstudio.com/items?itemName=tauri-apps.tauri-vscode) + [rust-analyzer](https://marketplace.visualstudio.com/items?itemName=rust-lang.rust-analyzer)

## Build Windows EXE (portable)

1. Install the Windows target:
   - `rustup target add x86_64-pc-windows-gnu`
2. From project root run:
   - `npm run tauri:build:win`
3. Output EXE path:
   - `client/src-tauri/target/release/bundle/app/TorrSyncPlayer.exe`
