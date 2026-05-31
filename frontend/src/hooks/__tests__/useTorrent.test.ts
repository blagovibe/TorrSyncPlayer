import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useTorrent } from '../useTorrent'

// Mock TorrentAPI
const mockAddByMagnet = vi.fn()
const mockGetFiles = vi.fn()
const mockGetStreamURL = vi.fn()

vi.mock('../../services/wails-api', () => ({
  TorrentAPI: {
    addByMagnet: (...args: any[]) => mockAddByMagnet(...args),
    getFiles: (...args: any[]) => mockGetFiles(...args),
    getStreamURL: (...args: any[]) => mockGetStreamURL(...args),
  },
}))

// Mock useWailsEvent
vi.mock('../useWails', () => ({
  useWailsEvent: vi.fn(),
}))

describe('useTorrent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns initial state', () => {
    const { result } = renderHook(() => useTorrent())
    expect(result.current.torrents).toEqual([])
    expect(result.current.currentTorrent).toBeNull()
    expect(result.current.files).toEqual([])
    expect(result.current.streamURL).toBe('')
  })

  it('adds torrent successfully', async () => {
    const mockTorrentInfo = { hash: 'test-hash', name: 'Test Torrent' }
    mockAddByMagnet.mockResolvedValue(mockTorrentInfo)
    const { result } = renderHook(() => useTorrent())
    await act(async () => {
      await result.current.addTorrent('magnet:?xt=urn:btih:test')
    })
    expect(mockAddByMagnet).toHaveBeenCalledWith('magnet:?xt=urn:btih:test')
    expect(result.current.currentTorrent).toEqual(mockTorrentInfo)
  })

  it('handles add torrent error', async () => {
    const error = new Error('Failed to add')
    mockAddByMagnet.mockRejectedValue(error)
    const { result } = renderHook(() => useTorrent())
    await expect(
      act(async () => {
        await result.current.addTorrent('magnet:?xt=urn:btih:test')
      })
    ).rejects.toThrow('Failed to add')
  })

  it('loads files successfully', async () => {
    const mockFiles = [
      { path: 'video.mp4', size: 1000, progress: 1 },
      { path: 'subtitle.srt', size: 100, progress: 1 },
    ]
    mockGetFiles.mockResolvedValue(mockFiles)
    const { result } = renderHook(() => useTorrent())
    await act(async () => {
      await result.current.loadFiles('test-hash')
    })
    expect(mockGetFiles).toHaveBeenCalledWith('test-hash')
    expect(result.current.files).toEqual(mockFiles)
  })

  it('handles load files error', async () => {
    const error = new Error('Failed to load')
    mockGetFiles.mockRejectedValue(error)
    const { result } = renderHook(() => useTorrent())
    await expect(
      act(async () => {
        await result.current.loadFiles('test-hash')
      })
    ).rejects.toThrow('Failed to load')
  })

  it('gets stream URL successfully', async () => {
    const mockURL = 'http://localhost/stream/video.mp4'
    mockGetStreamURL.mockResolvedValue(mockURL)
    const { result } = renderHook(() => useTorrent())
    await act(async () => {
      await result.current.getStreamUrl('test-hash', 'video.mp4')
    })
    expect(mockGetStreamURL).toHaveBeenCalledWith('test-hash', 'video.mp4')
    expect(result.current.streamURL).toBe(mockURL)
  })

  it('handles get stream URL error', async () => {
    const error = new Error('Failed to get URL')
    mockGetStreamURL.mockRejectedValue(error)
    const { result } = renderHook(() => useTorrent())
    await expect(
      act(async () => {
        await result.current.getStreamUrl('test-hash', 'video.mp4')
      })
    ).rejects.toThrow('Failed to get URL')
  })
})
