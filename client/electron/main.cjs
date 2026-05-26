/**
 * @fileoverview Electron main process entry point.
 * Manages window lifecycle, IPC handlers, and torrent bridge.
 */

const { app, BrowserWindow, shell } = require("electron");
const path = require("node:path");
const { TorrentBridge } = require("./torrent-bridge.cjs");
const { StaticServer } = require("./static-server.cjs");
const { registerIpcHandlers } = require("./ipc-handlers.cjs");

/** @type {boolean} */
const isDev = !app.isPackaged;
/** @type {string} */
const devServerUrl = process.env.VITE_DEV_SERVER_URL || "http://127.0.0.1:1420";
/** @type {string} */
const appRoot = path.resolve(__dirname, "..");
/** @type {string} */
const distDir = path.join(appRoot, "dist");
/** @type {TorrentBridge} */
const torrentBridge = new TorrentBridge();

/** @type {StaticServer} */
const staticServer = new StaticServer({ distDir, devServerUrl, torrentBridge });

/** @type {number|null} */
let mainWindowWebContentsId = null;

/**
 * Creates the main application window.
 * @param {string} loadUrl - The URL to load in the window.
 * @returns {void}
 */
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
    try {
      const parsed = new URL(url);
      if (parsed.protocol === "https:" || parsed.protocol === "http:") {
        void shell.openExternal(url);
      } else {
        console.warn(`[TorrSyncPlayer] Blocked shell.openExternal with unsafe protocol: ${parsed.protocol}`);
      }
    } catch {
      console.warn(`[TorrSyncPlayer] Blocked shell.openExternal with invalid URL: ${url}`);
    }
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

registerIpcHandlers(require("electron").ipcMain, torrentBridge, {
  getStaticServerInstance: () => staticServer.instance,
  getMainWindowWebContentsId: () => mainWindowWebContentsId,
  devServerUrl,
});

const { buildCspHeader } = require("./static-server.cjs");

app.whenReady().then(async () => {
  app.session.defaultSession.webRequest.onHeadersReceived((details, callback) => {
    callback({
      responseHeaders: {
        ...details.responseHeaders,
        "Content-Security-Policy": [buildCspHeader(null)],
      },
    });
  });

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
       const staticApp = await staticServer.start();
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
       if (staticServer.instance) {
         try {
           staticServer.instance.server.close();
         } catch {
           // Ignore
         }
         staticServer.instance = null;
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
