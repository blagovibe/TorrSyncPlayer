import { renderHook, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useP2P } from '../useP2P'

// Mock Wails runtime
const mockEventsOn = vi.fn()
const mockEventsEmit = vi.fn()

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: (...args: any[]) => mockEventsOn(...args),
  EventsEmit: (...args: any[]) => mockEventsEmit(...args),
}))

// Mock P2PAPI
const mockCreateRoom = vi.fn()
const mockJoinRoom = vi.fn()
const mockDisconnect = vi.fn()

vi.mock('../../services/wails-api', () => ({
  P2PAPI: {
    createRoom: (...args: any[]) => mockCreateRoom(...args),
    joinRoom: (...args: any[]) => mockJoinRoom(...args),
    disconnect: (...args: any[]) => mockDisconnect(...args),
  },
}))

describe('useP2P', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Сбрасываем mock для EventsOn, чтобы он возвращать функцию писки
    mockEventsOn.mockReturnValue(vi.fn())
  })

  describe('Initial State', () => {
    it('returns initial state with default values', () => {
      const { result } = renderHook(() => useP2P())

      expect(result.current.isHost).toBe(false)
      expect(result.current.roomID).toBe('')
      expect(result.current.peers).toEqual([])
      expect(result.current.connected).toBe(false)
    })
  })

  describe('Room Events', () => {
    it('handles p2p:room_created event', () => {
      const { result } = renderHook(() => useP2P())

      // Находим обработчик для p2p:room_created
      const roomCreatedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:room_created'
      )?.[1]

      expect(roomCreatedHandler).toBeDefined()

      act(() => {
        roomCreatedHandler({ roomID: 'test-room-123' })
      })

      expect(result.current.roomID).toBe('test-room-123')
      expect(result.current.isHost).toBe(true)
      expect(result.current.connected).toBe(true)
    })

    it('handles p2p:room_joined event', () => {
      const { result } = renderHook(() => useP2P())

      const roomJoinedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:room_joined'
      )?.[1]

      expect(roomJoinedHandler).toBeDefined()

      act(() => {
        roomJoinedHandler({ roomID: 'joined-room-456' })
      })

      expect(result.current.roomID).toBe('joined-room-456')
      expect(result.current.isHost).toBe(false)
      expect(result.current.connected).toBe(true)
    })

    it('ignores p2p:room_created with invalid roomID (special characters)', () => {
      const { result } = renderHook(() => useP2P())

      const roomCreatedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:room_created'
      )?.[1]

      act(() => {
        roomCreatedHandler({ roomID: 'invalid room!@#' })
      })

      expect(result.current.roomID).toBe('')
      expect(result.current.connected).toBe(false)
    })

    it('ignores p2p:room_created with empty roomID', () => {
      const { result } = renderHook(() => useP2P())

      const roomCreatedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:room_created'
      )?.[1]

      act(() => {
        roomCreatedHandler({ roomID: '' })
      })

      expect(result.current.roomID).toBe('')
      expect(result.current.connected).toBe(false)
    })

    it('ignores p2p:room_created with non-string roomID', () => {
      const { result } = renderHook(() => useP2P())

      const roomCreatedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:room_created'
      )?.[1]

      act(() => {
        roomCreatedHandler({ roomID: 12345 })
      })

      expect(result.current.roomID).toBe('')
      expect(result.current.connected).toBe(false)
    })

    it('ignores p2p:room_created with too long roomID', () => {
      const { result } = renderHook(() => useP2P())

      const roomCreatedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:room_created'
      )?.[1]

      act(() => {
        roomCreatedHandler({ roomID: 'a'.repeat(101) })
      })

      expect(result.current.roomID).toBe('')
      expect(result.current.connected).toBe(false)
    })
  })

  describe('Peer Events', () => {
    it('handles p2p:peer_connected event', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      expect(peerConnectedHandler).toBeDefined()

      const validPeer = {
        id: 'peer-123',
        isHost: false,
        connected: true,
        lastSeen: '2024-01-01T00:00:00Z',
      }

      act(() => {
        peerConnectedHandler(validPeer)
      })

      expect(result.current.peers).toHaveLength(1)
      expect(result.current.peers[0]).toEqual(validPeer)
    })

    it('updates existing peer on p2p:peer_connected event', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      const peer1 = {
        id: 'peer-123',
        isHost: false,
        connected: true,
        lastSeen: '2024-01-01T00:00:00Z',
      }

      const peer1Updated = {
        id: 'peer-123',
        isHost: false,
        connected: true,
        lastSeen: '2024-01-01T01:00:00Z',
      }

      act(() => {
        peerConnectedHandler(peer1)
      })

      act(() => {
        peerConnectedHandler(peer1Updated)
      })

      expect(result.current.peers).toHaveLength(1)
      expect(result.current.peers[0]).toEqual(peer1Updated)
    })

    it('ignores p2p:peer_connected with invalid peer (missing id)', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      act(() => {
        peerConnectedHandler({
          isHost: false,
          connected: true,
          lastSeen: '2024-01-01T00:00:00Z',
        })
      })

      expect(result.current.peers).toHaveLength(0)
    })

    it('ignores p2p:peer_connected with invalid peer (non-boolean isHost)', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      act(() => {
        peerConnectedHandler({
          id: 'peer-123',
          isHost: 'yes',
          connected: true,
          lastSeen: '2024-01-01T00:00:00Z',
        })
      })

      expect(result.current.peers).toHaveLength(0)
    })

    it('ignores p2p:peer_connected with invalid peer (special characters in id)', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      act(() => {
        peerConnectedHandler({
          id: 'peer-123!@#',
          isHost: false,
          connected: true,
          lastSeen: '2024-01-01T00:00:00Z',
        })
      })

      expect(result.current.peers).toHaveLength(0)
    })

    it('handles p2p:peer_disconnected event', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      const peerDisconnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_disconnected'
      )?.[1]

      const validPeer = {
        id: 'peer-123',
        isHost: false,
        connected: true,
        lastSeen: '2024-01-01T00:00:00Z',
      }

      act(() => {
        peerConnectedHandler(validPeer)
      })

      expect(result.current.peers).toHaveLength(1)

      act(() => {
        peerDisconnectedHandler({ id: 'peer-123' })
      })

      expect(result.current.peers).toHaveLength(0)
    })

    it('ignores p2p:peer_disconnected with invalid id', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      const peerDisconnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_disconnected'
      )?.[1]

      const validPeer = {
        id: 'peer-123',
        isHost: false,
        connected: true,
        lastSeen: '2024-01-01T00:00:00Z',
      }

      act(() => {
        peerConnectedHandler(validPeer)
      })

      act(() => {
        peerDisconnectedHandler({ id: 'invalid id!@#' })
      })

      expect(result.current.peers).toHaveLength(1)
    })
  })

  describe('createRoom', () => {
    it('calls P2PAPI.createRoom and returns room ID', async () => {
      mockCreateRoom.mockResolvedValue('new-room-123')

      const { result } = renderHook(() => useP2P())

      let roomID: string | undefined
      await act(async () => {
        roomID = await result.current.createRoom()
      })

      expect(mockCreateRoom).toHaveBeenCalled()
      expect(roomID).toBe('new-room-123')
    })

    it('throws error when P2PAPI.createRoom fails', async () => {
      mockCreateRoom.mockRejectedValue(new Error('Connection failed'))

      const { result } = renderHook(() => useP2P())

      await expect(
        act(async () => {
          await result.current.createRoom()
        })
      ).rejects.toThrow('Connection failed')
    })
  })

  describe('joinRoom', () => {
    it('calls P2PAPI.joinRoom with valid parameters', async () => {
      const mockAnswerSDP = 'v=0\r\n...'
      mockJoinRoom.mockResolvedValue(mockAnswerSDP)

      const { result } = renderHook(() => useP2P())

      let answerSDP: string | undefined
      await act(async () => {
        answerSDP = await result.current.joinRoom('valid-room', 'offer-sdp')
      })

      expect(mockJoinRoom).toHaveBeenCalledWith('valid-room', 'offer-sdp')
      expect(answerSDP).toBe(mockAnswerSDP)
    })

    it('throws error for invalid room ID', async () => {
      const { result } = renderHook(() => useP2P())

      await expect(
        act(async () => {
          await result.current.joinRoom('invalid room!@#', 'offer-sdp')
        })
      ).rejects.toThrow('Invalid room ID')
    })

    it('throws error for empty room ID', async () => {
      const { result } = renderHook(() => useP2P())

      await expect(
        act(async () => {
          await result.current.joinRoom('', 'offer-sdp')
        })
      ).rejects.toThrow('Invalid room ID')
    })

    it('throws error for too long SDP', async () => {
      const { result } = renderHook(() => useP2P())

      await expect(
        act(async () => {
          await result.current.joinRoom('valid-room', 'a'.repeat(10001))
        })
      ).rejects.toThrow('Invalid SDP')
    })

    it('throws error when P2PAPI.joinRoom fails', async () => {
      mockJoinRoom.mockRejectedValue(new Error('Join failed'))

      const { result } = renderHook(() => useP2P())

      await expect(
        act(async () => {
          await result.current.joinRoom('valid-room', 'offer-sdp')
        })
      ).rejects.toThrow('Join failed')
    })
  })

  describe('disconnect', () => {
    it('calls P2PAPI.disconnect and resets state', async () => {
      mockDisconnect.mockResolvedValue(undefined)

      const { result } = renderHook(() => useP2P())

      // Устанавливаем начальное состояние
      const roomCreatedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:room_created'
      )?.[1]

      act(() => {
        roomCreatedHandler({ roomID: 'test-room' })
      })

      expect(result.current.connected).toBe(true)
      expect(result.current.roomID).toBe('test-room')

      await act(async () => {
        await result.current.disconnect()
      })

      expect(mockDisconnect).toHaveBeenCalled()
      expect(result.current.connected).toBe(false)
      expect(result.current.roomID).toBe('')
      expect(result.current.peers).toEqual([])
    })

    it('throws error when P2PAPI.disconnect fails', async () => {
      mockDisconnect.mockRejectedValue(new Error('Disconnect failed'))

      const { result } = renderHook(() => useP2P())

      await expect(
        act(async () => {
          await result.current.disconnect()
        })
      ).rejects.toThrow('Disconnect failed')
    })
  })

  describe('Room ID Validation', () => {
    it('accepts valid room IDs with letters, numbers, hyphens, and underscores', () => {
      const { result } = renderHook(() => useP2P())

      const roomCreatedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:room_created'
      )?.[1]

      const validIDs = [
        'room123',
        'ROOM-456',
        'my_room',
        'room-with-multiple-parts',
        'a',
        'A1_b2-C3',
      ]

      validIDs.forEach((id) => {
        act(() => {
          roomCreatedHandler({ roomID: id })
        })
        expect(result.current.roomID).toBe(id)
      })
    })

    it('rejects room IDs with spaces', () => {
      const { result } = renderHook(() => useP2P())

      const roomCreatedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:room_created'
      )?.[1]

      act(() => {
        roomCreatedHandler({ roomID: 'room with spaces' })
      })

      expect(result.current.roomID).toBe('')
    })

    it('rejects room IDs with special characters', () => {
      const { result } = renderHook(() => useP2P())

      const roomCreatedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:room_created'
      )?.[1]

      const invalidIDs = [
        'room@123',
        'room#456',
        'room$test',
        'room%percent',
        'room^caret',
        'room&ampersand',
        'room*star',
        'room(paren',
      ]

      invalidIDs.forEach((id) => {
        act(() => {
          roomCreatedHandler({ roomID: id })
        })
        expect(result.current.roomID).toBe('')
      })
    })
  })

  describe('PeerInfo Validation', () => {
    it('accepts valid peer info', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      const validPeers = [
        { id: 'peer1', isHost: true, connected: true, lastSeen: '2024-01-01' },
        { id: 'peer-2', isHost: false, connected: false, lastSeen: '2024-06-15T12:00:00Z' },
        { id: 'peer_3', isHost: false, connected: true, lastSeen: '' },
      ]

      validPeers.forEach((peer) => {
        act(() => {
          peerConnectedHandler(peer)
        })
      })

      expect(result.current.peers).toHaveLength(3)
    })

    it('rejects peer with missing required fields', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      const invalidPeers = [
        { isHost: true, connected: true, lastSeen: '2024-01-01' }, // missing id
        { id: 'peer1', connected: true, lastSeen: '2024-01-01' }, // missing isHost
        { id: 'peer1', isHost: true, lastSeen: '2024-01-01' }, // missing connected
        { id: 'peer1', isHost: true, connected: true }, // missing lastSeen
      ]

      invalidPeers.forEach((peer) => {
        act(() => {
          peerConnectedHandler(peer)
        })
      })

      expect(result.current.peers).toHaveLength(0)
    })

    it('rejects peer with wrong field types', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      const invalidPeers = [
        { id: 123, isHost: true, connected: true, lastSeen: '2024-01-01' }, // id is number
        { id: 'peer1', isHost: 'yes', connected: true, lastSeen: '2024-01-01' }, // isHost is string
        { id: 'peer1', isHost: true, connected: 1, lastSeen: '2024-01-01' }, // connected is number
        { id: 'peer1', isHost: true, connected: true, lastSeen: 12345 }, // lastSeen is number
      ]

      invalidPeers.forEach((peer) => {
        act(() => {
          peerConnectedHandler(peer)
        })
      })

      expect(result.current.peers).toHaveLength(0)
    })

    it('rejects peer with too long id', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      act(() => {
        peerConnectedHandler({
          id: 'a'.repeat(101),
          isHost: false,
          connected: true,
          lastSeen: '2024-01-01',
        })
      })

      expect(result.current.peers).toHaveLength(0)
    })

    it('rejects peer with too long lastSeen', () => {
      const { result } = renderHook(() => useP2P())

      const peerConnectedHandler = mockEventsOn.mock.calls.find(
        (call) => call[0] === 'p2p:peer_connected'
      )?.[1]

      act(() => {
        peerConnectedHandler({
          id: 'peer1',
          isHost: false,
          connected: true,
          lastSeen: 'a'.repeat(101),
        })
      })

      expect(result.current.peers).toHaveLength(0)
    })
  })
})
