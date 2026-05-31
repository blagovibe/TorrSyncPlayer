// Автоматически генерируется Wails

export function AddTorrentByMagnet(magnetURI: string): Promise<any> {
  return window.go?.main?.TorrentService?.AddTorrentByMagnet(magnetURI) || Promise.reject('Wails not available')
}

export function GetTorrentInfo(hash: string): Promise<any> {
  return window.go?.main?.TorrentService?.GetTorrentInfo(hash) || Promise.reject('Wails not available')
}

export function GetFiles(hash: string): Promise<any> {
  return window.go?.main?.TorrentService?.GetFiles(hash) || Promise.reject('Wails not available')
}

export function GetStreamURL(hash: string, filePath: string): Promise<string> {
  return window.go?.main?.TorrentService?.GetStreamURL(hash, filePath) || Promise.reject('Wails not available')
}

export function GetAllTorrents(): Promise<any> {
  return window.go?.main?.TorrentService?.GetAllTorrents() || Promise.reject('Wails not available')
}

export function PauseTorrent(hash: string): Promise<void> {
  return window.go?.main?.TorrentService?.PauseTorrent(hash) || Promise.reject('Wails not available')
}

export function ResumeTorrent(hash: string): Promise<void> {
  return window.go?.main?.TorrentService?.ResumeTorrent(hash) || Promise.reject('Wails not available')
}

export function RemoveTorrent(hash: string): Promise<void> {
  return window.go?.main?.TorrentService?.RemoveTorrent(hash) || Promise.reject('Wails not available')
}

export function SetFilePriority(hash: string, filePath: string, priority: number): Promise<void> {
  return window.go?.main?.TorrentService?.SetFilePriority(hash, filePath, priority) || Promise.reject('Wails not available')
}
