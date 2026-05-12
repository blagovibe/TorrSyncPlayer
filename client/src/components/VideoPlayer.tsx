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
  canControlAudioTracks?: boolean;
  selectedAudioTrackIndex?: number | null;
  onPlaybackStart?: () => void;
  onAudioTrackChange?: (trackIndex: number | null) => void;
  onPlayerReady?: (ready: boolean) => void;
}

interface AudioTrackSnapshot {
  sourceIndex: number;
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
  canControlAudioTracks = canControlPlayback,
  selectedAudioTrackIndex = null,
  onPlaybackStart,
  onAudioTrackChange,
  onPlayerReady,
}: VideoPlayerProps) {
  const internalVideoRef = useRef<HTMLVideoElement | null>(null);
  const videoRef = externalVideoRef ?? internalVideoRef;
  const hideTimerRef = useRef<number | null>(null);
  const onPlayerReadyRef = useRef(onPlayerReady);
  const onAudioTrackChangeRef = useRef(onAudioTrackChange);
  const hasMediaMetadataRef = useRef(false);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const [showControls, setShowControls] = useState(true);
  const [audioTracksSupported, setAudioTracksSupported] = useState(true);
  const [audioTracks, setAudioTracks] = useState<AudioTrackSnapshot[]>([]);

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
    onPlayerReadyRef.current = onPlayerReady;
  }, [onPlayerReady]);

  useEffect(() => {
    onAudioTrackChangeRef.current = onAudioTrackChange;
  }, [onAudioTrackChange]);

  useEffect(() => {
    resetHideTimer();
    return () => {
      if (hideTimerRef.current) {
        window.clearTimeout(hideTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    onPlayerReadyRef.current?.(true);
    return () => onPlayerReadyRef.current?.(false);
  }, []);

  const activeAudioTrackIndex = useMemo(() => {
    const selectedSnapshotTrack = audioTracks.find((track) => track.sourceIndex === selectedAudioTrackIndex);
    if (selectedSnapshotTrack) {
      return selectedSnapshotTrack.sourceIndex;
    }

    const enabledSnapshotTrack = audioTracks.find((track) => track.enabled) ?? audioTracks[0] ?? null;
    return enabledSnapshotTrack?.sourceIndex ?? null;
  }, [audioTracks, selectedAudioTrackIndex]);

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

    if (!mediaLabel) {
      setAudioTracks([]);
      return;
    }

    if (!hasMediaMetadataRef.current) {
      setAudioTracks([]);
      return;
    }

    if (!trackList) {
      setAudioTracksSupported(false);
      setAudioTracks([]);
      return;
    }

    setAudioTracksSupported(true);

    const snapshot = Array.from(trackList).map((track, index) => ({
      sourceIndex: index,
      label: track.label || `Audio ${index + 1}`,
      language: track.language || "",
      enabled: track.enabled,
    }));

    const requestedTrack =
      selectedAudioTrackIndex !== null
        ? snapshot.find((track) => track.sourceIndex === selectedAudioTrackIndex) ?? null
        : null;
    const enabledTrack = snapshot.find((track) => track.enabled) ?? snapshot[0] ?? null;
    const resolvedTrack = requestedTrack ?? enabledTrack;

    if (resolvedTrack) {
      for (const [index, track] of Array.from(trackList).entries()) {
        track.enabled = index === resolvedTrack.sourceIndex;
      }
    }

    const normalizedSnapshot = snapshot.map((track) => ({
      ...track,
      enabled: resolvedTrack ? track.sourceIndex === resolvedTrack.sourceIndex : track.enabled,
    }));
    setAudioTracks(normalizedSnapshot);

    const resolvedTrackIndex = resolvedTrack?.sourceIndex ?? null;
    if (resolvedTrackIndex !== selectedAudioTrackIndex) {
      onAudioTrackChangeRef.current?.(resolvedTrackIndex);
    }
  }, [mediaLabel, selectedAudioTrackIndex, videoRef]);

  const activateAudioTrack = useCallback(
    (trackIndex: number) => {
      const video = videoRef.current;
      const trackList = video ? (video as VideoWithAudioTracks).audioTracks : undefined;
      if (!trackList) {
        return;
      }

      const selectedTrack = audioTracks.find((track) => track.sourceIndex === trackIndex);
      if (!selectedTrack) {
        return;
      }

      for (const [index, track] of Array.from(trackList).entries()) {
        track.enabled = index === selectedTrack.sourceIndex;
      }

      setAudioTracks(
        audioTracks.map((track) => ({
          ...track,
          enabled: track.sourceIndex === selectedTrack.sourceIndex,
        })),
      );
      onAudioTrackChangeRef.current?.(selectedTrack.sourceIndex);
    },
    [audioTracks, videoRef],
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
    hasMediaMetadataRef.current = false;
    setAudioTracks([]);
    setAudioTracksSupported(true);
  }, [mediaLabel]);

  useEffect(() => {
    if (audioTracksSupported) {
      syncAudioTracks();
    }
  }, [audioTracksSupported, syncAudioTracks]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) {
      return;
    }

    video.defaultMuted = false;
    video.muted = false;
    video.volume = volume;
  }, [mediaLabel, videoRef, volume]);

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
          hasMediaMetadataRef.current = true;
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
                    key={track.sourceIndex}
                    type="button"
                    className={`audio-track-button ${activeAudioTrackIndex === track.sourceIndex ? "active" : ""}`}
                    onClick={() => activateAudioTrack(track.sourceIndex)}
                    disabled={!canControlAudioTracks}
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
