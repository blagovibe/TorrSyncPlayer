import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

// https://vite.dev/config/
export default defineConfig(async () => ({
  plugins: [react()],
  base: "./",
  root: ".",
  build: {
    outDir: "dist",
  },
  resolve: {
    alias: {
      events: "events/",
      path: "path-browserify",
      crypto: "crypto-browserify",
      randomfill: fileURLToPath(new URL("./src/shims/randomfill.ts", import.meta.url)),
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

  clearScreen: false,
  server: {
    port: 1420,
    strictPort: true,
    host: false,
  },
}));
