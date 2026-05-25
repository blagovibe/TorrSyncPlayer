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
      ip: fileURLToPath(new URL("./src/shims/ip-patch/lib/ip.js", import.meta.url)),
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
