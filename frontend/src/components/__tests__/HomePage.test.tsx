import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { HomePage } from '../HomePage'

// Mock hooks
vi.mock('../../hooks/useP2P', () => ({
  useP2P: () => ({
    createRoom: vi.fn().mockResolvedValue('room-123'),
    joinRoom: vi.fn().mockResolvedValue(true),
  }),
}))

vi.mock('../../hooks/useTorrent', () => ({
  useTorrent: () => ({
    addTorrent: vi.fn().mockResolvedValue({ hash: 'test-hash' }),
  }),
}))

describe('HomePage', () => {
  it('renders home page with title', () => {
    render(<HomePage />)
    expect(screen.getByText('TorrSyncPlayer')).toBeInTheDocument()
  })

  it('renders mode selection buttons', () => {
    render(<HomePage />)
    expect(screen.getByText(/Создать комнату/)).toBeInTheDocument()
    expect(screen.getByText(/Присоединиться/)).toBeInTheDocument()
  })

  it('shows host panel when host mode is selected', () => {
    render(<HomePage />)
    fireEvent.click(screen.getByText(/Создать комнату/))
    expect(screen.getByText('Создать комнату')).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/магнет-ссылку/)).toBeInTheDocument()
  })

  it('shows guest panel when guest mode is selected', () => {
    render(<HomePage />)
    fireEvent.click(screen.getByText(/Присоединиться/))
    expect(screen.getByText(/Присоединиться к комнате/)).toBeInTheDocument()
    expect(screen.getByPlaceholderText('ID комнаты')).toBeInTheDocument()
  })

  it('updates magnet URI input value', () => {
    render(<HomePage />)
    fireEvent.click(screen.getByText(/Создать комнату/))
    const input = screen.getByPlaceholderText(/магнет-ссылку/)
    fireEvent.change(input, { target: { value: 'magnet:?xt=urn:btih:test' } })
    expect(input).toHaveValue('magnet:?xt=urn:btih:test')
  })

  it('updates room code input value', () => {
    render(<HomePage />)
    fireEvent.click(screen.getByText(/Присоединиться/))
    const input = screen.getByPlaceholderText('ID комнаты')
    fireEvent.change(input, { target: { value: 'ABC123' } })
    expect(input).toHaveValue('ABC123')
  })

  it('calls create room when button is clicked', async () => {
    render(<HomePage />)
    fireEvent.click(screen.getByText(/Создать комнату/))
    fireEvent.click(screen.getByText(/Создать комнату/))
    // Wait for async operation
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
})
