import { useState, useCallback } from 'react'
import { TorrentAPI } from '../services/wails-api'
import { useWailsEvent } from './useWails'

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

export function useTorrent() {
  const [torrents, setTorrents] = useState<TorrentInfo[]>([])
  const [currentTorrent, setCurrentTorrent] = useState<TorrentInfo | null>(null)
  const [files, setFiles] = useState<TorrentFile[]>([])
  const [streamURL, setStreamURL] = useState<string>('')

  // Подписка на события торрента с валидацией
  useWailsEvent<TorrentInfo>('torrent:added', (torrent) => {
    // Валидация данных торрента
    if (torrent && typeof torrent === 'object' && typeof torrent.hash === 'string') {
      const validatedTorrent: TorrentInfo = {
        hash: torrent.hash,
        name: typeof torrent.name === 'string' ? torrent.name : 'Unknown',
        size: typeof torrent.size === 'number' && !isNaN(torrent.size) && torrent.size >= 0
          ? torrent.size : 0,
        progress: typeof torrent.progress === 'number' && !isNaN(torrent.progress)
          ? Math.max(0, Math.min(1, torrent.progress)) : 0,
        peers: typeof torrent.peers === 'number' && !isNaN(torrent.peers) && torrent.peers >= 0
          ? Math.floor(torrent.peers) : 0,
        seeds: typeof torrent.seeds === 'number' && !isNaN(torrent.seeds) && torrent.seeds >= 0
          ? Math.floor(torrent.seeds) : 0,
        downloadSpeed: typeof torrent.downloadSpeed === 'number' && !isNaN(torrent.downloadSpeed)
          ? torrent.downloadSpeed : 0,
        uploadSpeed: typeof torrent.uploadSpeed === 'number' && !isNaN(torrent.uploadSpeed)
          ? torrent.uploadSpeed : 0,
      }
      setTorrents(prev => [...prev, validatedTorrent])
    }
  })

  useWailsEvent<TorrentInfo>('torrent:progress', (torrent) => {
    // Валидация данных торрента
    if (torrent && typeof torrent === 'object' && typeof torrent.hash === 'string') {
      const validatedTorrent: TorrentInfo = {
        hash: torrent.hash,
        name: typeof torrent.name === 'string' ? torrent.name : 'Unknown',
        size: typeof torrent.size === 'number' && !isNaN(torrent.size) && torrent.size >= 0
          ? torrent.size : 0,
        progress: typeof torrent.progress === 'number' && !isNaN(torrent.progress)
          ? Math.max(0, Math.min(1, torrent.progress)) : 0,
        peers: typeof torrent.peers === 'number' && !isNaN(torrent.peers) && torrent.peers >= 0
          ? Math.floor(torrent.peers) : 0,
        seeds: typeof torrent.seeds === 'number' && !isNaN(torrent.seeds) && torrent.seeds >= 0
          ? Math.floor(torrent.seeds) : 0,
        downloadSpeed: typeof torrent.downloadSpeed === 'number' && !isNaN(torrent.downloadSpeed)
          ? torrent.downloadSpeed : 0,
        uploadSpeed: typeof torrent.uploadSpeed === 'number' && !isNaN(torrent.uploadSpeed)
          ? torrent.uploadSpeed : 0,
      }
      setTorrents(prev => prev.map(t => t.hash === validatedTorrent.hash ? validatedTorrent : t))
    }
  })

  const addTorrent = useCallback(async (magnetURI: string) => {
    try {
      const info = await TorrentAPI.addByMagnet(magnetURI)
      setCurrentTorrent(info)
      return info
    } catch (error) {
      console.error('Failed to add torrent:', error)
      throw error
    }
  }, [])

  const loadFiles = useCallback(async (hash: string) => {
    try {
      const fileList = await TorrentAPI.getFiles(hash)
      setFiles(fileList)
      return fileList
    } catch (error) {
      console.error('Failed to load files:', error)
      throw error
    }
  }, [])

  const getStreamUrl = useCallback(async (hash: string, filePath: string) => {
    try {
      const url = await TorrentAPI.getStreamURL(hash, filePath)
      setStreamURL(url)
      return url
    } catch (error) {
      console.error('Failed to get stream URL:', error)
      throw error
    }
  }, [])

  return {
    torrents,
    currentTorrent,
    files,
    streamURL,
    addTorrent,
    loadFiles,
    getStreamUrl,
  }
}
