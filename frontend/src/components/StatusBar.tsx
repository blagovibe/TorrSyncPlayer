import React, { useState, useEffect } from 'react'
import { PeerInfo } from '../hooks/useP2P'
import './StatusBar.css'

interface StatusBarProps {
  isHost: boolean
  roomID: string
  peers: PeerInfo[]
  onDisconnect: () => void
  torrentProgress?: number
  downloadSpeed?: number
  peersCount?: number
  connectionStatus?: 'connected' | 'disconnected' | 'connecting'
}

// SVG иконки
const CrownIcon: React.FC = () => (
  <svg className="icon" viewBox="0 0 24 24" fill="currentColor">
    <path d="M5 16L3 5l5.5 5L12 4l3.5 6L21 5l-2 11H5zm14 3c0 .6-.4 1-1 1H6c-.6 0-1-.4-1-1v-1h14v1z"/>
  </svg>
)

const UserIcon: React.FC = () => (
  <svg className="icon" viewBox="0 0 24 24" fill="currentColor">
    <path d="M12 12c2.21 0 4-1.79 4-4s-1.79-4-4-4-4 1.79-4 4 1.79 4 4 4zm0 2c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z"/>
  </svg>
)

const CopyIcon: React.FC<{ className?: string }> = ({ className }) => (
  <svg className={className || "icon"} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <rect x="9" y="9" width="13" height="13" rx="2" ry="2"/>
    <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1"/>
  </svg>
)

const CheckIcon: React.FC = () => (
  <svg className="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <polyline points="20 6 9 17 4 12"/>
  </svg>
)

const DownloadIcon: React.FC = () => (
  <svg className="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4"/>
    <polyline points="7 10 12 15 17 10"/>
    <line x1="12" y1="15" x2="12" y2="3"/>
  </svg>
)

const UsersIcon: React.FC = () => (
  <svg className="icon" viewBox="0 0 24 24" fill="currentColor">
    <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/>
  </svg>
)

const ClockIcon: React.FC = () => (
  <svg className="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <circle cx="12" cy="12" r="10"/>
    <polyline points="12 6 12 12 16 14"/>
  </svg>
)

const DisconnectIcon: React.FC = () => (
  <svg className="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
    <path d="M18.36 6.64a9 9 0 11-12.73 0"/>
    <line x1="12" y1="2" x2="12" y2="12"/>
  </svg>
)

const formatTime = (seconds: number): string => {
  const hrs = Math.floor(seconds / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  const secs = seconds % 60
  return `${hrs.toString().padStart(2, '0')}:${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
}

export const StatusBar: React.FC<StatusBarProps> = ({
  isHost,
  roomID,
  peers,
  onDisconnect,
  torrentProgress = 0,
  downloadSpeed = 0,
  peersCount = 0,
  connectionStatus = 'connected',
}) => {
  const [copied, setCopied] = useState(false)
  const [sessionTime, setSessionTime] = useState(0)

  // Таймер сессии
  useEffect(() => {
    const interval = setInterval(() => {
      setSessionTime(prev => prev + 1)
    }, 1000)
    return () => clearInterval(interval)
  }, [])

  // Копирование ID комнаты
  const copyRoomId = async () => {
    try {
      await navigator.clipboard.writeText(roomID)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }

  const getStatusText = () => {
    switch (connectionStatus) {
      case 'connected': return 'Подключено'
      case 'disconnected': return 'Отключено'
      case 'connecting': return 'Подключение...'
      default: return 'Подключено'
    }
  }

  return (
    <div className="status-bar">
      {/* Левая секция */}
      <div className="status-left">
        {/* Индикатор подключения */}
        <div className="connection-status">
          <span className={`status-dot ${connectionStatus}`} />
          <span className="status-text">{getStatusText()}</span>
        </div>

        {/* Роль */}
        <div className={`role-badge ${isHost ? 'host' : 'guest'}`}>
          {isHost ? <CrownIcon /> : <UserIcon />}
          <span>{isHost ? 'Host' : 'Guest'}</span>
        </div>

        {/* ID комнаты */}
        <div className="room-id">
          <span className="label">Комната:</span>
          <span className="value">{roomID}</span>
          <button 
            className={`copy-btn ${copied ? 'copied' : ''}`} 
            onClick={copyRoomId}
            title="Копировать ID комнаты"
          >
            {copied ? <CheckIcon /> : <CopyIcon />}
          </button>
        </div>
      </div>

      {/* Центральная секция - информация о торренте */}
      <div className="status-center">
        <div className="torrent-info">
          <div className="torrent-progress-mini">
            <div 
              className="progress-fill" 
              style={{ width: `${Math.min(torrentProgress, 100)}%` }}
            />
          </div>
          <span className="torrent-stats">
            <DownloadIcon />
            {torrentProgress.toFixed(1)}% • {downloadSpeed.toFixed(1)} MB/s • {peersCount} пиров
          </span>
        </div>
      </div>

      {/* Правая секция */}
      <div className="status-right">
        {/* Пользователи */}
        <div className="users-count">
          <UsersIcon />
          <span>{peers.length}</span>
          
          {/* Выпадающий список участников */}
          {peers.length > 0 && (
            <div className="users-dropdown">
              <div className="users-dropdown-header">
                Участники ({peers.length})
              </div>
              {peers.map((peer, index) => (
                <div key={peer.id} className="users-dropdown-item">
                  <div className="user-avatar">
                    {peer.id.charAt(0).toUpperCase()}
                  </div>
                  <span>{peer.isHost ? 'Host' : `Guest ${index + 1}`}</span>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Время сессии */}
        <div className="session-time">
          <ClockIcon />
          <span className="time">{formatTime(sessionTime)}</span>
        </div>

        {/* Кнопка отключения */}
        <button className="disconnect-btn" onClick={onDisconnect}>
          <DisconnectIcon />
          <span>Отключиться</span>
        </button>
      </div>
    </div>
  )
}
