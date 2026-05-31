import React, { useState } from 'react'
import { useP2P } from '../hooks/useP2P'
import { useTorrent } from '../hooks/useTorrent'
import { validateMagnetURI, sanitizeInput } from '../utils/sanitize'
import './HomePage.css'

// SVG Icons as inline components
const PlayIcon: React.FC = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M8 5v14l11-7z" />
  </svg>
)

const UsersIcon: React.FC = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
  </svg>
)

const LinkIcon: React.FC = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18">
    <path d="M3.9 12c0-1.71 1.39-3.1 3.1-3.1h4V7H7c-2.76 0-5 2.24-5 5s2.24 5 5 5h4v-1.9H7c-1.71 0-3.1-1.39-3.1-3.1zM8 13h8v-2H8v2zm9-6h-4v1.9h4c1.71 0 3.1 1.39 3.1 3.1s-1.39 3.1-3.1 3.1h-4V17h4c2.76 0 5-2.24 5-5s-2.24-5-5-5z" />
  </svg>
)

const LockIcon: React.FC = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18">
    <path d="M18 8h-1V6c0-2.76-2.24-5-5-5S7 3.24 7 6v2H6c-1.1 0-2 .9-2 2v10c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V10c0-1.1-.9-2-2-2zm-6 9c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2zm3.1-9H8.9V6c0-1.71 1.39-3.1 3.1-3.1 1.71 0 3.1 1.39 3.1 3.1v2z" />
  </svg>
)

const KeyIcon: React.FC = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18">
    <path d="M12.65 10C11.83 7.67 9.61 6 7 6c-3.31 0-6 2.69-6 6s2.69 6 6 6c2.61 0 4.83-1.67 5.65-4H17v4h4v-4h2v-4H12.65zM7 14c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2z" />
  </svg>
)

export const HomePage: React.FC = () => {
  const { joinRoom } = useP2P()
  const { addTorrent } = useTorrent()
  const [magnetURI, setMagnetURI] = useState('')
  const [password, setPassword] = useState('')
  const [roomCode, setRoomCode] = useState('')
  const [mode, setMode] = useState<'select' | 'host' | 'guest'>('select')

  const handleAddTorrent = async () => {
    if (!magnetURI) return
    
    // Санитизация входных данных
    const sanitizedURI = sanitizeInput(magnetURI)
    if (!sanitizedURI) {
      console.error('Invalid magnet URI after sanitization')
      return
    }
    
    // Валидация формата magnet-ссылки
    if (!validateMagnetURI(sanitizedURI)) {
      console.error('Invalid magnet URI format')
      return
    }
    
    try {
      await addTorrent(sanitizedURI)
    } catch (error) {
      console.error('Failed to add torrent:', error)
    }
  }

  const handleJoinRoom = async () => {
    if (!roomCode) return
    try {
      await joinRoom(roomCode, password)
    } catch (error) {
      console.error('Failed to join room:', error)
    }
  }

  return (
    <div className="homepage">
      <div className="homepage-backdrop" />
      
      <div className="homepage-card">
        {/* Header */}
        <div className="homepage-header">
          <h1 className="homepage-title">TorrSyncPlayer</h1>
          <p className="homepage-subtitle">Смотрите вместе в реальном времени</p>
        </div>

        {/* Mode Selection */}
        {mode === 'select' && (
          <div className="homepage-section">
            <button 
              className="btn btn-primary btn-host" 
              onClick={() => setMode('host')}
            >
              <PlayIcon /> Создать комнату
            </button>
            
            <div className="divider">
              <span>или</span>
            </div>
            
            <button 
              className="btn btn-secondary btn-guest" 
              onClick={() => setMode('guest')}
            >
              <UsersIcon /> Присоединиться
            </button>
          </div>
        )}

        {/* Host Panel */}
        {mode === 'host' && (
          <div className="homepage-section">
            <h2 className="section-title">Создать комнату</h2>
            
            <div className="input-group">
              <div className="input-wrapper">
                <span className="input-icon"><LinkIcon /></span>
                <input
                  type="text"
                  placeholder="Вставьте магнет-ссылку или хеш..."
                  value={magnetURI}
                  onChange={(e) => setMagnetURI(e.target.value)}
                  className="homepage-input"
                />
              </div>
              
              <div className="input-wrapper">
                <span className="input-icon"><LockIcon /></span>
                <input
                  type="password"
                  placeholder="Пароль (необязательно)"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="homepage-input"
                />
              </div>
            </div>
            
            <button className="btn btn-primary btn-host" onClick={handleAddTorrent}>
              <PlayIcon /> Создать и смотреть
            </button>
            
            <button 
              className="btn btn-secondary" 
              onClick={() => setMode('select')}
              style={{ marginTop: 'var(--spacing-md)', background: 'transparent', border: 'none' }}
            >
              ← Назад
            </button>
          </div>
        )}

        {/* Guest Panel */}
        {mode === 'guest' && (
          <div className="homepage-section">
            <h2 className="section-title">Присоединиться к комнате</h2>
            
            <div className="input-group">
              <div className="input-wrapper">
                <span className="input-icon"><KeyIcon /></span>
                <input
                  type="text"
                  placeholder="ID комнаты"
                  value={roomCode}
                  onChange={(e) => setRoomCode(e.target.value)}
                  className="homepage-input"
                />
              </div>
            </div>
            
            <button 
              className="btn btn-secondary btn-guest" 
              onClick={handleJoinRoom}
            >
              <UsersIcon /> Присоединиться
            </button>
            
            <button 
              className="btn btn-secondary" 
              onClick={() => setMode('select')}
              style={{ marginTop: 'var(--spacing-md)', background: 'transparent', border: 'none' }}
            >
              ← Назад
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
