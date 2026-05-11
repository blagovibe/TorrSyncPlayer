const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("torrsyncElectronTorrent", {
  addMagnet: (magnetLink) => ipcRenderer.invoke("torrent:addMagnet", magnetLink),
  addTorrentFile: (torrentFile) => ipcRenderer.invoke("torrent:addTorrentFile", torrentFile),
  getStats: () => ipcRenderer.invoke("torrent:getStats"),
  clear: () => ipcRenderer.invoke("torrent:clear"),
});
