export interface TorrentInfo {
  hash: string
  name: string
  size: number
  progress: number
  peers: number
  seeds: number
  downloadSpeed: number
  uploadSpeed: number
}

export interface TorrentFile {
  path: string
  size: number
  progress: number
}

export interface PeerInfo {
  id: string
  isHost: boolean
  connected: boolean
  lastSeen: string
}

export interface PlaybackState {
  isPlaying: boolean
  position: number
  duration: number
  timestamp: number
  playbackRate: number
}

export interface SyncStats {
  rtt: number
  drift: number
  lastSyncTime: number
  syncTolerance: number
  correctionCount: number
}
