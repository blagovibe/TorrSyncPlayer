import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useSync } from '../useSync'

// Mock Wails runtime
const mockEventsOn = vi.fn()
const mockEventsEmit = vi.fn()

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: (...args: any[]) => mockEventsOn(...args),
  EventsEmit: (...args: any[]) => mockEventsEmit(...args),
}))

// Mock SyncAPI
const mockPlay = vi.fn()
const mockPause = vi.fn()
const mockSeek = vi.fn()
const mockSetTolerance = vi.fn()

vi.mock('../../services/wails-api', () => ({
  SyncAPI: {
    play: (...args: any[]) => mockPlay(...args),
    pause: (...args: any[]) => mockPause(...args),
    seek: (...args: any[]) => mockSeek(...args),
    setTolerance: (...args: any[]) => mockSetTolerance(...args),
  },
}))

describe('useSync', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockEventsOn.mockReturnValue(vi.fn())
  })

  describe('Initial State', () => {
    it('returns initial state with default values', () => {
      const { result } = renderHook(() => useSync())

      expect(result.current.state).toEqual({
        isPlaying: false,
        position: 0,
        duration: 0,
        timestamp: 0,
        playbackRate: 1,
      })

      expect(result.current.stats).toEqual({
        rtt: 0,
        drift: 0,
        lastSyncTime: 0,
        syncTolerance: 1500,
        correctionCount: 0,
      })
    })
  })

  describe('PlaybackState Events', () => {
    it('handles sync:state_changed event with valid data', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      expect(stateChangedHandler).toBeDefined()

      const validState = {
        isPlaying: true,
        position: 10.5,
        duration: 120,
        timestamp: 1704067200000,
        playbackRate: 1.5,
      }

      act(() => {
        stateChangedHandler(validState)
      })

      expect(result.current.state).toEqual(validState)
    })

    it('handles sync:state_changed with partial data', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      act(() => {
        stateChangedHandler({ isPlaying: true })
      })

      expect(result.current.state.isPlaying).toBe(true)
      expect(result.current.state.position).toBe(0)
      expect(result.current.state.duration).toBe(0)
      expect(result.current.state.timestamp).toBe(0)
      expect(result.current.state.playbackRate).toBe(1)
    })

    it('handles sync:state_changed with invalid data types', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      act(() => {
        stateChangedHandler({
          isPlaying: 'yes',
          position: 'ten',
          duration: 'long',
          timestamp: 'now',
          playbackRate: 'fast',
        })
      })

      expect(result.current.state.isPlaying).toBe(false)
      expect(result.current.state.position).toBe(0)
      expect(result.current.state.duration).toBe(0)
      expect(result.current.state.timestamp).toBe(0)
      expect(result.current.state.playbackRate).toBe(1)
    })

    it('handles sync:state_changed with NaN values', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      act(() => {
        stateChangedHandler({
          isPlaying: true,
          position: NaN,
          duration: NaN,
          timestamp: NaN,
          playbackRate: NaN,
        })
      })

      expect(result.current.state.isPlaying).toBe(true)
      expect(result.current.state.position).toBe(0)
      expect(result.current.state.duration).toBe(0)
      expect(result.current.state.timestamp).toBe(0)
      expect(result.current.state.playbackRate).toBe(1)
    })

    it('handles sync:state_changed with negative values', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      act(() => {
        stateChangedHandler({
          isPlaying: true,
          position: -10,
          duration: -100,
          timestamp: -1000,
          playbackRate: -1,
        })
      })

      expect(result.current.state.isPlaying).toBe(true)
      expect(result.current.state.position).toBe(0)
      expect(result.current.state.duration).toBe(0)
      expect(result.current.state.timestamp).toBe(-1000) // timestamp может быть отрицательным
      expect(result.current.state.playbackRate).toBe(-1) // playbackRate может быть отрицательным
    })

    it('handles sync:state_changed with null data', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      act(() => {
        stateChangedHandler(null)
      })

      expect(result.current.state).toEqual({
        isPlaying: false,
        position: 0,
        duration: 0,
        timestamp: 0,
        playbackRate: 1,
      })
    })

    it('handles sync:state_changed with undefined data', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      act(() => {
        stateChangedHandler(undefined)
      })

      expect(result.current.state).toEqual({
        isPlaying: false,
        position: 0,
        duration: 0,
        timestamp: 0,
        playbackRate: 1,
      })
    })
  })

  describe('SyncStats Events', () => {
    it('handles sync:stats_updated event with valid data', () => {
      const { result } = renderHook(() => useSync())

      const statsUpdatedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:stats_updated'
      )?.[1]

      expect(statsUpdatedHandler).toBeDefined()

      const validStats = {
        rtt: 50,
        drift: 10,
        lastSyncTime: 1704067200000,
        syncTolerance: 2000,
        correctionCount: 5,
      }

      act(() => {
        statsUpdatedHandler(validStats)
      })

      expect(result.current.stats).toEqual(validStats)
    })

    it('handles sync:stats_updated with partial data', () => {
      const { result } = renderHook(() => useSync())

      const statsUpdatedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:stats_updated'
      )?.[1]

      act(() => {
        statsUpdatedHandler({ rtt: 100 })
      })

      expect(result.current.stats.rtt).toBe(100)
      expect(result.current.stats.drift).toBe(0)
      expect(result.current.stats.lastSyncTime).toBe(0)
      expect(result.current.stats.syncTolerance).toBe(1500)
      expect(result.current.stats.correctionCount).toBe(0)
    })

    it('handles sync:stats_updated with invalid data types', () => {
      const { result } = renderHook(() => useSync())

      const statsUpdatedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:stats_updated'
      )?.[1]

      act(() => {
        statsUpdatedHandler({
          rtt: 'fast',
          drift: 'small',
          lastSyncTime: 'recent',
          syncTolerance: 'high',
          correctionCount: 'many',
        })
      })

      expect(result.current.stats.rtt).toBe(0)
      expect(result.current.stats.drift).toBe(0)
      expect(result.current.stats.lastSyncTime).toBe(0)
      expect(result.current.stats.syncTolerance).toBe(1500)
      expect(result.current.stats.correctionCount).toBe(0)
    })

    it('handles sync:stats_updated with NaN values', () => {
      const { result } = renderHook(() => useSync())

      const statsUpdatedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:stats_updated'
      )?.[1]

      act(() => {
        statsUpdatedHandler({
          rtt: NaN,
          drift: NaN,
          lastSyncTime: NaN,
          syncTolerance: NaN,
          correctionCount: NaN,
        })
      })

      expect(result.current.stats.rtt).toBe(0)
      expect(result.current.stats.drift).toBe(0)
      expect(result.current.stats.lastSyncTime).toBe(0)
      expect(result.current.stats.syncTolerance).toBe(1500)
      expect(result.current.stats.correctionCount).toBe(0)
    })

    it('handles sync:stats_updated with negative values', () => {
      const { result } = renderHook(() => useSync())

      const statsUpdatedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:stats_updated'
      )?.[1]

      act(() => {
        statsUpdatedHandler({
          rtt: -50,
          drift: -10,
          lastSyncTime: -1000,
          syncTolerance: -2000,
          correctionCount: -5,
        })
      })

      expect(result.current.stats.rtt).toBe(0)
      expect(result.current.stats.drift).toBe(-10) // drift может быть отрицательным
      expect(result.current.stats.lastSyncTime).toBe(-1000) // lastSyncTime может быть отрицательным
      expect(result.current.stats.syncTolerance).toBe(-2000) // syncTolerance может быть отрицательным
      expect(result.current.stats.correctionCount).toBe(-5) // correctionCount может быть отрицательным
    })

    it('handles sync:stats_updated with null data', () => {
      const { result } = renderHook(() => useSync())

      const statsUpdatedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:stats_updated'
      )?.[1]

      act(() => {
        statsUpdatedHandler(null)
      })

      expect(result.current.stats).toEqual({
        rtt: 0,
        drift: 0,
        lastSyncTime: 0,
        syncTolerance: 1500,
        correctionCount: 0,
      })
    })
  })

  describe('play', () => {
    it('calls SyncAPI.play with position', async () => {
      mockPlay.mockResolvedValue(undefined)

      const { result } = renderHook(() => useSync())

      await act(async () => {
        await result.current.play(10.5)
      })

      expect(mockPlay).toHaveBeenCalledWith(10.5)
    })

    it('calls SyncAPI.play with zero position', async () => {
      mockPlay.mockResolvedValue(undefined)

      const { result } = renderHook(() => useSync())

      await act(async () => {
        await result.current.play(0)
      })

      expect(mockPlay).toHaveBeenCalledWith(0)
    })

    it('throws error when SyncAPI.play fails', async () => {
      mockPlay.mockRejectedValue(new Error('Play failed'))

      const { result } = renderHook(() => useSync())

      await expect(
        act(async () => {
          await result.current.play(10)
        })
      ).rejects.toThrow('Play failed')
    })
  })

  describe('pause', () => {
    it('calls SyncAPI.pause with position', async () => {
      mockPause.mockResolvedValue(undefined)

      const { result } = renderHook(() => useSync())

      await act(async () => {
        await result.current.pause(20.5)
      })

      expect(mockPause).toHaveBeenCalledWith(20.5)
    })

    it('calls SyncAPI.pause with zero position', async () => {
      mockPause.mockResolvedValue(undefined)

      const { result } = renderHook(() => useSync())

      await act(async () => {
        await result.current.pause(0)
      })

      expect(mockPause).toHaveBeenCalledWith(0)
    })

    it('throws error when SyncAPI.pause fails', async () => {
      mockPause.mockRejectedValue(new Error('Pause failed'))

      const { result } = renderHook(() => useSync())

      await expect(
        act(async () => {
          await result.current.pause(20)
        })
      ).rejects.toThrow('Pause failed')
    })
  })

  describe('seek', () => {
    it('calls SyncAPI.seek with position', async () => {
      mockSeek.mockResolvedValue(undefined)

      const { result } = renderHook(() => useSync())

      await act(async () => {
        await result.current.seek(30.5)
      })

      expect(mockSeek).toHaveBeenCalledWith(30.5)
    })

    it('calls SyncAPI.seek with zero position', async () => {
      mockSeek.mockResolvedValue(undefined)

      const { result } = renderHook(() => useSync())

      await act(async () => {
        await result.current.seek(0)
      })

      expect(mockSeek).toHaveBeenCalledWith(0)
    })

    it('throws error when SyncAPI.seek fails', async () => {
      mockSeek.mockRejectedValue(new Error('Seek failed'))

      const { result } = renderHook(() => useSync())

      await expect(
        act(async () => {
          await result.current.seek(30)
        })
      ).rejects.toThrow('Seek failed')
    })
  })

  describe('setTolerance', () => {
    it('calls SyncAPI.setTolerance with value', async () => {
      mockSetTolerance.mockResolvedValue(undefined)

      const { result } = renderHook(() => useSync())

      await act(async () => {
        await result.current.setTolerance(2000)
      })

      expect(mockSetTolerance).toHaveBeenCalledWith(2000)
    })

    it('calls SyncAPI.setTolerance with zero value', async () => {
      mockSetTolerance.mockResolvedValue(undefined)

      const { result } = renderHook(() => useSync())

      await act(async () => {
        await result.current.setTolerance(0)
      })

      expect(mockSetTolerance).toHaveBeenCalledWith(0)
    })

    it('throws error when SyncAPI.setTolerance fails', async () => {
      mockSetTolerance.mockRejectedValue(new Error('Set tolerance failed'))

      const { result } = renderHook(() => useSync())

      await expect(
        act(async () => {
          await result.current.setTolerance(2000)
        })
      ).rejects.toThrow('Set tolerance failed')
    })
  })

  describe('PlaybackState Validation', () => {
    it('validates isPlaying field correctly', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      // Valid boolean true
      act(() => {
        stateChangedHandler({ isPlaying: true })
      })
      expect(result.current.state.isPlaying).toBe(true)

      // Valid boolean false
      act(() => {
        stateChangedHandler({ isPlaying: false })
      })
      expect(result.current.state.isPlaying).toBe(false)

      // Invalid string
      act(() => {
        stateChangedHandler({ isPlaying: 'true' })
      })
      expect(result.current.state.isPlaying).toBe(false)

      // Invalid number
      act(() => {
        stateChangedHandler({ isPlaying: 1 })
      })
      expect(result.current.state.isPlaying).toBe(false)
    })

    it('validates position field correctly', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      // Valid positive number
      act(() => {
        stateChangedHandler({ position: 100.5 })
      })
      expect(result.current.state.position).toBe(100.5)

      // Valid zero
      act(() => {
        stateChangedHandler({ position: 0 })
      })
      expect(result.current.state.position).toBe(0)

      // Invalid negative number
      act(() => {
        stateChangedHandler({ position: -10 })
      })
      expect(result.current.state.position).toBe(0)

      // Invalid NaN
      act(() => {
        stateChangedHandler({ position: NaN })
      })
      expect(result.current.state.position).toBe(0)

      // Invalid string
      act(() => {
        stateChangedHandler({ position: '100' })
      })
      expect(result.current.state.position).toBe(0)
    })

    it('validates duration field correctly', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      // Valid positive number
      act(() => {
        stateChangedHandler({ duration: 3600 })
      })
      expect(result.current.state.duration).toBe(3600)

      // Valid zero
      act(() => {
        stateChangedHandler({ duration: 0 })
      })
      expect(result.current.state.duration).toBe(0)

      // Invalid negative number
      act(() => {
        stateChangedHandler({ duration: -100 })
      })
      expect(result.current.state.duration).toBe(0)

      // Invalid NaN
      act(() => {
        stateChangedHandler({ duration: NaN })
      })
      expect(result.current.state.duration).toBe(0)
    })

    it('validates playbackRate field correctly', () => {
      const { result } = renderHook(() => useSync())

      const stateChangedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:state_changed'
      )?.[1]

      // Valid positive number
      act(() => {
        stateChangedHandler({ playbackRate: 2.0 })
      })
      expect(result.current.state.playbackRate).toBe(2.0)

      // Valid 1 (default)
      act(() => {
        stateChangedHandler({ playbackRate: 1 })
      })
      expect(result.current.state.playbackRate).toBe(1)

      // Invalid NaN
      act(() => {
        stateChangedHandler({ playbackRate: NaN })
      })
      expect(result.current.state.playbackRate).toBe(1)

      // Invalid string
      act(() => {
        stateChangedHandler({ playbackRate: '2x' })
      })
      expect(result.current.state.playbackRate).toBe(1)
    })
  })

  describe('SyncStats Validation', () => {
    it('validates rtt field correctly', () => {
      const { result } = renderHook(() => useSync())

      const statsUpdatedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:stats_updated'
      )?.[1]

      // Valid positive number
      act(() => {
        statsUpdatedHandler({ rtt: 100 })
      })
      expect(result.current.stats.rtt).toBe(100)

      // Valid zero
      act(() => {
        statsUpdatedHandler({ rtt: 0 })
      })
      expect(result.current.stats.rtt).toBe(0)

      // Invalid negative number
      act(() => {
        statsUpdatedHandler({ rtt: -50 })
      })
      expect(result.current.stats.rtt).toBe(0)

      // Invalid NaN
      act(() => {
        statsUpdatedHandler({ rtt: NaN })
      })
      expect(result.current.stats.rtt).toBe(0)
    })

    it('validates drift field correctly', () => {
      const { result } = renderHook(() => useSync())

      const statsUpdatedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:stats_updated'
      )?.[1]

      // Valid positive number
      act(() => {
        statsUpdatedHandler({ drift: 50 })
      })
      expect(result.current.stats.drift).toBe(50)

      // Valid negative number (drift может быть отрицательным)
      act(() => {
        statsUpdatedHandler({ drift: -50 })
      })
      expect(result.current.stats.drift).toBe(-50)

      // Valid zero
      act(() => {
        statsUpdatedHandler({ drift: 0 })
      })
      expect(result.current.stats.drift).toBe(0)

      // Invalid NaN
      act(() => {
        statsUpdatedHandler({ drift: NaN })
      })
      expect(result.current.stats.drift).toBe(0)
    })

    it('validates syncTolerance field correctly', () => {
      const { result } = renderHook(() => useSync())

      const statsUpdatedHandler = mockEventsOn.mock.calls.find(
        (call: any[]) => call[0] === 'sync:stats_updated'
      )?.[1]

      // Valid positive number
      act(() => {
        statsUpdatedHandler({ syncTolerance: 2000 })
      })
      expect(result.current.stats.syncTolerance).toBe(2000)

      // Valid zero
      act(() => {
        statsUpdatedHandler({ syncTolerance: 0 })
      })
      expect(result.current.stats.syncTolerance).toBe(0)

      // Invalid NaN
      act(() => {
        statsUpdatedHandler({ syncTolerance: NaN })
      })
      expect(result.current.stats.syncTolerance).toBe(1500) // default value
    })
  })
})
