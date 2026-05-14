const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("torrsyncElectronTorrent", {
  addMagnet: (magnetLink) => {
    if (typeof magnetLink !== "string" || magnetLink.length > 10000) {
      return Promise.reject(new Error("Invalid magnet link"));
    }
    return ipcRenderer.invoke("torrent:addMagnet", magnetLink);
  },
  addTorrentFile: (torrentFile) => {
    if (!(torrentFile instanceof Uint8Array) && !Array.isArray(torrentFile)) {
      return Promise.reject(new Error("Invalid torrent file"));
    }
    return ipcRenderer.invoke("torrent:addTorrentFile", torrentFile);
  },
  getStats: () => ipcRenderer.invoke("torrent:getStats"),
  clear: () => ipcRenderer.invoke("torrent:clear"),
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
});
