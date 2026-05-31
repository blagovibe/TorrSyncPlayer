import React, { useEffect, useState, useRef, useCallback } from 'react'
import { useP2P } from '../hooks/useP2P'
import { useTorrent } from '../hooks/useTorrent'
import { useSync } from '../hooks/useSync'
import { sanitizeInput } from '../utils/sanitize'
import './RoomPage.css'

// ===== SVG Иконки =====
const PlayIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M8 5v14l11-7z"/>
  </svg>
)

const PauseIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/>
  </svg>
)

const VolumeHighIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z"/>
  </svg>
)

const VolumeMuteIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M16.5 12c0-1.77-1.02-3.29-2.5-4.03v2.21l2.45 2.45c.03-.2.05-.41.05-.63zm2.5 0c0 .94-.2 1.82-.54 2.64l1.51 1.51C20.63 14.91 21 13.5 21 12c0-4.28-2.99-7.86-7-8.77v2.06c2.89.86 5 3.54 5 6.71zM4.27 3L3 4.27 7.73 9H3v6h4l5 5v-6.73l4.25 4.25c-.67.52-1.42.93-2.25 1.18v2.06c1.38-.31 2.63-.95 3.69-1.81L19.73 21 21 19.73l-9-9L4.27 3zM12 4L9.91 6.09 12 8.18V4z"/>
  </svg>
)

const FullscreenIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M7 14H5v5h5v-2H7v-3zm-2-4h2V7h3V5H5v5zm12 7h-3v2h5v-5h-2v3zM14 5v2h3v3h2V5h-5z"/>
  </svg>
)

const FullscreenExitIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z"/>
  </svg>
)

const ExitIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M10.09 15.59L11.5 17l5-5-5-5-1.41 1.41L12.67 11H3v2h9.67l-2.58 2.59zM19 3H5c-1.11 0-2 .9-2 2v4h2V5h14v14H5v-4H3v4c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2z"/>
  </svg>
)

const CopyIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" width="16" height="16">
    <path d="M16 1H4c-1.1 0-2 .9-2 2v14h2V3h12V1zm3 4H8c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h11c1.1 0 2-.9 2-2V7c0-1.1-.9-2-2-2zm0 16H8V7h11v14z"/>
  </svg>
)

const CloseIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" width="20" height="20">
    <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/>
  </svg>
)

const UsersIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor" width="18" height="18">
    <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/>
  </svg>
)

const DownloadIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z"/>
  </svg>
)

const PeersIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/>
  </svg>
)

const VideoFileIcon = () => (
  <svg viewBox="0 0 24 24" fill="currentColor">
    <path d="M18 4l2 4h-3l-2-4h-2l2 4h-3l-2-4H8l2 4H7L5 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V4h-4z"/>
  </svg>
)

// ===== Вспомогательные функции =====
const formatTime = (seconds: number): string => {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (h > 0) {
    return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
  }
  return `${m}:${s.toString().padStart(2, '0')}`
}

const formatSpeed = (bytesPerSecond: number): string => {
  if (bytesPerSecond < 1024) return `${bytesPerSecond.toFixed(0)} B/s`
  if (bytesPerSecond < 1024 * 1024) return `${(bytesPerSecond / 1024).toFixed(1)} KB/s`
  return `${(bytesPerSecond / (1024 * 1024)).toFixed(1)} MB/s`
}

// Генерация цвета для аватара
const getAvatarColor = (name: string): string => {
  const colors = [
    '#e50914', '#0071eb', '#46d369', '#f5c518', 
    '#ff6b6b', '#4ecdc4', '#45b7d1', '#96ceb4',
    '#ffeaa7', '#dfe6e9', '#fd79a8', '#a29bfe'
  ]
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash)
  }
  return colors[Math.abs(hash) % colors.length]
}

// ===== Типы =====
interface User {
  id: string
  name: string
  isHost: boolean
  connected: boolean
  color: string
}

export const RoomPage: React.FC = () => {
  const { roomID, peers, disconnect } = useP2P()
  const { currentTorrent, files, streamURL, loadFiles, getStreamUrl } = useTorrent()
  const { state, play, pause, seek } = useSync()
  
  const [selectedFile, setSelectedFile] = useState<string>('')
  const [isPlaying, setIsPlaying] = useState(false)
  const [currentTime, setCurrentTime] = useState(0)
  const [duration, setDuration] = useState(0)
  const [volume, setVolume] = useState(100)
  const [isMuted, setIsMuted] = useState(false)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [isPanelOpen, setIsPanelOpen] = useState(false)
  const [isOverlayVisible, setIsOverlayVisible] = useState(true)
  const [isCopied, setIsCopied] = useState(false)
  const [seekProgress, setSeekProgress] = useState(0)
  
  const videoRef = useRef<HTMLVideoElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const overlayTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  
  // Формируем список пользователей
  const users: User[] = [
    {
      id: 'host',
      name: 'Вы (Host)',
      isHost: true,
      connected: true,
      color: getAvatarColor('Host')
    },
    ...peers.map((peer, index) => ({
      id: peer.id || `peer-${index}`,
      name: `User ${index + 1}`,
      isHost: false,
      connected: true,
      color: getAvatarColor(peer.id || `peer-${index}`)
    }))
  ]

  // Загрузка файлов при изменении торрента
  useEffect(() => {
    if (currentTorrent) {
      loadFiles(currentTorrent.hash)
    }
  }, [currentTorrent, loadFiles])

  // Обработка выбора файла
  const handleFileSelect = async (filePath: string) => {
    setSelectedFile(filePath)
    if (currentTorrent) {
      await getStreamUrl(currentTorrent.hash, filePath)
    }
  }

  // Управление воспроизведением
  const togglePlay = useCallback(() => {
    if (isPlaying) {
      pause(state.position)
      setIsPlaying(false)
    } else {
      play(state.position)
      setIsPlaying(true)
    }
  }, [isPlaying, pause, play, state.position])

  // Обработка seek
  const handleSeek = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    const percent = (e.clientX - rect.left) / rect.width
    const newPosition = percent * duration
    seek(newPosition)
    setSeekProgress(percent * 100)
  }, [duration, seek])

  // Обновление времени воспроизведения
  useEffect(() => {
    const video = videoRef.current
    if (!video) return

    const handleTimeUpdate = () => {
      setCurrentTime(video.currentTime)
      if (duration > 0) {
        setSeekProgress((video.currentTime / duration) * 100)
      }
    }

    const handleLoadedMetadata = () => {
      setDuration(video.duration)
    }

    const handlePlay = () => setIsPlaying(true)
    const handlePause = () => setIsPlaying(false)

    video.addEventListener('timeupdate', handleTimeUpdate)
    video.addEventListener('loadedmetadata', handleLoadedMetadata)
    video.addEventListener('play', handlePlay)
    video.addEventListener('pause', handlePause)

    return () => {
      video.removeEventListener('timeupdate', handleTimeUpdate)
      video.removeEventListener('loadedmetadata', handleLoadedMetadata)
      video.removeEventListener('play', handlePlay)
      video.removeEventListener('pause', handlePause)
    }
  }, [duration])

  // Управление громкостью
  const handleVolumeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newVolume = parseInt(e.target.value)
    setVolume(newVolume)
    if (videoRef.current) {
      videoRef.current.volume = newVolume / 100
    }
    if (newVolume === 0) {
      setIsMuted(true)
    } else if (isMuted) {
      setIsMuted(false)
    }
  }

  const toggleMute = () => {
    if (isMuted) {
      setIsMuted(false)
      if (videoRef.current) {
        videoRef.current.volume = volume / 100
      }
    } else {
      setIsMuted(true)
      if (videoRef.current) {
        videoRef.current.volume = 0
      }
    }
  }

  // Полноэкранный режим
  const toggleFullscreen = async () => {
    if (!containerRef.current) return

    if (!isFullscreen) {
      if (containerRef.current.requestFullscreen) {
        await containerRef.current.requestFullscreen()
      }
    } else {
      if (document.exitFullscreen) {
        await document.exitFullscreen()
      }
    }
    setIsFullscreen(!isFullscreen)
  }

  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement)
    }
    document.addEventListener('fullscreenchange', handleFullscreenChange)
    return () => document.removeEventListener('fullscreenchange', handleFullscreenChange)
  }, [])

  // Автоматическое скрытие overlay
  const resetOverlayTimeout = useCallback(() => {
    setIsOverlayVisible(true)
    if (overlayTimeoutRef.current) {
      clearTimeout(overlayTimeoutRef.current)
    }
    overlayTimeoutRef.current = setTimeout(() => {
      if (isPlaying) {
        setIsOverlayVisible(false)
      }
    }, 3000)
  }, [isPlaying])

  useEffect(() => {
    resetOverlayTimeout()
    return () => {
      if (overlayTimeoutRef.current) {
        clearTimeout(overlayTimeoutRef.current)
      }
    }
  }, [resetOverlayTimeout])

  // Копирование ID комнаты
  const copyRoomId = async () => {
    try {
      await navigator.clipboard.writeText(roomID || '')
      setIsCopied(true)
      setTimeout(() => setIsCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }

  // Выход из комнаты
  const handleLeave = () => {
    disconnect()
  }

  // Получение имени выбранного файла
  const selectedFileName = sanitizeInput(files.find(f => f.path === selectedFile)?.path?.split('/').pop() || '')

  return (
    <div className="room-page">
      {/* Видеоплеер на весь экран */}
      <div 
        ref={containerRef}
        className="video-container"
        onMouseMove={resetOverlayTimeout}
        onClick={resetOverlayTimeout}
      >
        {streamURL ? (
          <video
            ref={videoRef}
            src={streamURL}
            autoPlay
            onClick={togglePlay}
            data-testid="room-video"
          />
        ) : (
          <div className="no-video">
            <div className="no-video-icon">
              <VideoFileIcon />
            </div>
            <p>Выберите файл для воспроизведения</p>
          </div>
        )}

        {/* Overlay с управлением */}
        <div className={`video-overlay ${isOverlayVisible ? 'visible' : ''}`}>
          {/* Верхняя часть */}
          <div className="overlay-top">
            <div className="room-info">
              <span className="room-id">Комната: {roomID?.slice(0, 8)}...</span>
              <button 
                className={`copy-btn ${isCopied ? 'copied' : ''}`}
                onClick={copyRoomId}
              >
                <CopyIcon />
                <span>{isCopied ? 'Скопировано!' : 'Копировать'}</span>
              </button>
            </div>
            <button className="leave-btn" onClick={handleLeave}>
              <ExitIcon />
              <span>Выйти</span>
            </button>
          </div>

          {/* Нижняя часть */}
          <div className="overlay-bottom">
            {/* Информация о торренте */}
            {currentTorrent && (
              <div className="torrent-info-card">
                <h4>Торрент</h4>
                <div className="torrent-file-name">{selectedFileName || sanitizeInput(currentTorrent.name)}</div>
                
                {/* Прогресс загрузки */}
                <div className="torrent-progress">
                  <div className="progress-bar">
                    <div
                      className="progress-fill"
                      style={{ width: `${(currentTorrent.progress || 0) * 100}%` }}
                    />
                  </div>
                  <span className="progress-text">
                    {((currentTorrent.progress || 0) * 100).toFixed(1)}% загружено
                  </span>
                  <div className="torrent-stats">
                    <div className="stat-item">
                      <DownloadIcon />
                      <span>{formatSpeed(currentTorrent.downloadSpeed || 0)}</span>
                    </div>
                    <div className="stat-item">
                      <PeersIcon />
                      <span>{currentTorrent.peers || 0} пиров</span>
                    </div>
                  </div>
                </div>
              </div>
            )}

            {/* Управление воспроизведением */}
            <div className="playback-controls">
              <button 
                className="control-btn play-btn"
                onClick={togglePlay}
              >
                {isPlaying ? <PauseIcon /> : <PlayIcon />}
              </button>

              <div className="seek-bar-container">
                <div className="seek-bar" onClick={handleSeek}>
                  <div 
                    className="progress-fill" 
                    style={{ width: `${seekProgress}%` }}
                  />
                </div>
                <span className="time-display">
                  {formatTime(currentTime)} / {formatTime(duration)}
                </span>
              </div>

              <div className="volume-control">
                <button className="control-btn" onClick={toggleMute}>
                  {isMuted || volume === 0 ? <VolumeMuteIcon /> : <VolumeHighIcon />}
                </button>
                <input
                  type="range"
                  className="volume-slider"
                  min="0"
                  max="100"
                  value={isMuted ? 0 : volume}
                  onChange={handleVolumeChange}
                />
              </div>

              <button className="control-btn" onClick={toggleFullscreen}>
                {isFullscreen ? <FullscreenExitIcon /> : <FullscreenIcon />}
              </button>
            </div>
          </div>
        </div>

        {/* Список файлов (если нет видео) */}
        {!streamURL && files.length > 0 && (
          <div className="file-list-overlay">
            <h4>Доступные файлы</h4>
            {files.map((file) => (
              <div
                key={file.path}
                className={`file-item ${selectedFile === file.path ? 'selected' : ''}`}
                onClick={() => handleFileSelect(file.path)}
              >
                <span className="file-item-name">{file.path.split('/').pop()}</span>
                <span className="file-item-progress">
                  {(file.progress * 100).toFixed(1)}%
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Кнопка открытия панели пользователей */}
      <button 
        className="panel-toggle"
        onClick={() => setIsPanelOpen(true)}
      >
        <UsersIcon />
        <span>Участники</span>
        <span className="badge">{users.length}</span>
      </button>

      {/* Боковая панель пользователей */}
      <div className={`users-panel ${isPanelOpen ? 'open' : ''}`}>
        <div className="panel-header">
          <h3>Участники ({users.length})</h3>
          <button 
            className="panel-close-btn"
            onClick={() => setIsPanelOpen(false)}
          >
            <CloseIcon />
          </button>
        </div>
        <div className="users-list">
          {users.map(user => (
            <div key={user.id} className="user-item">
              <div 
                className="user-avatar" 
                style={{ backgroundColor: user.color }}
              >
                {user.name.charAt(0).toUpperCase()}
              </div>
              <div className="user-info">
                <span className="user-name">{user.name}</span>
                <span className={`user-status ${user.isHost ? 'host' : ''}`}>
                  {user.isHost ? 'Host' : 'Guest'}
                </span>
              </div>
              <div 
                className={`status-dot ${user.connected ? 'online' : 'offline'}`}
              />
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
