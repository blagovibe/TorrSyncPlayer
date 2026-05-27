const { contextBridge, ipcRenderer } = require("electron");
const shared = require("../torrent-shared.json");

const MAX_TORRENT_FILE_BYTES = shared.maxTorrentFileBytes;
const MAX_TORRENT_FILE_BYTES_MB = MAX_TORRENT_FILE_BYTES / 1024 / 1024;
const MAX_MAGNET_LINK_LENGTH = shared.maxMagnetLinkLength;

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
