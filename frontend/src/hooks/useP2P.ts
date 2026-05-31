import { useState, useCallback } from 'react'
import { P2PAPI } from '../services/wails-api'
import { useWailsEvent } from './useWails'

export interface PeerInfo {
  id: string
  isHost: boolean
  connected: boolean
  lastSeen: string
}

// Константы для валидации
const MAX_ROOM_ID_LENGTH = 100
const MAX_PEER_ID_LENGTH = 100
const MAX_SDP_LENGTH = 10000

// Функция валидации roomID
function validateRoomID(roomID: unknown): string | null {
  if (typeof roomID !== 'string') {
    console.error('Invalid roomID: not a string')
    return null
  }
  if (roomID.length === 0 || roomID.length > MAX_ROOM_ID_LENGTH) {
    console.error('Invalid roomID: invalid length')
    return null
  }
  // Разрешаем только буквы, цифры, дефисы и подчеркивания
  if (!/^[a-zA-Z0-9_-]+$/.test(roomID)) {
    console.error('Invalid roomID: contains invalid characters')
    return null
  }
  return roomID
}

// Функция валидации PeerInfo
function validatePeerInfo(peer: unknown): PeerInfo | null {
  if (!peer || typeof peer !== 'object') {
    console.error('Invalid peer: not an object')
    return null
  }
  
  const p = peer as Record<string, unknown>
  
  // Валидация id
  if (typeof p.id !== 'string' || p.id.length === 0 || p.id.length > MAX_PEER_ID_LENGTH) {
    console.error('Invalid peer: invalid id')
    return null
  }
  if (!/^[a-zA-Z0-9_-]+$/.test(p.id)) {
    console.error('Invalid peer: id contains invalid characters')
    return null
  }
  
  // Валидация isHost
  if (typeof p.isHost !== 'boolean') {
    console.error('Invalid peer: isHost is not boolean')
    return null
  }
  
  // Валидация connected
  if (typeof p.connected !== 'boolean') {
    console.error('Invalid peer: connected is not boolean')
    return null
  }
  
  // Валидация lastSeen
  if (typeof p.lastSeen !== 'string' || p.lastSeen.length > 100) {
    console.error('Invalid peer: invalid lastSeen')
    return null
  }
  
  return {
    id: p.id,
    isHost: p.isHost,
    connected: p.connected,
    lastSeen: p.lastSeen
  }
}

// Функция валидации данных отключения пира
function validatePeerDisconnected(data: unknown): string | null {
  if (!data || typeof data !== 'object') {
    console.error('Invalid disconnect data: not an object')
    return null
  }
  
  const d = data as Record<string, unknown>
  
  if (typeof d.id !== 'string' || d.id.length === 0 || d.id.length > MAX_PEER_ID_LENGTH) {
    console.error('Invalid disconnect data: invalid id')
    return null
  }
  
  if (!/^[a-zA-Z0-9_-]+$/.test(d.id)) {
    console.error('Invalid disconnect data: id contains invalid characters')
    return null
  }
  
  return d.id
}

export function useP2P() {
  const [isHost, setIsHost] = useState(false)
  const [roomID, setRoomID] = useState('')
  const [peers, setPeers] = useState<PeerInfo[]>([])
  const [connected, setConnected] = useState(false)

  // Подписка на события P2P с валидацией
  useWailsEvent<{ roomID: string }>('p2p:room_created', (data) => {
    const validRoomID = validateRoomID(data?.roomID)
    if (!validRoomID) return
    
    setRoomID(validRoomID)
    setIsHost(true)
    setConnected(true)
  })

  useWailsEvent<{ roomID: string }>('p2p:room_joined', (data) => {
    const validRoomID = validateRoomID(data?.roomID)
    if (!validRoomID) return
    
    setRoomID(validRoomID)
    setIsHost(false)
    setConnected(true)
  })

  useWailsEvent<PeerInfo>('p2p:peer_connected', (peer) => {
    const validPeer = validatePeerInfo(peer)
    if (!validPeer) return
    
    setPeers(prev => [...prev.filter(p => p.id !== validPeer.id), validPeer])
  })

  useWailsEvent<{ id: string }>('p2p:peer_disconnected', (data) => {
    const validID = validatePeerDisconnected(data)
    if (!validID) return
    
    setPeers(prev => prev.filter(p => p.id !== validID))
  })

  const createRoom = useCallback(async () => {
    try {
      const id = await P2PAPI.createRoom()
      return id
    } catch (error) {
      console.error('Failed to create room:', error)
      throw error
    }
  }, [])

  const joinRoom = useCallback(async (id: string, offerSDP: string) => {
    // Валидация входных параметров
    const validID = validateRoomID(id)
    if (!validID) {
      throw new Error('Invalid room ID')
    }
    
    if (typeof offerSDP !== 'string' || offerSDP.length > MAX_SDP_LENGTH) {
      throw new Error('Invalid SDP')
    }
    
    try {
      const answerSDP = await P2PAPI.joinRoom(validID, offerSDP)
      return answerSDP
    } catch (error) {
      console.error('Failed to join room:', error)
      throw error
    }
  }, [])

  const disconnect = useCallback(async () => {
    try {
      await P2PAPI.disconnect()
      setConnected(false)
      setRoomID('')
      setPeers([])
    } catch (error) {
      console.error('Failed to disconnect:', error)
      throw error
    }
  }, [])

  return {
    isHost,
    roomID,
    peers,
    connected,
    createRoom,
    joinRoom,
    disconnect,
  }
}
