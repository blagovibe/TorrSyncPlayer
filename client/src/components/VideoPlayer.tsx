import { type RefObject, useEffect, useMemo, useRef, useState } from "react";
import "./VideoPlayer.css";

const HIDE_DELAY_MS = 3000;

function formatTime(timeInSeconds: number): string {
  const safe = Number.isFinite(timeInSeconds) ? Math.max(0, timeInSeconds) : 0;
  const minutes = Math.floor(safe / 60);
  const seconds = Math.floor(safe % 60);
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

interface VideoPlayerProps {
  videoRef?: RefObject<HTMLVideoElement | null>;
}

function VideoPlayer({ videoRef: externalVideoRef }: VideoPlayerProps) {
  const internalVideoRef = useRef<HTMLVideoElement | null>(null);
  const videoRef = externalVideoRef ?? internalVideoRef;
  const hideTimerRef = useRef<number | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const [showControls, setShowControls] = useState(true);

  const progress = useMemo(() => {
    if (!duration) {
      return 0;
    }
    return (currentTime / duration) * 100;
  }, [currentTime, duration]);

  const resetHideTimer = () => {
    setShowControls(true);
    if (hideTimerRef.current) {
      window.clearTimeout(hideTimerRef.current);
    }
    hideTimerRef.current = window.setTimeout(() => {
      setShowControls(false);
    }, HIDE_DELAY_MS);
  };

  useEffect(() => {
    resetHideTimer();
    return () => {
      if (hideTimerRef.current) {
        window.clearTimeout(hideTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const video = videoRef.current;
      if (!video) {
        return;
      }

      if (event.code === "Space") {
        event.preventDefault();
        void togglePlay();
      }
      if (event.key.toLowerCase() === "f") {
        if (document.fullscreenElement) {
          void document.exitFullscreen();
        } else {
          void video.requestFullscreen();
        }
      }
      if (event.key === "ArrowRight") {
        video.currentTime = Math.min(video.duration || Infinity, video.currentTime + 5);
      }
      if (event.key === "ArrowLeft") {
        video.currentTime = Math.max(0, video.currentTime - 5);
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const togglePlay = async () => {
    const video = videoRef.current;
    if (!video) {
      return;
    }
    if (video.paused) {
      await video.play();
      setIsPlaying(true);
    } else {
      video.pause();
      setIsPlaying(false);
    }
  };

  return (
    <section className="video-player" onMouseMove={resetHideTimer}>
      <video
        ref={videoRef as RefObject<HTMLVideoElement>}
        className="video-element"
        onClick={() => void togglePlay()}
        onTimeUpdate={(event) => setCurrentTime(event.currentTarget.currentTime)}
        onLoadedMetadata={(event) => setDuration(event.currentTarget.duration)}
        onPause={() => setIsPlaying(false)}
        onPlay={() => setIsPlaying(true)}
      />

      <div className={`video-controls ${showControls ? "visible" : "hidden"}`}>
        <button type="button" onClick={() => void togglePlay()}>
          {isPlaying ? "Pause" : "Play"}
        </button>

        <input
          type="range"
          min={0}
          max={duration || 0}
          step={0.1}
          value={currentTime}
          onChange={(event) => {
            const value = Number(event.target.value);
            setCurrentTime(value);
            if (videoRef.current) {
              videoRef.current.currentTime = value;
            }
          }}
        />

        <span className="time-label">
          {formatTime(currentTime)} / {formatTime(duration)}
        </span>

        <input
          type="range"
          min={0}
          max={1}
          step={0.05}
          value={volume}
          onChange={(event) => {
            const value = Number(event.target.value);
            setVolume(value);
            if (videoRef.current) {
              videoRef.current.volume = value;
            }
          }}
          aria-label="Volume"
        />

        <button
          type="button"
          onClick={() => {
            const video = videoRef.current;
            if (!video) {
              return;
            }
            if (document.fullscreenElement) {
              void document.exitFullscreen();
            } else {
              void video.requestFullscreen();
            }
          }}
        >
          Fullscreen
        </button>
      </div>
      <div className="video-progress" style={{ width: `${progress}%` }} />
    </section>
  );
}

export default VideoPlayer;
