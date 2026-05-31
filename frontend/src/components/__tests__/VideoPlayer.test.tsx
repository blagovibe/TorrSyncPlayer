import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import { VideoPlayer } from '../VideoPlayer'

describe('VideoPlayer', () => {
  const defaultProps = {
    src: 'test-video.mp4',
    isPlaying: false,
    position: 0,
    onPlay: vi.fn(),
    onPause: vi.fn(),
    onSeek: vi.fn(),
  }

  it('renders video player', () => {
    render(<VideoPlayer {...defaultProps} />)
    expect(screen.getByTestId('video-element')).toBeInTheDocument()
  })

  it('renders play button when not playing', () => {
    render(<VideoPlayer {...defaultProps} isPlaying={false} />)
    const playButton = screen.getByTitle('Воспроизвести')
    expect(playButton).toBeInTheDocument()
  })

  it('renders pause button when playing', () => {
    render(<VideoPlayer {...defaultProps} isPlaying={true} />)
    const pauseButton = screen.getByTitle('Пауза')
    expect(pauseButton).toBeInTheDocument()
  })

  it('calls onPlay when play button is clicked', () => {
    const onPlay = vi.fn()
    render(<VideoPlayer {...defaultProps} isPlaying={false} onPlay={onPlay} />)
    const playButton = screen.getByTitle('Воспроизвести')
    fireEvent.click(playButton)
    expect(onPlay).toHaveBeenCalled()
  })

  it('calls onPause when pause button is clicked', () => {
    const onPause = vi.fn()
    render(<VideoPlayer {...defaultProps} isPlaying={true} onPause={onPause} />)
    const pauseButton = screen.getByTitle('Пауза')
    fireEvent.click(pauseButton)
    expect(onPause).toHaveBeenCalled()
  })

  it('calls onSeek when progress bar is clicked', () => {
    const onSeek = vi.fn()
    render(<VideoPlayer {...defaultProps} onSeek={onSeek} />)
    const progressContainer = screen.getByTestId('progress-container')
    fireEvent.click(progressContainer, { clientX: 50 })
    expect(onSeek).toHaveBeenCalled()
  })

  it('displays time format correctly', () => {
    render(<VideoPlayer {...defaultProps} />)
    expect(screen.getByText('0:00 / 0:00')).toBeInTheDocument()
  })
})
