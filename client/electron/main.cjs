/**
 * @fileoverview Electron main process entry point.
 * Manages window lifecycle, IPC handlers, and torrent bridge.
 */

const { app, BrowserWindow, shell } = require("electron");
const path = require("node:path");
const { TorrentBridge } = require("./torrent-bridge.cjs");
const { StaticServer } = require("./static-server.cjs");
const { registerIpcHandlers } = require("./ipc-handlers.cjs");
const { electronLogger } = require("./electron-logger.cjs");

// Diagnostic logging at startup
electronLogger.info("=== TorrSyncPlayer Starting ===");
electronLogger.info(`Electron version: ${process.versions.electron}`);
electronLogger.info(`Node version: ${process.versions.node}`);
electronLogger.info(`Chrome version: ${process.versions.chrome}`);
electronLogger.info(`Platform: ${process.platform} ${process.arch}`);
electronLogger.info(`App path: ${app.getAppPath()}`);
electronLogger.info(`isPackaged: ${app.isPackaged}`);

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
  electronLogger.info(`Creating window with URL: ${loadUrl}`);
  
  let mainWindow;
  try {
    mainWindow = new BrowserWindow({
      width: 1000,
      height: 700,
      minWidth: 760,
      minHeight: 520,
      title: "TorrSyncPlayer",
      backgroundColor: "#0f172a",
      show: false, // Don't show until content is loaded
      webPreferences: {
        contextIsolation: true,
        nodeIntegration: false,
        sandbox: true,
        webSecurity: true,
        preload: path.join(__dirname, "preload.cjs"),
      },
    });
    electronLogger.info("BrowserWindow created successfully");
  } catch (error) {
    electronLogger.error("Failed to create BrowserWindow:", error);
    throw error;
  }
  
  mainWindowWebContentsId = mainWindow.webContents.id;
  electronLogger.info(`Main window webContents ID: ${mainWindowWebContentsId}`);

  // Show window when content is ready
  mainWindow.once("ready-to-show", () => {
    electronLogger.info("Window ready to show");
    mainWindow.show();
    mainWindow.focus();
  });

  // Log did-finish-load
  mainWindow.webContents.on("did-finish-load", () => {
    electronLogger.info("Page finished loading");
  });

  // Log did-fail-load
  mainWindow.webContents.on("did-fail-load", (event, errorCode, errorDescription, validatedURL) => {
    electronLogger.error(`Page failed to load: ${errorCode} - ${errorDescription} (${validatedURL})`);
  });

  // Log console messages from renderer
  mainWindow.webContents.on("console-message", (event, level, message, line, sourceId) => {
    const levels = ["verbose", "info", "warning", "error"];
    electronLogger.info(`[Renderer Console ${levels[level] || level}] ${message} (${sourceId}:${line})`);
  });

  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    try {
      const parsed = new URL(url);
      if (parsed.protocol === "https:" || parsed.protocol === "http:") {
        const allowedHostPatterns = [
          /^(\.)?peerjs\.com$/,
          /^tracker\.(btorrent\.xyz|openwebtorrent\.com|webtorrent\.dev)$/,
          /^(\.)?github\.com$/,
          /^(\.)?kilo\.ai$/,
        ];
        const hostname = parsed.hostname.toLowerCase();
        const isAllowed = allowedHostPatterns.some((pattern) => pattern.test(hostname));
        if (isAllowed && !parsed.username && !parsed.password) {
          void shell.openExternal(url);
        } else {
          electronLogger.warn(`Blocked shell.openExternal with untrusted host: ${hostname}`);
        }
      } else {
        electronLogger.warn(`Blocked shell.openExternal with unsafe protocol: ${parsed.protocol}`);
      }
    } catch {
      electronLogger.warn(`Blocked shell.openExternal with invalid URL: ${url}`);
    }
    return { action: "deny" };
  });

  mainWindow.on("close", (event) => {
    if (mainWindow.webContents.isLoading()) return;
    event.preventDefault();
    mainWindow.webContents.send("window-close-request");
  });

  mainWindow.on("closed", () => {
    electronLogger.info("Main window closed");
    mainWindowWebContentsId = null;
  });

  // Load URL with error handling
  const loadPromise = isDev 
    ? mainWindow.loadURL(loadUrl).then(() => {
        electronLogger.info("Dev URL loaded successfully");
        mainWindow.webContents.openDevTools({ mode: "detach" });
      })
    : mainWindow.loadURL(loadUrl).then(() => {
        electronLogger.info("Production URL loaded successfully");
      });
  
  loadPromise.catch((error) => {
    electronLogger.error(`Failed to load URL ${loadUrl}:`, error);
    // Try to show error page
    mainWindow.loadURL(`data:text/html,<html><body><h1>Failed to load application</h1><p>${error.message}</p></body></html>`).catch(() => {
      app.quit();
    });
  });
}

app.commandLine.appendSwitch("enable-features", "WebRTC-H264WithOpenH264FFmpeg");

registerIpcHandlers(require("electron").ipcMain, torrentBridge, {
  getStaticServerInstance: () => staticServer.instance,
  getMainWindowWebContentsId: () => mainWindowWebContentsId,
  devServerUrl,
});

const { buildCspHeader } = require("./static-server.cjs");

app.whenReady().then(async () => {
  electronLogger.info("App is ready");
  
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
      electronLogger.warn("ffmpeg not found — audio track features will be unavailable");
    } else {
      electronLogger.info("ffmpeg detected");
    }
  } catch (error) {
    electronLogger.warn("ffmpeg check failed:", error);
  }

  let loadUrl = devServerUrl;
  electronLogger.info(`Initial load URL: ${loadUrl}`);

  if (!isDev) {
     try {
       electronLogger.info("Starting static server...");
       const staticApp = await staticServer.start();
       loadUrl = `${staticApp.url}/index.html`;
       electronLogger.info(`Static server started at: ${staticApp.url}`);
     } catch (error) {
        electronLogger.error("Failed to start local static server:", error);
       app.quit();
       return;
     }
   }

  electronLogger.info(`Final load URL: ${loadUrl}`);
  createWindow(loadUrl);

  app.on("activate", () => {
    electronLogger.info("App activated");
    if (BrowserWindow.getAllWindows().length === 0) {
      electronLogger.info("No windows found, creating new window");
      createWindow(loadUrl);
    }
  });
});

const ELECTRON_FORCE_EXIT_TIMEOUT_MS = 5000;

let isQuitting = false;

async function gracefulCleanup() {
  try {
    await torrentBridge.destroy();
  } catch (err) {
    electronLogger.error("Torrent bridge cleanup failed:", err);
  }
  if (staticServer.instance) {
    try {
      staticServer.instance.server.close();
    } catch {
      // Ignore
    }
    staticServer.instance = null;
  }
}

app.on("before-quit", (event) => {
    if (isQuitting) return;
    isQuitting = true;
    const windows = BrowserWindow.getAllWindows();
    if (windows.length === 0) {
      return;
    }
    event.preventDefault();
    let forceExited = false;
    const forceExit = () => {
      if (forceExited) return;
      forceExited = true;
      for (const win of BrowserWindow.getAllWindows()) {
        try { win.destroy(); } catch { /* already destroyed */ }
      }
      app.exit(0);
    };
    const safetyTimeout = setTimeout(forceExit, ELECTRON_FORCE_EXIT_TIMEOUT_MS);
    gracefulCleanup().then(() => {
      clearTimeout(safetyTimeout);
      if (!forceExited) {
        forceExit();
      }
    });
  });

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    if (isQuitting) return;
    app.quit();
  }
});
