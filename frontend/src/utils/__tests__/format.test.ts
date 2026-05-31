import { describe, it, expect } from 'vitest'
import { formatBytes, formatSpeed, formatTime } from '../format'

describe('formatBytes', () => {
  it('formats 0 bytes correctly', () => {
    expect(formatBytes(0)).toBe('0 B')
  })

  it('formats bytes correctly', () => {
    expect(formatBytes(500)).toBe('500 B')
  })

  it('formats kilobytes correctly', () => {
    expect(formatBytes(1024)).toBe('1 KB')
  })

  it('formats megabytes correctly', () => {
    expect(formatBytes(1024 * 1024)).toBe('1 MB')
  })

  it('formats gigabytes correctly', () => {
    expect(formatBytes(1024 * 1024 * 1024)).toBe('1 GB')
  })

  it('formats terabytes correctly', () => {
    expect(formatBytes(1024 * 1024 * 1024 * 1024)).toBe('1 TB')
  })

  it('formats with decimal places', () => {
    expect(formatBytes(1536)).toBe('1.5 KB')
  })

  it('formats large values correctly', () => {
    expect(formatBytes(5 * 1024 * 1024 * 1024)).toBe('5 GB')
  })
})

describe('formatSpeed', () => {
  it('formats 0 speed correctly', () => {
    expect(formatSpeed(0)).toBe('0 B/s')
  })

  it('formats speed in bytes per second', () => {
    expect(formatSpeed(500)).toBe('500 B/s')
  })

  it('formats speed in kilobytes per second', () => {
    expect(formatSpeed(1024)).toBe('1 KB/s')
  })

  it('formats speed in megabytes per second', () => {
    expect(formatSpeed(1024 * 1024)).toBe('1 MB/s')
  })
})

describe('formatTime', () => {
  it('formats 0 seconds correctly', () => {
    expect(formatTime(0)).toBe('0:00')
  })

  it('formats seconds less than a minute', () => {
    expect(formatTime(30)).toBe('0:30')
  })

  it('formats minutes and seconds', () => {
    expect(formatTime(90)).toBe('1:30')
  })

  it('formats multiple minutes', () => {
    expect(formatTime(3661)).toBe('61:01')
  })

  it('pads seconds with leading zero', () => {
    expect(formatTime(65)).toBe('1:05')
  })
})
