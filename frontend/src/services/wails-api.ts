// Wails API сервис для вызова Go функций
import {
  AddTorrentByMagnet,
  GetTorrentInfo,
  GetFiles,
  GetStreamURL,
  GetAllTorrents,
  PauseTorrent,
  ResumeTorrent,
  RemoveTorrent
} from '../wailsjs/go/main/TorrentService'

import {
  CreateRoom,
  JoinRoom,
  SendMessage,
  GetPeers,
  IsHost,
  GetRoomID,
  Disconnect
} from '../wailsjs/go/main/P2PService'

import {
  Play,
  Pause,
  Seek,
  GetState,
  GetStats,
  SetSyncTolerance
} from '../wailsjs/go/main/SyncService'

import {
  GetAppInfo
} from '../wailsjs/go/main/App'

export const TorrentAPI = {
  addByMagnet: AddTorrentByMagnet,
  getInfo: GetTorrentInfo,
  getFiles: GetFiles,
  getStreamURL: GetStreamURL,
  getAll: GetAllTorrents,
  pause: PauseTorrent,
  resume: ResumeTorrent,
  remove: RemoveTorrent,
}

export const P2PAPI = {
  createRoom: CreateRoom,
  joinRoom: JoinRoom,
  sendMessage: SendMessage,
  getPeers: GetPeers,
  isHost: IsHost,
  getRoomID: GetRoomID,
  disconnect: Disconnect,
}

export const SyncAPI = {
  play: Play,
  pause: Pause,
  seek: Seek,
  getState: GetState,
  getStats: GetStats,
  setTolerance: SetSyncTolerance,
}

export const AppAPI = {
  getInfo: GetAppInfo,
}
