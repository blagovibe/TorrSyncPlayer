import { type RefObject, useCallback, useEffect, useMemo, useRef, useState } from "react";
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
  mediaLabel?: string | null;
  mediaKind?: "video" | "audio" | null;
  statusMessage?: string | null;
  canControlPlayback?: boolean;
  onPlaybackStart?: () => void;
}

interface AudioTrackSnapshot {
  id: string;
  label: string;
  language: string;
  enabled: boolean;
}

interface AudioTrackLike {
  id: string;
  label: string;
  language: string;
  enabled: boolean;
}

interface AudioTrackListLike extends ArrayLike<AudioTrackLike>, EventTarget {}

type VideoWithAudioTracks = HTMLVideoElement & {
  audioTracks?: AudioTrackListLike;
};

function VideoPlayer({
  videoRef: externalVideoRef,
  mediaLabel,
  mediaKind,
  statusMessage,
  canControlPlayback = true,
  onPlaybackStart,
}: VideoPlayerProps) {
  const internalVideoRef = useRef<HTMLVideoElement | null>(null);
  const videoRef = externalVideoRef ?? internalVideoRef;
  const hideTimerRef = useRef<number | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const [showControls, setShowControls] = useState(true);
  const [audioTracksSupported, setAudioTracksSupported] = useState(true);
  const [audioTracks, setAudioTracks] = useState<AudioTrackSnapshot[]>([]);
  const [selectedAudioTrackId, setSelectedAudioTrackId] = useState<string | null>(null);

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

  const togglePlay = useCallback(async () => {
    if (!canControlPlayback) {
      return;
    }

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
  }, [canControlPlayback, videoRef]);

  const syncAudioTracks = useCallback(() => {
    const video = videoRef.current;
    const trackList = video ? (video as VideoWithAudioTracks).audioTracks : undefined;

    if (!trackList) {
      setAudioTracksSupported(false);
      setAudioTracks([]);
      setSelectedAudioTrackId(null);
      return;
    }

    setAudioTracksSupported(true);

    const snapshot = Array.from(trackList).map((track, index) => ({
      id: track.id || `${index}`,
      label: track.label || `Audio ${index + 1}`,
      language: track.language || "",
      enabled: track.enabled,
    }));

    const activeTrack = snapshot.find((track) => track.enabled) ?? snapshot[0] ?? null;
    if (activeTrack && !activeTrack.enabled) {
      for (const track of Array.from(trackList)) {
        track.enabled = track.id === activeTrack.id;
      }
    }

    setAudioTracks(snapshot);
    setSelectedAudioTrackId(activeTrack?.id ?? null);
  }, [videoRef]);

  const activateAudioTrack = useCallback(
    (trackId: string) => {
      const video = videoRef.current;
      const trackList = video ? (video as VideoWithAudioTracks).audioTracks : undefined;
      if (!trackList) {
        return;
      }

      for (const track of Array.from(trackList)) {
        track.enabled = track.id === trackId;
      }

      setSelectedAudioTrackId(trackId);
      syncAudioTracks();
    },
    [syncAudioTracks, videoRef],
  );

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (!canControlPlayback) {
        return;
      }

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
  }, [canControlPlayback, togglePlay, videoRef]);

  useEffect(() => {
    setAudioTracks([]);
    setSelectedAudioTrackId(null);
    setAudioTracksSupported(true);
  }, [mediaLabel]);

  const hasSelectedMedia = Boolean(mediaLabel);

  return (
    <section className={`video-player ${mediaKind === "audio" ? "audio-mode" : "video-mode"}`} onMouseMove={resetHideTimer}>
      {!hasSelectedMedia && (
        <div className="video-placeholder">
          <div className="placeholder-art" />
          <p className="placeholder-title">Load a torrent to start playback</p>
          <p className="placeholder-copy">Video and audio files will appear in the library below.</p>
        </div>
      )}
        <video
        ref={videoRef as RefObject<HTMLVideoElement>}
        className="video-element"
        preload="auto"
        playsInline
        onClick={() => {
          if (canControlPlayback) {
            void togglePlay();
          }
        }}
        onTimeUpdate={(event) => setCurrentTime(event.currentTarget.currentTime)}
        onLoadedMetadata={(event) => {
          setDuration(event.currentTarget.duration);
          syncAudioTracks();
        }}
        onPause={() => setIsPlaying(false)}
        onPlay={() => {
          setIsPlaying(true);
          onPlaybackStart?.();
        }}
      />

      {hasSelectedMedia && (
        <div className="video-legend">
          <span className={`video-kind ${mediaKind ?? "video"}`}>
            {mediaKind === "audio" ? "Audio" : "Video"}
          </span>
          <span className="video-title">{mediaLabel}</span>
        </div>
      )}

      {hasSelectedMedia && (
          <div className="audio-track-panel">
          <div className="audio-track-panel-header">
            <span className="audio-track-title">Internal audio tracks</span>
            <span className="audio-track-status">
              {audioTracksSupported
                ? audioTracks.length > 0
                  ? `${audioTracks.length} available`
                  : "Waiting for media metadata"
                : "Not exposed by this runtime"}
            </span>
          </div>

          {audioTracksSupported ? (
            audioTracks.length > 0 ? (
              <div className="audio-track-list">
                {audioTracks.map((track) => (
                  <button
                    key={track.id}
                    type="button"
                    className={`audio-track-button ${selectedAudioTrackId === track.id ? "active" : ""}`}
                    onClick={() => activateAudioTrack(track.id)}
                    disabled={!canControlPlayback}
                  >
                    <span className="audio-track-name">{track.label}</span>
                    <span className="audio-track-meta">
                      {track.language ? track.language.toUpperCase() : "Unknown language"}
                    </span>
                  </button>
                ))}
              </div>
            ) : (
              <p className="audio-track-empty">
                No internal audio tracks are visible yet. Load a muxed MKV/MP4 and wait for media metadata.
              </p>
            )
          ) : (
            <p className="audio-track-empty">
              This browser/runtime does not expose <code>audioTracks</code>. Internal track switching will not work
              here.
            </p>
          )}
        </div>
      )}

      {statusMessage && (
        <div className="playback-message" role="status" aria-live="polite">
          {statusMessage}
        </div>
      )}

      <div className={`video-controls ${showControls ? "visible" : "hidden"}`}>
        <button type="button" onClick={() => void togglePlay()} disabled={!canControlPlayback}>
          {isPlaying ? "Pause" : "Play"}
        </button>

        <input
          type="range"
          min={0}
          max={duration || 0}
          step={0.1}
          value={currentTime}
          disabled={!canControlPlayback}
          onChange={(event) => {
            if (!canControlPlayback) {
              return;
            }
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
          disabled={!canControlPlayback}
          onClick={() => {
            if (!canControlPlayback) {
              return;
            }
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
