import '@testing-library/jest-dom'
import { vi } from 'vitest'

// Mock HTMLMediaElement methods not implemented in jsdom
window.HTMLMediaElement.prototype.pause = vi.fn()
window.HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined)
window.HTMLMediaElement.prototype.load = vi.fn()
