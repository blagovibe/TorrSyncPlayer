const { TorrentBridge } = require("./torrent-bridge.cjs");
const { isValidMagnetLink, MAX_MAGNET_LINK_LENGTH, MAX_TORRENT_FILE_BYTES } = require("./torrent-constants.cjs");
const { electronLogger } = require("./electron-logger.cjs");

function getAllowedOrigins(staticServerInstance, devServerUrl) {
  const origins = [];
  if (staticServerInstance) {
    origins.push(new URL(staticServerInstance.url));
  }
  try { origins.push(new URL(devServerUrl)); } catch { /* ignore invalid URL */ }
  return origins;
}

function validateIpcSender(event, staticServerInstance, devServerUrl, getMainWindowWebContentsId) {
  if (event.sender.isDestroyed()) {
    electronLogger.warn("IPC validation failed: sender webContents has been destroyed");
    return false;
  }
  const mainWindowId = getMainWindowWebContentsId();
  if (mainWindowId !== null && event.sender.id !== mainWindowId) {
    electronLogger.warn(`IPC validation failed: sender webContents ID ${event.sender.id} does not match main window ID ${mainWindowId}`);
    return false;
  }
  const frameUrl = event.senderFrame?.url;
  if (!frameUrl) {
    electronLogger.warn("IPC validation failed: no frame URL");
    return false;
  }
  if (frameUrl.startsWith("file:")) {
    electronLogger.warn(`IPC validation failed: file:// protocol is not allowed`);
    return false;
  }
  try {
    const parsed = new URL(frameUrl);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      electronLogger.warn(`IPC validation failed: invalid protocol '${parsed.protocol}' from ${frameUrl}`);
      return false;
    }
    if (parsed.hostname !== "127.0.0.1" && parsed.hostname !== "localhost") {
      electronLogger.warn(`IPC validation failed: invalid hostname '${parsed.hostname}' from ${frameUrl}`);
      return false;
    }
    if (parsed.username || parsed.password) {
      electronLogger.warn(`IPC validation failed: credentials in URL from ${frameUrl}`);
      return false;
    }
    const allowedOrigins = getAllowedOrigins(staticServerInstance, devServerUrl);
    const normalizeHost = (h) => (h === "localhost" ? "127.0.0.1" : h);
    const matched = allowedOrigins.some(
      (origin) => origin.protocol === parsed.protocol && normalizeHost(origin.hostname) === normalizeHost(parsed.hostname) && origin.port === parsed.port
    );
    if (!matched) {
      electronLogger.warn(`IPC validation failed: origin mismatch. Frame: ${frameUrl}, allowed: ${allowedOrigins.map(o => o.origin).join(", ")}`);
      return false;
    }
    return true;
  } catch {
      electronLogger.warn(`IPC validation failed: URL parse error for '${frameUrl}'`);
    return false;
  }
}

function registerIpcHandlers(ipcMain, torrentBridge, deps) {
  const { getStaticServerInstance, getMainWindowWebContentsId, devServerUrl } = deps;

  const validate = (event) => validateIpcSender(event, getStaticServerInstance(), devServerUrl, getMainWindowWebContentsId);

  ipcMain.handle("torrent:addMagnet", async (event, magnetLink) => {
    if (!validate(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    if (typeof magnetLink !== "string" || magnetLink.length > MAX_MAGNET_LINK_LENGTH || !isValidMagnetLink(magnetLink)) {
      throw new Error("Invalid magnet link");
    }
    return torrentBridge.addMagnet(magnetLink);
  });

  ipcMain.handle("torrent:addTorrentFile", async (event, torrentFile) => {
    if (!validate(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    if (!(torrentFile instanceof Uint8Array) && !Array.isArray(torrentFile)) {
      throw new Error("Invalid torrent file");
    }
    const bytes = torrentFile instanceof Uint8Array ? torrentFile : Uint8Array.from(torrentFile);
    const byteLength = bytes.byteLength;
    if (byteLength > MAX_TORRENT_FILE_BYTES) {
      throw new Error(`Torrent file too large (${(byteLength / 1024 / 1024).toFixed(1)} MB). Maximum size is ${MAX_TORRENT_FILE_BYTES / 1024 / 1024} MB.`);
    }
    // Validate torrent file magic bytes: bencoded dict starting with 'd'
    if (byteLength < 10 || bytes[0] !== 0x64) {
      throw new Error("Invalid torrent file: not a valid bencoded torrent");
    }
    const headerStr = new TextDecoder().decode(bytes.subarray(0, Math.min(256, byteLength)));
    const headerLower = headerStr.toLowerCase();
    if (!headerLower.includes("announce") && !headerLower.includes("info")) {
      throw new Error("Invalid torrent file: missing required torrent fields (announce or info)");
    }
    if (!headerLower.includes("info")) {
      throw new Error("Invalid torrent file: missing required 'info' dictionary");
    }
    // Decode bencoded announce-list to validate tracker URLs
    try {
      const blockedHosts = new Set(["localhost", "127.0.0.1", "0.0.0.0", "::1"]);
      const decoder = new TextDecoder();
      const fullStr = decoder.decode(bytes);
      // Find all tracker URLs safely by scanning for 'announce' followed by 'http'
      const announceLen = 8; // "announce".length
      let searchFrom = 0;
      while (searchFrom < fullStr.length) {
        const idx = fullStr.indexOf("announce", searchFrom);
        if (idx === -1) break;
        const afterKey = idx + announceLen;
        let pos = afterKey;
        // Skip optional bencode digits/hyphen for key variants like "announce-list"
        while (pos < fullStr.length && (fullStr[pos] >= "0" && fullStr[pos] <= "9" || fullStr[pos] === "-")) {
          pos++;
        }
        // Look for "http" at this position, bounded by bencode string length prefix
        if (pos + 4 <= fullStr.length && fullStr.substring(pos, pos + 4) === "http") {
          const urlHttpStart = pos;
          // Extract URL up to 2048 chars or first control character
          let urlEnd = urlHttpStart + 4;
          const maxUrlLen = Math.min(fullStr.length, urlHttpStart + 2048);
          while (urlEnd < maxUrlLen) {
            const ch = fullStr.charCodeAt(urlEnd);
            if (ch < 0x20 || ch === 0x7f) break;
            urlEnd++;
          }
          const url = fullStr.substring(urlHttpStart, urlEnd);
          try {
            const parsed = new URL(url);
            const hostname = parsed.hostname.toLowerCase();
            if (blockedHosts.has(hostname) || hostname.startsWith("10.") || hostname.startsWith("192.168.")) {
              throw new Error(`Invalid torrent file: tracker URL points to internal/private address: ${hostname}`);
            }
            if (hostname.startsWith("172.")) {
              const second = parseInt(hostname.split(".")[1], 10);
              if (second >= 16 && second <= 31) {
                throw new Error(`Invalid torrent file: tracker URL points to internal/private address: ${hostname}`);
              }
            }
          } catch (urlError) {
            if (urlError.message.startsWith("Invalid torrent file:")) throw urlError;
          }
        }
        searchFrom = afterKey;
      }
    } catch (decodeError) {
      if (decodeError.message.startsWith("Invalid torrent file:")) throw decodeError;
      // Bencoded parsing failure — file structure is invalid
      throw new Error("Invalid torrent file: unable to parse bencoded tracker data");
    }
    return torrentBridge.addTorrentFile(torrentFile);
  });

  ipcMain.handle("torrent:getStats", async (event) => {
    if (!validate(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    return torrentBridge.getStats();
  });

  ipcMain.handle("torrent:clear", async (event) => {
    if (!validate(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    return torrentBridge.clear();
  });

  ipcMain.handle("torrent:setMaxBufferMB", async (event, mb) => {
    if (!validate(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    if (typeof mb !== "number" || !Number.isFinite(mb) || mb <= 0) {
      throw new Error("Invalid buffer size");
    }
    torrentBridge.setMaxBufferMB(mb);
  });

  ipcMain.handle("torrent:probeAudioTracks", async (event, streamUrl) => {
    if (!validate(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    if (typeof streamUrl !== "string" || streamUrl.length > 5000) {
      throw new Error("Invalid stream URL");
    }
    return torrentBridge.probeAudioTracks(streamUrl);
  });

  ipcMain.handle("torrent:createAudioTrackStreamUrl", async (event, params) => {
    if (!validate(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    if (!params || typeof params !== "object") {
      throw new Error("Invalid audio track params");
    }
    return torrentBridge.createAudioTrackStreamUrl(params);
  });

  ipcMain.handle("torrent:createMultiplexedStreamUrl", async (event, params) => {
    if (!validate(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    if (!params || typeof params !== "object") {
      throw new Error("Invalid mux params");
    }
    return torrentBridge.createMultiplexedStreamUrl(params);
  });

  ipcMain.handle("torrent:createSubtitleStreamUrl", async (event, params) => {
    if (!validate(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    if (!params || typeof params !== "object") {
      throw new Error("Invalid subtitle params");
    }
    return torrentBridge.createSubtitleStreamUrl(params);
  });

  ipcMain.handle("ffmpeg:isAvailable", async (event) => {
    if (!validate(event)) {
      throw new Error("Unauthorized IPC caller");
    }
    return TorrentBridge.checkFfmpegAvailable();
  });

  ipcMain.on("window-close-confirmed", (event) => {
    if (!validate(event)) return;
    const { app } = require("electron");
    app.exit(0);
  });

  ipcMain.on("window-close-cancelled", (event) => {
    if (!validate(event)) return;
  });
}

module.exports = { registerIpcHandlers, validateIpcSender };
