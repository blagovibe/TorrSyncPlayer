import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

// @ts-expect-error process is a nodejs global
const host = process.env.TAURI_DEV_HOST;

// https://vite.dev/config/
export default defineConfig(async () => ({
  plugins: [react()],
  root: ".",
  build: {
    outDir: "dist",
  },
  resolve: {
    alias: {
      events: "events/",
      path: "path-browserify",
      crypto: "crypto-browserify",
      stream: "stream-browserify",
      buffer: "buffer/",
      process: "process/browser",
      util: "util/",
      "bittorrent-dht": fileURLToPath(new URL("./src/shims/bittorrent-dht.ts", import.meta.url)),
    },
  },
  define: {
    global: "globalThis",
    "process.env": "{}",
    "process.browser": "true",
  },
  optimizeDeps: {
    include: ["buffer", "process", "webtorrent"],
  },

  // Vite options tailored for Tauri development and only applied in `tauri dev` or `tauri build`
  //
  // 1. prevent Vite from obscuring rust errors
  clearScreen: false,
  // 2. tauri expects a fixed port, fail if that port is not available
  server: {
    port: 1420,
    strictPort: true,
    host: host || false,
    hmr: host
      ? {
          protocol: "ws",
          host,
          port: 1421,
        }
      : undefined,
    watch: {
      // 3. tell Vite to ignore watching `src-tauri`
      ignored: ["**/src-tauri/**"],
    },
  },
}));
