const { contextBridge, ipcRenderer } = require("electron");
const shared = require("../torrent-shared.json");
const path = require("node:path");

const MAX_TORRENT_FILE_BYTES = shared.maxTorrentFileBytes;
const MAX_TORRENT_FILE_BYTES_MB = MAX_TORRENT_FILE_BYTES / 1024 / 1024;
const MAX_MAGNET_LINK_LENGTH = shared.maxMagnetLinkLength;

const BASELINE_HRTIME = process.hrtime();

contextBridge.exposeInMainWorld("torrsyncElectronTorrent", {
  addMagnet: (magnetLink) => {
    if (typeof magnetLink !== "string" || magnetLink.length > MAX_MAGNET_LINK_LENGTH) {
      return Promise.reject(new Error("Invalid magnet link"));
    }
    return ipcRenderer.invoke("torrent:addMagnet", magnetLink);
  },
  addTorrentFile: (torrentFile) => {
    if (!(torrentFile instanceof Uint8Array) && !Array.isArray(torrentFile)) {
      return Promise.reject(new Error("Invalid torrent file"));
    }
      const byteLength = torrentFile instanceof Uint8Array ? torrentFile.byteLength : torrentFile.length;
      if (byteLength > MAX_TORRENT_FILE_BYTES) {
        return Promise.reject(new Error(`Torrent file too large (${(byteLength / 1024 / 1024).toFixed(1)} MB). Maximum size is ${MAX_TORRENT_FILE_BYTES_MB} MB.`));
      }
    return ipcRenderer.invoke("torrent:addTorrentFile", torrentFile);
  },
  getStats: () => ipcRenderer.invoke("torrent:getStats"),
  clear: () => ipcRenderer.invoke("torrent:clear"),
  setMaxBufferMB: (mb) => {
    if (typeof mb !== "number" || !Number.isFinite(mb) || mb <= 0) {
      return Promise.reject(new Error("Invalid buffer size"));
    }
    return ipcRenderer.invoke("torrent:setMaxBufferMB", mb);
  },
  probeAudioTracks: (streamUrl) => {
    if (typeof streamUrl !== "string" || streamUrl.length > 5000) {
      return Promise.reject(new Error("Invalid stream URL"));
    }
    return ipcRenderer.invoke("torrent:probeAudioTracks", streamUrl);
  },
  createAudioTrackStreamUrl: (params) => {
    if (!params || typeof params !== "object") {
      return Promise.reject(new Error("Invalid audio track params"));
    }
    return ipcRenderer.invoke("torrent:createAudioTrackStreamUrl", params);
  },
  // Multiplexed audio+video stream for perfect sync
  createMultiplexedStreamUrl: (params) => {
    if (!params || typeof params !== "object") {
      return Promise.reject(new Error("Invalid mux params"));
    }
    return ipcRenderer.invoke("torrent:createMultiplexedStreamUrl", params);
  },
  createSubtitleStreamUrl: (params) => {
    if (!params || typeof params !== "object") {
      return Promise.reject(new Error("Invalid subtitle params"));
    }
    return ipcRenderer.invoke("torrent:createSubtitleStreamUrl", params);
  },
  isFfmpegAvailable: () => ipcRenderer.invoke("ffmpeg:isAvailable"),
});

let closeRequestHandler = null;
let closeRequestWrapper = null;
contextBridge.exposeInMainWorld("torrsyncElectronWindow", {
  onCloseRequest: (callback) => {
    closeRequestHandler = callback;
    if (closeRequestWrapper) return;
    closeRequestWrapper = () => closeRequestHandler?.();
    ipcRenderer.once("window-close-request", closeRequestWrapper);
  },
  closeConfirmed: () => {
    ipcRenderer.send("window-close-confirmed");
  },
  closeCancelled: () => {
    ipcRenderer.send("window-close-cancelled");
  },
});

const encoder = new TextEncoder();
async function deriveHmacKey(sharedSecret) {
  const keyMaterial = await crypto.subtle.importKey(
    "raw",
    encoder.encode(sharedSecret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign", "verify"]
  );
  return keyMaterial;
}

async function signMessage(message, key) {
  const data = encoder.encode(JSON.stringify(message));
  const signature = await crypto.subtle.sign("HMAC", key, data);
  return Array.from(new Uint8Array(signature)).map(b => b.toString(16).padStart(2, '0')).join('');
}

async function verifyMessage(message, signature, key) {
  const data = encoder.encode(JSON.stringify(message));
  const sigBytes = new Uint8Array(signature.match(/.{2}/g).map(byte => parseInt(byte, 16)));
  return await crypto.subtle.verify("HMAC", key, sigBytes, data);
}

let hmacKey = null;
let hmacEnabled = false;

contextBridge.exposeInMainWorld("torrsyncCrypto", {
  async initHmac(sharedSecret) {
    if (!sharedSecret || typeof sharedSecret !== 'string') {
      hmacEnabled = false;
      hmacKey = null;
      return false;
    }
    try {
      hmacKey = await deriveHmacKey(sharedSecret);
      hmacEnabled = true;
      return true;
    } catch (error) {
      console.error('[TorrSync] HMAC init failed:', error);
      hmacEnabled = false;
      hmacKey = null;
      return false;
    }
  },

  async hmacSign(message) {
    if (!hmacEnabled || !hmacKey) return null;
    try {
      return await signMessage(message, hmacKey);
    } catch {
      return null;
    }
  },

  async hmacVerify(message, signature) {
    if (!hmacEnabled || !hmacKey) return true;
    if (!signature) return false;
    try {
      return await verifyMessage(message, signature, hmacKey);
    } catch {
      return false;
    }
  },

  isHmacEnabled() {
    return hmacEnabled;
  },

  generateNonce() {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join('');
  },
});
