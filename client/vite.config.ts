import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";
import path from "node:path";
import fs from "node:fs";

const bittorrentProtocolMseShimId = '\x00bittorrent-protocol/mse-shim'

const bittorrentProtocolMseShim = () => ({
  name: 'bittorrent-protocol-mse-shim',
  enforce: 'pre' as const,
  resolveId(source: string) {
    if (source === './mse.js' || source.endsWith('/bittorrent-protocol/mse.js')) {
      return bittorrentProtocolMseShimId
    }
    return null
  },
  load(id: string) {
    if (id === bittorrentProtocolMseShimId) {
      return "export const nativeRC4 = false; export class MessageStreamEncryptor { constructor() {} encrypt(d){return d} decrypt(d){return d} }"
    }
    return null
  }
})

// https://vite.dev/config/
export default defineConfig(async () => ({
  plugins: [react(), bittorrentProtocolMseShim()],
  base: "./",
  root: ".",
  build: {
    outDir: "dist",
    rollupOptions: {
      output: {
        manualChunks: {
          react: ["react", "react-dom"],
          webtorrent: ["webtorrent"],
          peerjs: ["peerjs"],
        },
      },
    },
  },
  resolve: {
    alias: {
      events: "events/",
      path: "path-browserify",
      crypto: fileURLToPath(new URL("./src/shims/crypto-shim.ts", import.meta.url)),
      randomfill: fileURLToPath(new URL("./src/shims/randomfill.ts", import.meta.url)),
      stream: "stream-browserify",
      buffer: "buffer/",
      process: "process/browser",
      util: "util/",
      "bittorrent-dht": fileURLToPath(new URL("./src/shims/bittorrent-dht.ts", import.meta.url)),
      ip: "ip",
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
  test: {
    projects: [
      {
        test: {
          name: "client",
          include: ["src/**/*.{test,spec}.ts?(x)"],
          environment: "jsdom",
          setupFiles: ["src/test-setup.ts"],
        },
      },
      {
        test: {
          name: "electron",
          include: ["electron/**/*.test.cjs"],
          environment: "node",
          globals: true,
        },
      },
    ],
  },
}));
