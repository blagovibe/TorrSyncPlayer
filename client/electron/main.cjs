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

function startStaticServer() {
  if (staticServerInstance) {
    return Promise.resolve(staticServerInstance);
  }

  if (staticServerPromise) {
    return staticServerPromise;
  }

  staticServerPromise = new Promise((resolve, reject) => {
    const server = http.createServer(async (request, response) => {
      try {
        if (!request.url) {
          response.statusCode = 400;
          response.end();
          return;
        }

        if (request.method !== "GET" && request.method !== "HEAD") {
          response.statusCode = 405;
          response.setHeader("Allow", "GET, HEAD");
          response.end();
          return;
        }

        const { pathname } = new URL(request.url, "http://127.0.0.1");
        const assetPath = await resolveStaticAsset(pathname);

        if (!assetPath) {
          response.statusCode = 404;
          response.end();
          return;
        }

        const body = await fs.readFile(assetPath);
        response.statusCode = 200;
        response.setHeader("Content-Type", getContentType(assetPath));
        response.setHeader("Cache-Control", "no-store");
        response.setHeader("X-Content-Type-Options", "nosniff");
        response.setHeader("X-Frame-Options", "DENY");
        response.setHeader("Referrer-Policy", "no-referrer");
        response.end(request.method === "HEAD" ? undefined : body);
      } catch (error) {
        response.statusCode = 500;
        response.end("Internal Server Error");
      }
    });

    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("Unable to start local static server"));
        return;
      }

      resolve({ server, url: `http://127.0.0.1:${address.port}` });
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

  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    void shell.openExternal(url);
    return { action: "deny" };
  });

  if (isDev) {
    void mainWindow.loadURL(loadUrl);
    mainWindow.webContents.openDevTools({ mode: "detach" });
  } else {
    void mainWindow.loadURL(loadUrl);
  }
}

app.commandLine.appendSwitch("enable-features", "WebRTC-H264WithOpenH264FFmpeg");

ipcMain.handle("torrent:addMagnet", async (_event, magnetLink) => {
  if (typeof magnetLink !== "string" || magnetLink.length > 10000) {
    throw new Error("Invalid magnet link");
  }
  return torrentBridge.addMagnet(magnetLink);
});
ipcMain.handle("torrent:addTorrentFile", async (_event, torrentFile) => {
  if (!(torrentFile instanceof Uint8Array) && !Array.isArray(torrentFile)) {
    throw new Error("Invalid torrent file");
  }
  return torrentBridge.addTorrentFile(torrentFile);
});
ipcMain.handle("torrent:getStats", async () => torrentBridge.getStats());
ipcMain.handle("torrent:clear", async () => torrentBridge.clear());
ipcMain.handle("torrent:probeAudioTracks", async (_event, streamUrl) => {
  if (typeof streamUrl !== "string" || streamUrl.length > 5000) {
    throw new Error("Invalid stream URL");
  }
  return torrentBridge.probeAudioTracks(streamUrl);
});
ipcMain.handle(
  "torrent:createAudioTrackStreamUrl",
  async (_event, params) => {
    if (!params || typeof params !== "object") {
      throw new Error("Invalid audio track params");
    }
    return torrentBridge.createAudioTrackStreamUrl(params);
  },
);

app.whenReady().then(async () => {
  let loadUrl = devServerUrl;

  if (!isDev) {
    try {
      const staticApp = await startStaticServer();
      loadUrl = `${staticApp.url}/index.html`;
    } catch (error) {
      console.error("Failed to start local static server, falling back to file URL:", error);
      loadUrl = pathToFileURL(path.join(distDir, "index.html")).href;
    }
  }

  createWindow(loadUrl);

  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow(loadUrl);
    }
  });
});

app.on("before-quit", (event) => {
  if (BrowserWindow.getAllWindows().length > 0) {
    event.preventDefault();
    (async () => {
      try {
        await torrentBridge.destroy();
      } catch {
        // Ignore cleanup errors during shutdown
      }
      if (staticServerInstance) {
        try {
          staticServerInstance.server.close();
        } catch {
          // Ignore
        }
        staticServerInstance = null;
      }
      // Close all windows without triggering before-quit again
      for (const win of BrowserWindow.getAllWindows()) {
        win.destroy();
      }
      // Use exit() instead of quit() to avoid re-triggering before-quit
      app.exit(0);
    })();
  }
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});
