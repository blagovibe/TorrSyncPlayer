/// <reference types="node" />
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { RoomPage } from '../RoomPage'

// Mock hooks
const mockUseP2P = {
  isHost: true,
  roomID: 'room-123',
  peers: [
    { id: 'peer-1', isHost: false, connected: true, lastSeen: '2023-01-01' },
    { id: 'peer-2', isHost: false, connected: true, lastSeen: '2023-01-01' },
  ],
  disconnect: vi.fn(),
}

const mockUseTorrent: {
  currentTorrent: { hash: string; name: string } | null
  files: { path: string; size: number; progress: number }[]
  streamURL: string
  loadFiles: ReturnType<typeof vi.fn>
  getStreamUrl: ReturnType<typeof vi.fn>
} = {
  currentTorrent: { hash: 'test-hash', name: 'Test Torrent' },
  files: [
    { path: 'video.mp4', size: 1000, progress: 1 },
    { path: 'subtitle.srt', size: 100, progress: 1 },
  ],
  streamURL: 'http://localhost/stream/video.mp4',
  loadFiles: vi.fn(),
  getStreamUrl: vi.fn(),
}

const mockUseSync = {
  state: { isPlaying: false, position: 0, duration: 100, timestamp: 0, playbackRate: 1 },
  play: vi.fn(),
  pause: vi.fn(),
  seek: vi.fn(),
}

vi.mock('../../hooks/useP2P', () => ({
  useP2P: () => mockUseP2P,
}))

vi.mock('../../hooks/useTorrent', () => ({
  useTorrent: () => mockUseTorrent,
}))

vi.mock('../../hooks/useSync', () => ({
  useSync: () => mockUseSync,
}))

describe('RoomPage', () => {
  it('renders room page with room ID', () => {
    render(<RoomPage />)
    // Component shows "Комната: room-123..." (truncated with ellipsis)
    expect(screen.getByText(/room-123/)).toBeInTheDocument()
  })

  it('renders video player when stream URL is available', () => {
    render(<RoomPage />)
    expect(screen.getByTestId('room-video')).toBeInTheDocument()
  })

  it('renders peer list', () => {
    render(<RoomPage />)
    // Component shows "User 1", "User 2" instead of peer ids
    expect(screen.getByText('User 1')).toBeInTheDocument()
    expect(screen.getByText('User 2')).toBeInTheDocument()
  })

  it('shows no video message when stream URL is empty', () => {
    // Update the mock for this test
    mockUseTorrent.streamURL = ''
    mockUseTorrent.files = []
    mockUseTorrent.currentTorrent = null
    
    render(<RoomPage />)
    expect(screen.getByText('Выберите файл для воспроизведения')).toBeInTheDocument()
    
    // Restore mock
    mockUseTorrent.streamURL = 'http://localhost/stream/video.mp4'
    mockUseTorrent.files = [
      { path: 'video.mp4', size: 1000, progress: 1 },
      { path: 'subtitle.srt', size: 100, progress: 1 },
    ]
    mockUseTorrent.currentTorrent = { hash: 'test-hash', name: 'Test Torrent' }
  })
})
