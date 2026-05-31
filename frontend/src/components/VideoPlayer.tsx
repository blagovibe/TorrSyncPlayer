import React, { useRef, useEffect, useState, useCallback } from 'react'
import { sanitizeInput } from '../utils/sanitize'
import './VideoPlayer.css'

interface VideoPlayerProps {
  src: string
  isPlaying: boolean
  position: number
  onPlay: () => void
  onPause: () => void
  onSeek: (position: number) => void
  title?: string
  roomId?: string
  viewers?: number
  torrentProgress?: number
  onBack?: () => void
}

// ===== SVG Иконки =====
const Icons = {
  Play: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M8 5v14l11-7z" />
    </svg>
  ),
  Pause: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" />
    </svg>
  ),
  Rewind10: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z" />
      <text x="9" y="16" fontSize="8" fill="currentColor" fontWeight="bold">10</text>
    </svg>
  ),
  Forward10: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 5V1l5 5-5 5V7c-3.31 0-6 2.69-6 6s2.69 6 6 6 6-2.69 6-6h2c0 4.42-3.58 8-8 8s-8-3.58-8-8 3.58-8 8-8z" />
      <text x="9" y="16" fontSize="8" fill="currentColor" fontWeight="bold">10</text>
    </svg>
  ),
  VolumeHigh: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z" />
    </svg>
  ),
  VolumeMute: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M16.5 12c0-1.77-1.02-3.29-2.5-4.03v2.21l2.45 2.45c.03-.2.05-.41.05-.63zm2.5 0c0 .94-.2 1.82-.54 2.64l1.51 1.51C20.63 14.91 21 13.5 21 12c0-4.28-2.99-7.86-7-8.77v2.06c2.89.86 5 3.54 5 6.71zM4.27 3L3 4.27 7.73 9H3v6h4l5 5v-6.73l4.25 4.25c-.67.52-1.42.93-2.25 1.18v2.06c1.38-.31 2.63-.95 3.69-1.81L19.73 21 21 19.73l-9-9L4.27 3zM12 4L9.91 6.09 12 8.18V4z" />
    </svg>
  ),
  Subtitles: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M20 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2zm0 14H4V6h16v12zM6 10h2v2H6zm0 4h8v2H6zm10 0h2v2h-2zm-6-4h8v2h-8z" />
    </svg>
  ),
  Settings: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M19.14 12.94c.04-.31.06-.63.06-.94 0-.31-.02-.63-.06-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.04.31-.06.63-.06.94s.02.63.06.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z" />
    </svg>
  ),
  Fullscreen: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M7 14H5v5h5v-2H7v-3zm-2-4h2V7h3V5H5v5zm12 7h-3v2h5v-5h-2v3zM14 5v2h3v3h2V5h-5z" />
    </svg>
  ),
  FullscreenExit: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z" />
    </svg>
  ),
  ArrowBack: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20v-2z" />
    </svg>
  ),
  Users: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z" />
    </svg>
  ),
  Error: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 15h-2v-2h2v2zm0-4h-2V7h2v6z" />
    </svg>
  ),
  Refresh: () => (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M17.65 6.35C16.2 4.9 14.21 4 12 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08c-.82 2.33-3.04 4-5.65 4-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z" />
    </svg>
  ),
}

export const VideoPlayer: React.FC<VideoPlayerProps> = ({
  src,
  isPlaying,
  position,
  onPlay,
  onPause,
  onSeek,
  title = 'Видео',
  roomId,
  viewers = 0,
  torrentProgress = 0,
  onBack,
}) => {
  const videoRef = useRef<HTMLVideoElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const hideControlsTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  
  const [duration, setDuration] = useState(0)
  const [currentTime, setCurrentTime] = useState(0)
  const [volume, setVolume] = useState(100)
  const [isMuted, setIsMuted] = useState(false)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [isBuffering, setIsBuffering] = useState(false)
  const [hasError, setHasError] = useState(false)
  const [errorMessage, setErrorMessage] = useState('')
  const [showControls, setShowControls] = useState(true)
  const [buffered, setBuffered] = useState(0)
  const [isPausedClicked, setIsPausedClicked] = useState(false)

  // Автоскрытие контролов
  const resetHideControlsTimeout = useCallback(() => {
    if (hideControlsTimeoutRef.current) {
      clearTimeout(hideControlsTimeoutRef.current)
    }
    setShowControls(true)
    
    if (isPlaying) {
      hideControlsTimeoutRef.current = setTimeout(() => {
        setShowControls(false)
      }, 3000)
    }
  }, [isPlaying])

  // Управление воспроизведением
  useEffect(() => {
    const video = videoRef.current
    if (!video) return

    if (isPlaying) {
      const playPromise = video.play()
      if (playPromise !== undefined) {
        playPromise.catch(() => {
          setHasError(true)
          setErrorMessage('Ошибка воспроизведения видео')
        })
      }
      setIsPausedClicked(false)
    } else {
      video.pause()
    }
  }, [isPlaying])

  // Синхронизация позиции
  useEffect(() => {
    const video = videoRef.current
    if (!video) return

    const timeDiff = Math.abs(video.currentTime - position)
    if (timeDiff > 0.5) {
      video.currentTime = position
    }
  }, [position])

  // Управление громкостью
  useEffect(() => {
    const video = videoRef.current
    if (!video) return
    video.volume = isMuted ? 0 : volume / 100
  }, [volume, isMuted])

  // Полноэкранный режим
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement)
    }
    document.addEventListener('fullscreenchange', handleFullscreenChange)
    return () => document.removeEventListener('fullscreenchange', handleFullscreenChange)
  }, [])

  // Очистка таймера при размонтировании
  useEffect(() => {
    return () => {
      if (hideControlsTimeoutRef.current) {
        clearTimeout(hideControlsTimeoutRef.current)
      }
    }
  }, [])

  // Обработчики событий видео
  const handleTimeUpdate = () => {
    const video = videoRef.current
    if (!video) return
    setCurrentTime(video.currentTime)
    
    // Обновление буферизации
    if (video.buffered.length > 0) {
      setBuffered(video.buffered.end(video.buffered.length - 1))
    }
  }

  const handleLoadedMetadata = () => {
    const video = videoRef.current
    if (!video) return
    setDuration(video.duration)
    setIsBuffering(false)
  }

  const handleWaiting = () => setIsBuffering(true)
  const handleCanPlay = () => setIsBuffering(false)

  const handleError = () => {
    const video = videoRef.current
    if (!video?.error) return
    
    let message = 'Произошла ошибка при воспроизведении'
    switch (video.error.code) {
      case 1:
        message = 'Воспроизведение прервано'
        break
      case 2:
        message = 'Ошибка сети. Проверьте подключение'
        break
      case 3:
        message = 'Ошибка декодирования видео'
        break
      case 4:
        message = 'Формат видео не поддерживается'
        break
    }
    setHasError(true)
    setErrorMessage(message)
    setIsBuffering(false)
  }

  // Управление
  const togglePlay = () => {
    if (isPlaying) {
      onPause()
      setIsPausedClicked(true)
    } else {
      onPlay()
      setIsPausedClicked(false)
    }
  }

  const handleProgressClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const container = e.currentTarget
    const rect = container.getBoundingClientRect()
    const percent = (e.clientX - rect.left) / rect.width
    const newTime = percent * duration
    onSeek(newTime)
  }

  const rewind10s = () => {
    const video = videoRef.current
    if (!video) return
    const newTime = Math.max(0, video.currentTime - 10)
    onSeek(newTime)
  }

  const forward10s = () => {
    const video = videoRef.current
    if (!video) return
    const newTime = Math.min(duration, video.currentTime + 10)
    onSeek(newTime)
  }

  const toggleMute = () => setIsMuted(!isMuted)

  const handleVolumeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newVolume = parseInt(e.target.value)
    setVolume(newVolume)
    if (newVolume === 0) {
      setIsMuted(true)
    } else if (isMuted) {
      setIsMuted(false)
    }
  }

  const toggleFullscreen = async () => {
    if (!containerRef.current) return
    
    if (!isFullscreen) {
      await containerRef.current.requestFullscreen()
    } else {
      await document.exitFullscreen()
    }
  }

  const handleRetry = () => {
    setHasError(false)
    setErrorMessage('')
    const video = videoRef.current
    if (video) {
      video.load()
    }
  }

  const handlePlayerClick = () => {
    if (!hasError) {
      togglePlay()
    }
  }

  // Форматирование времени
  const formatTime = (time: number) => {
    if (!time || isNaN(time)) return '0:00'
    const hours = Math.floor(time / 3600)
    const minutes = Math.floor((time % 3600) / 60)
    const seconds = Math.floor(time % 60)
    
    if (hours > 0) {
      return `${hours}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`
    }
    return `${minutes}:${seconds.toString().padStart(2, '0')}`
  }

  // Вычисление процентов для прогресс-баров
  const playbackProgress = duration > 0 ? (currentTime / duration) * 100 : 0
  const bufferProgress = duration > 0 ? (buffered / duration) * 100 : 0

  return (
    <div 
      ref={containerRef} 
      className="video-player"
      onMouseMove={resetHideControlsTimeout}
      onMouseLeave={() => isPlaying && setShowControls(false)}
    >
      {/* Видео элемент */}
      <video
        ref={videoRef}
        src={src}
        className="video-element"
        onTimeUpdate={handleTimeUpdate}
        onLoadedMetadata={handleLoadedMetadata}
        onWaiting={handleWaiting}
        onCanPlay={handleCanPlay}
        onError={handleError}
        onPlay={onPlay}
        onPause={onPause}
        onClick={handlePlayerClick}
        data-testid="video-element"
      />

      {/* Индикатор буферизации */}
      {isBuffering && !hasError && (
        <div className="buffering-overlay">
          <div className="spinner" />
          <span className="buffering-text">Загрузка...</span>
        </div>
      )}

      {/* Экран ошибки */}
      {hasError && (
        <div className="error-overlay">
          <div className="error-icon">
            <Icons.Error />
          </div>
          <h3 className="error-title">Ошибка воспроизведения</h3>
          <p className="error-message">{errorMessage}</p>
          <button className="retry-btn" onClick={handleRetry}>
            <Icons.Refresh />
            Повторить
          </button>
        </div>
      )}

      {/* Экран паузы (большой Play) */}
      {!isPlaying && !hasError && currentTime > 0 && isPausedClicked && (
        <div className="pause-overlay" onClick={togglePlay}>
          <div className="big-play-btn">
            <Icons.Play />
          </div>
        </div>
      )}

      {/* Верхняя панель */}
      <div className={`top-bar ${showControls ? 'visible' : ''}`}>
        {onBack && (
          <button className="back-btn" onClick={onBack}>
            <Icons.ArrowBack />
            <span>Назад</span>
          </button>
        )}
        <div className="video-info">
          <h2 className="video-title">{sanitizeInput(title)}</h2>
          {(roomId || viewers > 0) && (
            <span className="room-badge">
              <Icons.Users />
              {viewers} зрителей
            </span>
          )}
        </div>
      </div>

      {/* Нижняя панель управления */}
      <div className={`controls-bar ${showControls ? 'visible' : ''}`}>
        {/* Прогресс-бар */}
        <div 
          className="progress-container"
          onClick={handleProgressClick}
          data-testid="progress-container"
        >
          {/* Прогресс загрузки торрента */}
          <div className="progress-bar torrent-progress">
            <div 
              className="progress-fill" 
              style={{ width: `${torrentProgress}%` }}
            />
          </div>
          
          {/* Прогресс буферизации */}
          <div className="progress-bar buffer-progress">
            <div 
              className="progress-fill" 
              style={{ width: `${bufferProgress}%` }}
            />
          </div>
          
          {/* Прогресс воспроизведения */}
          <div className="progress-bar playback-progress">
            <div 
              className="progress-fill" 
              style={{ width: `${playbackProgress}%` }}
            />
            <div 
              className="progress-handle" 
              style={{ left: `${playbackProgress}%` }}
            />
          </div>
        </div>

        {/* Кнопки управления */}
        <div className="controls">
          <div className="controls-left">
            {/* Play/Pause */}
            <button 
              className="control-btn play-btn" 
              onClick={togglePlay}
              title={isPlaying ? 'Пауза' : 'Воспроизвести'}
            >
              {isPlaying ? <Icons.Pause /> : <Icons.Play />}
            </button>

            {/* Перемотка назад */}
            <button 
              className="control-btn" 
              onClick={rewind10s}
              title="Назад 10 сек"
            >
              <Icons.Rewind10 />
            </button>

            {/* Перемотка вперёд */}
            <button 
              className="control-btn" 
              onClick={forward10s}
              title="Вперёд 10 сек"
            >
              <Icons.Forward10 />
            </button>

            {/* Громкость */}
            <div className="volume-control">
              <button 
                className="control-btn" 
                onClick={toggleMute}
                title={isMuted ? 'Включить звук' : 'Выключить звук'}
              >
                {isMuted ? <Icons.VolumeMute /> : <Icons.VolumeHigh />}
              </button>
              <input
                type="range"
                min="0"
                max="100"
                value={isMuted ? 0 : volume}
                onChange={handleVolumeChange}
                className="volume-slider"
              />
            </div>

            {/* Время */}
            <span className="time-display">
              {formatTime(currentTime)} / {formatTime(duration)}
            </span>
          </div>

          <div className="controls-right">
            {/* Субтитры (заглушка) */}
            <button 
              className="control-btn" 
              title="Субтитры"
              onClick={() => {}}
            >
              <Icons.Subtitles />
            </button>

            {/* Настройки (заглушка) */}
            <button 
              className="control-btn" 
              title="Настройки"
              onClick={() => {}}
            >
              <Icons.Settings />
            </button>

            {/* Полноэкранный режим */}
            <button 
              className="control-btn" 
              onClick={toggleFullscreen}
              title={isFullscreen ? 'Выйти из полноэкранного режима' : 'Полноэкранный режим'}
            >
              {isFullscreen ? <Icons.FullscreenExit /> : <Icons.Fullscreen />}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
