const { app, BrowserWindow, ipcMain, shell } = require("electron");
const http = require("node:http");
const fs = require("node:fs/promises");
const path = require("node:path");
const { pathToFileURL } = require("node:url");
const { TorrentBridge } = require("./torrent-bridge.cjs");

const isDev = !app.isPackaged;
const devServerUrl = process.env.VITE_DEV_SERVER_URL || "http://127.0.0.1:1420";
const appRoot = path.resolve(__dirname, "..");
const distDir = path.join(appRoot, "dist");
const torrentBridge = new TorrentBridge();

let staticServerPromise = null;
let staticServerInstance = null;

const MIME_TYPES = new Map([
  [".html", "text/html; charset=utf-8"],
  [".js", "text/javascript; charset=utf-8"],
  [".css", "text/css; charset=utf-8"],
  [".json", "application/json; charset=utf-8"],
  [".svg", "image/svg+xml"],
  [".png", "image/png"],
  [".jpg", "image/jpeg"],
  [".jpeg", "image/jpeg"],
  [".gif", "image/gif"],
  [".ico", "image/x-icon"],
  [".webp", "image/webp"],
  [".map", "application/json; charset=utf-8"],
  [".txt", "text/plain; charset=utf-8"],
]);

function getContentType(filePath) {
  return MIME_TYPES.get(path.extname(filePath).toLowerCase()) || "application/octet-stream";
}

function isPathInsideDirectory(targetPath, directoryPath) {
  const relativePath = path.relative(directoryPath, targetPath);
  return relativePath !== "" && !relativePath.startsWith("..") && !path.isAbsolute(relativePath);
}

async function fileExists(filePath) {
  try {
    const stat = await fs.stat(filePath);
    return stat.isFile();
  } catch {
    return false;
  }
}

async function resolveStaticAsset(requestPath) {
  const decodedPath = decodeURIComponent(requestPath);
  const sanitizedPath = decodedPath === "/" ? "/index.html" : decodedPath;
  const absolutePath = path.normalize(path.join(distDir, sanitizedPath));

  if (!isPathInsideDirectory(absolutePath, distDir)) {
    return path.join(distDir, "index.html");
  }

  if (await fileExists(absolutePath)) {
    return absolutePath;
  }

  if (!path.extname(sanitizedPath)) {
    return path.join(distDir, "index.html");
  }

  return null;
}

function buildCspHeader(streamBaseUrl) {
  const mediaSource = streamBaseUrl || "'self'";
  return `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob: ${mediaSource}; connect-src 'self' wss://0.peerjs.com wss://*.openwebtorrent.com wss://*.webtorrent.dev wss://*.btorrent.xyz ${mediaSource}; font-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'`;
}

function getAllowedServerOrigins() {
  const origins = new Set<string>();
  if (staticServerInstance) {
    origins.add(staticServerInstance.url);
  }
  try { origins.add(new URL(devServerUrl).origin); } catch { /* ignore */ }
  return origins;
}

function validateServerOrigin(request) {
  const origin = request.headers.origin ?? request.headers.referer;
  if (!origin) {
    if (isDev) return true;
    return false;
  }
  try {
    const parsed = new URL(origin);
    const allowed = getAllowedServerOrigins();
    if (allowed.size === 0) return true;
    for (const a of allowed) {
      if (new URL(a).origin === parsed.origin) return true;
    }
    return false;
  } catch {
    return false;
  }
}

function startStaticServer() {
  if (staticServerInstance) {
    return Promise.resolve(staticServerInstance);
  }

  if (staticServerPromise) {
    return staticServerPromise;
  }

  staticServerPromise = new Promise((resolve, reject) => {
    let streamBaseUrl = null;
    const server = http.createServer(async (request, response) => {
      try {
        if (!request.url) {
          response.statusCode = 400;
          response.setHeader("X-Content-Type-Options", "nosniff");
          response.setHeader("X-Frame-Options", "DENY");
          response.setHeader("Content-Security-Policy", buildCspHeader(streamBaseUrl));
          response.end();
          return;
        }

        if (!validateServerOrigin(request)) {
          response.statusCode = 403;
          response.setHeader("X-Content-Type-Options", "nosniff");
          response.end("Forbidden");
          return;
        }

        const { pathname } = new URL(request.url, "http://127.0.0.1");

        if (pathname.startsWith("/mux/") || pathname.startsWith("/audio/") || pathname.startsWith("/subtitle/")) {
          torrentBridge.handleAudioRequest(request, response).catch((error) => {
            console.error("[TorrSyncPlayer] Audio/mux request failed:", error);
            if (!response.headersSent) {
              response.statusCode = 500;
              response.setHeader("X-Content-Type-Options", "nosniff");
              response.setHeader("X-Frame-Options", "DENY");
              response.setHeader("X-XSS-Protection", "1; mode=block");
              response.setHeader("Referrer-Policy", "no-referrer");
              response.setHeader("Content-Security-Policy", buildCspHeader(streamBaseUrl));
              response.end("Stream request failed");
            } else {
              response.destroy();
            }
          });
          return;
        }

        if (request.method !== "GET" && request.method !== "HEAD") {
          response.statusCode = 405;
          response.setHeader("Allow", "GET, HEAD");
          response.setHeader("X-Content-Type-Options", "nosniff");
          response.setHeader("X-Frame-Options", "DENY");
          response.setHeader("Content-Security-Policy", buildCspHeader(streamBaseUrl));
          response.end();
          return;
        }

        const assetPath = await resolveStaticAsset(pathname);

        if (!assetPath) {
          response.statusCode = 404;
          response.setHeader("X-Content-Type-Options", "nosniff");
          response.setHeader("X-Frame-Options", "DENY");
          response.setHeader("Content-Security-Policy", buildCspHeader(streamBaseUrl));
          response.end();
          return;
        }

        const body = await fs.readFile(assetPath);
        response.statusCode = 200;
        response.setHeader("Content-Type", getContentType(assetPath));
        response.setHeader("Cache-Control", "no-store");
        response.setHeader("X-Content-Type-Options", "nosniff");
        response.setHeader("X-Frame-Options", "DENY");
        response.setHeader("X-XSS-Protection", "1; mode=block");
        response.setHeader("Referrer-Policy", "no-referrer");
        response.setHeader("Content-Security-Policy", buildCspHeader(streamBaseUrl));
        response.end(request.method === "HEAD" ? undefined : body);
      } catch (error) {
        if (!response.headersSent) {
          response.statusCode = 500;
          response.setHeader("X-Content-Type-Options", "nosniff");
          response.setHeader("X-Frame-Options", "DENY");
          response.setHeader("X-XSS-Protection", "1; mode=block");
          response.setHeader("Referrer-Policy", "no-referrer");
          response.setHeader("Content-Security-Policy", buildCspHeader(streamBaseUrl));
        }
        if (!response.headersSent) {
          response.end("Internal Server Error");
        } else {
          response.destroy();
        }
      }
    });

    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("Unable to start local static server"));
        return;
      }

      streamBaseUrl = `http://127.0.0.1:${address.port}`;
      torrentBridge.setStreamBaseUrl(streamBaseUrl);

      resolve({ server, url: streamBaseUrl });
    });

    server.on("error", reject);
  }).then((result) => {
    staticServerInstance = result;
    staticServerPromise = null;
    return result;
  }).catch((error) => {
    staticServerPromise = null;
    throw error;
  });

  return staticServerPromise;
}

let mainWindowWebContentsId = null;

function createWindow(loadUrl) {
  const mainWindow = new BrowserWindow({
    width: 1000,
    height: 700,
    minWidth: 760,
    minHeight: 520,
    title: "TorrSyncPlayer",
    backgroundColor: "#0f172a",
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webSecurity: true,
      preload: path.join(__dirname, "preload.cjs"),
    },
  });
  mainWindowWebContentsId = mainWindow.webContents.id;

  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url);
    return { action: "deny" };
  });

  mainWindow.on("close", (event) => {
    if (mainWindow.webContents.isLoading()) return;
    event.preventDefault();
    mainWindow.webContents.send("window-close-request");
  });

  if (isDev) {
    void mainWindow.loadURL(loadUrl);
    mainWindow.webContents.openDevTools({ mode: "detach" });
  } else {
    void mainWindow.loadURL(loadUrl);
  }
}

app.commandLine.appendSwitch("enable-features", "WebRTC-H264WithOpenH264FFmpeg");

function getAllowedOrigins() {
  const origins = [];
  if (staticServerInstance) {
    origins.push(new URL(staticServerInstance.url));
  }
  try { origins.push(new URL(devServerUrl)); } catch { /* ignore invalid URL */ }
  return origins;
}

function validateIpcSender(event) {
  const frameUrl = event.senderFrame?.url;
  if (!frameUrl) {
    console.warn("[TorrSyncPlayer] IPC validation failed: no frame URL");
    return false;
  }
  try {
    const parsed = new URL(frameUrl);
    if (parsed.protocol === "file:") {
      console.warn(`[TorrSyncPlayer] IPC validation failed: file:// protocol is not allowed`);
      return false;
    }
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      console.warn(`[TorrSyncPlayer] IPC validation failed: invalid protocol '${parsed.protocol}' from ${frameUrl}`);
      return false;
    }
    if (parsed.hostname !== "127.0.0.1" && parsed.hostname !== "localhost") {
      console.warn(`[TorrSyncPlayer] IPC validation failed: invalid hostname '${parsed.hostname}' from ${frameUrl}`);
      return false;
    }
    const allowedOrigins = getAllowedOrigins();
    const matched = allowedOrigins.some(
      (origin) => origin.protocol === parsed.protocol && origin.hostname === parsed.hostname && origin.port === parsed.port
    );
    if (!matched) {
      console.warn(`[TorrSyncPlayer] IPC validation failed: origin mismatch. Frame: ${frameUrl}, allowed: ${allowedOrigins.map(o => o.origin).join(", ")}`);
      return false;
    }
    return true;
  } catch {
    console.warn(`[TorrSyncPlayer] IPC validation failed: URL parse error for '${frameUrl}'`);
    return false;
  }
}

ipcMain.handle("torrent:addMagnet", async (event, magnetLink) => {
  if (!validateIpcSender(event)) {
    throw new Error("Unauthorized IPC caller");
  }
  if (typeof magnetLink !== "string" || magnetLink.length > 7000) {
    throw new Error("Invalid magnet link");
  }
  return torrentBridge.addMagnet(magnetLink);
});
// Keep in sync with client/src/config.ts IPC_MAX_TORRENT_BYTES
const MAX_TORRENT_FILE_BYTES = 10 * 1024 * 1024;
ipcMain.handle("torrent:addTorrentFile", async (event, torrentFile) => {
  if (!validateIpcSender(event)) {
    throw new Error("Unauthorized IPC caller");
  }
  if (!(torrentFile instanceof Uint8Array) && !Array.isArray(torrentFile)) {
    throw new Error("Invalid torrent file");
  }
  const byteLength = torrentFile instanceof Uint8Array ? torrentFile.byteLength : torrentFile.length;
  if (byteLength > MAX_TORRENT_FILE_BYTES) {
    throw new Error(`Torrent file too large (${(byteLength / 1024 / 1024).toFixed(1)} MB). Maximum size is ${MAX_TORRENT_FILE_BYTES / 1024 / 1024} MB.`);
  }
  return torrentBridge.addTorrentFile(torrentFile);
});
ipcMain.handle("torrent:getStats", async (event) => {
  if (!validateIpcSender(event)) {
    throw new Error("Unauthorized IPC caller");
  }
  return torrentBridge.getStats();
});
ipcMain.handle("torrent:clear", async (event) => {
  if (!validateIpcSender(event)) {
    throw new Error("Unauthorized IPC caller");
  }
  return torrentBridge.clear();
});
ipcMain.handle("torrent:setMaxBufferMB", async (event, mb) => {
  if (!validateIpcSender(event)) {
    throw new Error("Unauthorized IPC caller");
  }
  if (typeof mb !== "number" || mb <= 0) {
    throw new Error("Invalid buffer size");
  }
  torrentBridge.setMaxBufferMB(mb);
});
ipcMain.handle("torrent:probeAudioTracks", async (event, streamUrl) => {
  if (!validateIpcSender(event)) {
    throw new Error("Unauthorized IPC caller");
  }
  if (typeof streamUrl !== "string" || streamUrl.length > 5000) {
    throw new Error("Invalid stream URL");
  }
  return torrentBridge.probeAudioTracks(streamUrl);
});
ipcMain.handle(
  "torrent:createAudioTrackStreamUrl",
  async (event, params) => {
    if (!validateIpcSender(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    if (!params || typeof params !== "object") {
      throw new Error("Invalid audio track params");
    }
    return torrentBridge.createAudioTrackStreamUrl(params);
  },
);
ipcMain.handle(
  "torrent:createMultiplexedStreamUrl",
  async (event, params) => {
    if (!validateIpcSender(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    if (!params || typeof params !== "object") {
      throw new Error("Invalid mux params");
    }
    return torrentBridge.createMultiplexedStreamUrl(params);
  },
);
ipcMain.handle(
  "torrent:createSubtitleStreamUrl",
  async (event, params) => {
    if (!validateIpcSender(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    if (!params || typeof params !== "object") {
      throw new Error("Invalid subtitle params");
    }
    return torrentBridge.createSubtitleStreamUrl(params);
  },
);

ipcMain.on("window-close-confirmed", (event) => {
  if (!validateIpcSender(event)) return;
  if (mainWindowWebContentsId !== null && event.sender.id !== mainWindowWebContentsId) {
    console.warn("[TorrSyncPlayer] window-close-confirmed rejected: sender is not the main window");
    return;
  }
  for (const win of BrowserWindow.getAllWindows()) {
    win.destroy();
  }
  app.exit(0);
});

ipcMain.on("window-close-cancelled", (event) => {
  if (!validateIpcSender(event)) return;
  if (mainWindowWebContentsId !== null && event.sender.id !== mainWindowWebContentsId) {
    console.warn("[TorrSyncPlayer] window-close-cancelled rejected: sender is not the main window");
    return;
  }
});

app.whenReady().then(async () => {
  // Check ffmpeg availability
  try {
    const ffmpegOk = await TorrentBridge.checkFfmpegAvailable();
    if (!ffmpegOk) {
      console.warn("[TorrSyncPlayer] ffmpeg not found — audio track features will be unavailable");
    } else {
      console.log("[TorrSyncPlayer] ffmpeg detected");
    }
  } catch (error) {
    console.warn("[TorrSyncPlayer] ffmpeg check failed:", error);
  }

  let loadUrl = devServerUrl;

  if (!isDev) {
     try {
       const staticApp = await startStaticServer();
       loadUrl = `${staticApp.url}/index.html`;
     } catch (error) {
       console.error("Failed to start local static server:", error);
       app.quit();
       return;
     }
   }

  createWindow(loadUrl);

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow(loadUrl);
    }
  });
});

const ELECTRON_FORCE_EXIT_TIMEOUT_MS = 5000;

let isQuitting = false;

app.on("before-quit", (event) => {
    if (isQuitting) return;
    const windows = BrowserWindow.getAllWindows();
    if (windows.length === 0) {
      return;
    }
    if (torrentBridge.isDestroyed?.()) {
      for (const win of windows) {
        win.destroy();
      }
      return;
    }
    isQuitting = true;
    event.preventDefault();
    let hasExited = false;
    const forceExit = () => {
      if (hasExited) return;
      hasExited = true;
      const allWindows = BrowserWindow.getAllWindows();
      for (const win of allWindows) {
        try { win.destroy(); } catch { /* already destroyed */ }
      }
      app.exit(0);
    };
    const safetyTimeout = setTimeout(forceExit, ELECTRON_FORCE_EXIT_TIMEOUT_MS);
      (async () => {
        try {
          await torrentBridge.destroy();
        } catch (err) {
          console.error("[TorrSyncPlayer] Cleanup failed:", err);
        }
       if (staticServerInstance) {
         try {
           staticServerInstance.server.close();
         } catch {
           // Ignore
         }
         staticServerInstance = null;
       }
       clearTimeout(safetyTimeout);
       if (!hasExited) {
         forceExit();
       }
     })();
  });

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    if (isQuitting) return;
    app.quit();
  }
});
