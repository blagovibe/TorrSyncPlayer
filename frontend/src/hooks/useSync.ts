import { useState, useCallback } from 'react'
import { SyncAPI } from '../services/wails-api'
import { useWailsEvent } from './useWails'

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

export function useSync() {
  const [state, setState] = useState<PlaybackState>({
    isPlaying: false,
    position: 0,
    duration: 0,
    timestamp: 0,
    playbackRate: 1,
  })

  const [stats, setStats] = useState<SyncStats>({
    rtt: 0,
    drift: 0,
    lastSyncTime: 0,
    syncTolerance: 1500,
    correctionCount: 0,
  })

  // Подписка на события синхронизации с валидацией
  useWailsEvent<PlaybackState>('sync:state_changed', (newState) => {
    // Валидация данных
    if (newState && typeof newState === 'object') {
      const validatedState: PlaybackState = {
        isPlaying: typeof newState.isPlaying === 'boolean' ? newState.isPlaying : false,
        position: typeof newState.position === 'number' && !isNaN(newState.position) && newState.position >= 0
          ? newState.position : 0,
        duration: typeof newState.duration === 'number' && !isNaN(newState.duration) && newState.duration >= 0
          ? newState.duration : 0,
        timestamp: typeof newState.timestamp === 'number' && !isNaN(newState.timestamp)
          ? newState.timestamp : 0,
        playbackRate: typeof newState.playbackRate === 'number' && !isNaN(newState.playbackRate)
          ? newState.playbackRate : 1,
      }
      setState(validatedState)
    }
  })

  useWailsEvent<SyncStats>('sync:stats_updated', (newStats) => {
    // Валидация данных
    if (newStats && typeof newStats === 'object') {
      const validatedStats: SyncStats = {
        rtt: typeof newStats.rtt === 'number' && !isNaN(newStats.rtt) && newStats.rtt >= 0
          ? newStats.rtt : 0,
        drift: typeof newStats.drift === 'number' && !isNaN(newStats.drift)
          ? newStats.drift : 0,
        lastSyncTime: typeof newStats.lastSyncTime === 'number' && !isNaN(newStats.lastSyncTime)
          ? newStats.lastSyncTime : 0,
        syncTolerance: typeof newStats.syncTolerance === 'number' && !isNaN(newStats.syncTolerance)
          ? newStats.syncTolerance : 1500,
        correctionCount: typeof newStats.correctionCount === 'number' && !isNaN(newStats.correctionCount)
          ? newStats.correctionCount : 0,
      }
      setStats(validatedStats)
    }
  })

  const play = useCallback(async (position: number) => {
    try {
      await SyncAPI.play(position)
    } catch (error) {
      console.error('Failed to play:', error)
      throw error
    }
  }, [])

  const pause = useCallback(async (position: number) => {
    try {
      await SyncAPI.pause(position)
    } catch (error) {
      console.error('Failed to pause:', error)
      throw error
    }
  }, [])

  const seek = useCallback(async (position: number) => {
    try {
      await SyncAPI.seek(position)
    } catch (error) {
      console.error('Failed to seek:', error)
      throw error
    }
  }, [])

  const setTolerance = useCallback(async (toleranceMs: number) => {
    try {
      await SyncAPI.setTolerance(toleranceMs)
    } catch (error) {
      console.error('Failed to set tolerance:', error)
      throw error
    }
  }, [])

  return {
    state,
    stats,
    play,
    pause,
    seek,
    setTolerance,
  }
}
