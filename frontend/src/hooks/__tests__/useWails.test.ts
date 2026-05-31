import { renderHook } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useWailsEvent, useWailsEmit } from '../useWails'

// Mock Wails runtime
const mockEventsOn = vi.fn()
const mockEventsEmit = vi.fn()

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: (...args: any[]) => mockEventsOn(...args),
  EventsEmit: (...args: any[]) => mockEventsEmit(...args),
}))

describe('useWailsEvent', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('subscribes to event on mount', () => {
    const callback = vi.fn()
    renderHook(() => useWailsEvent('test:event', callback))
    expect(mockEventsOn).toHaveBeenCalledWith('test:event', callback)
  })

  it('unsubscribes from event on unmount', () => {
    const unsubscribe = vi.fn()
    mockEventsOn.mockReturnValue(unsubscribe)
    const callback = vi.fn()
    const { unmount } = renderHook(() => useWailsEvent('test:event', callback))
    unmount()
    expect(unsubscribe).toHaveBeenCalled()
  })

  it('does not call unsubscribe if it is not a function', () => {
    mockEventsOn.mockReturnValue('not a function')
    const callback = vi.fn()
    const { unmount } = renderHook(() => useWailsEvent('test:event', callback))
    unmount()
    // Should not throw
  })
})

describe('useWailsEmit', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('returns emit function', () => {
    const { result } = renderHook(() => useWailsEmit())
    expect(typeof result.current).toBe('function')
  })

  it('calls EventsEmit with event name and data', () => {
    const { result } = renderHook(() => useWailsEmit())
    result.current('test:event', { data: 'test' })
    expect(mockEventsEmit).toHaveBeenCalledWith('test:event', { data: 'test' })
  })

  it('calls EventsEmit without data if not provided', () => {
    const { result } = renderHook(() => useWailsEmit())
    result.current('test:event')
    expect(mockEventsEmit).toHaveBeenCalledWith('test:event', undefined)
  })
})
