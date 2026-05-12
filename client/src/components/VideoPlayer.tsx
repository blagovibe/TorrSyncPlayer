import { type RefObject, useCallback, useEffect, useMemo, useRef, useState } from "react";
import "./VideoPlayer.css";
import type { AudioTrackInfo } from "../services/types";

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
  fallbackAudioTracks?: AudioTrackInfo[];
  selectedAudioTrackIndex?: number | null;
  onPlaybackStart?: () => void;
  onAudioTrackChange?: (trackIndex: number | null) => void;
  resolveFallbackAudioTrackSource?: (trackIndex: number, startSeconds: number) => Promise<string | null>;
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

function detectAudioTracksSupport(): boolean {
  if (typeof document === "undefined") {
    return false;
  }

  return "audioTracks" in document.createElement("video");
}

function VideoPlayer({
  videoRef: externalVideoRef,
  mediaLabel,
  mediaKind,
  statusMessage,
  canControlPlayback = true,
  canControlAudioTracks = canControlPlayback,
  fallbackAudioTracks = [],
  selectedAudioTrackIndex = null,
  onPlaybackStart,
  onAudioTrackChange,
  resolveFallbackAudioTrackSource,
  onPlayerReady,
}: VideoPlayerProps) {
  const internalVideoRef = useRef<HTMLVideoElement | null>(null);
  const fallbackAudioRef = useRef<HTMLAudioElement | null>(null);
  const videoRef = externalVideoRef ?? internalVideoRef;
  const hideTimerRef = useRef<number | null>(null);
  const onPlayerReadyRef = useRef(onPlayerReady);
  const onAudioTrackChangeRef = useRef(onAudioTrackChange);
  const fallbackAudioRequestIdRef = useRef(0);
  const hasMediaMetadataRef = useRef(false);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const [showControls, setShowControls] = useState(true);
  const [audioTracksSupported, setAudioTracksSupported] = useState(() => detectAudioTracksSupport());
  const [audioTracks, setAudioTracks] = useState<AudioTrackSnapshot[]>([]);
  const [fallbackAudioSourceUrl, setFallbackAudioSourceUrl] = useState<string | null>(null);

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

  const fallbackAudioTrackSnapshots = useMemo(
    () =>
      fallbackAudioTracks.map((track, index) => ({
        sourceIndex: index,
        label: track.label || `Audio ${index + 1}`,
        language: track.language || "",
        enabled: false,
      })),
    [fallbackAudioTracks],
  );

  const usingFallbackAudio =
    !audioTracksSupported &&
    fallbackAudioTrackSnapshots.length > 0 &&
    typeof resolveFallbackAudioTrackSource === "function";

  const visibleAudioTracks = useMemo(
    () => (audioTracksSupported ? audioTracks : usingFallbackAudio ? fallbackAudioTrackSnapshots : []),
    [audioTracks, audioTracksSupported, fallbackAudioTrackSnapshots, usingFallbackAudio],
  );

  const activeAudioTrackIndex = useMemo(() => {
    const selectedSnapshotTrack = visibleAudioTracks.find((track) => track.sourceIndex === selectedAudioTrackIndex);
    if (selectedSnapshotTrack) {
      return selectedSnapshotTrack.sourceIndex;
    }

    const enabledSnapshotTrack = visibleAudioTracks.find((track) => track.enabled) ?? visibleAudioTracks[0] ?? null;
    return enabledSnapshotTrack?.sourceIndex ?? null;
  }, [selectedAudioTrackIndex, visibleAudioTracks]);

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
      if (usingFallbackAudio && fallbackAudioRef.current && fallbackAudioSourceUrl) {
        void fallbackAudioRef.current.play().catch(() => undefined);
      }
      setIsPlaying(true);
    } else {
      video.pause();
      fallbackAudioRef.current?.pause();
      setIsPlaying(false);
    }
  }, [canControlPlayback, fallbackAudioSourceUrl, usingFallbackAudio, videoRef]);

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
      if (trackList) {
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
        return;
      }

      if (!usingFallbackAudio) {
        return;
      }

      const selectedTrack = fallbackAudioTrackSnapshots.find((track) => track.sourceIndex === trackIndex);
      if (!selectedTrack) {
        return;
      }

      onAudioTrackChangeRef.current?.(selectedTrack.sourceIndex);
    },
    [audioTracks, fallbackAudioTrackSnapshots, usingFallbackAudio, videoRef],
  );

  const requestFallbackAudioSource = useCallback(
    async (startSeconds: number) => {
      if (!usingFallbackAudio || activeAudioTrackIndex === null || !resolveFallbackAudioTrackSource) {
        fallbackAudioRequestIdRef.current += 1;
        setFallbackAudioSourceUrl(null);
        fallbackAudioRef.current?.pause();
        return;
      }

      const requestId = ++fallbackAudioRequestIdRef.current;
      const audio = fallbackAudioRef.current;
      audio?.pause();
      setFallbackAudioSourceUrl(null);

      try {
        const nextSourceUrl = await resolveFallbackAudioTrackSource(activeAudioTrackIndex, Math.max(0, startSeconds));
        if (requestId !== fallbackAudioRequestIdRef.current) {
          return;
        }

        setFallbackAudioSourceUrl(nextSourceUrl);
      } catch {
        if (requestId === fallbackAudioRequestIdRef.current) {
          setFallbackAudioSourceUrl(null);
        }
      }
    },
    [activeAudioTrackIndex, resolveFallbackAudioTrackSource, usingFallbackAudio],
  );

  useEffect(() => {
    if (!usingFallbackAudio) {
      fallbackAudioRequestIdRef.current += 1;
      setFallbackAudioSourceUrl(null);
      fallbackAudioRef.current?.pause();
      return;
    }

    if (activeAudioTrackIndex !== null && selectedAudioTrackIndex !== activeAudioTrackIndex) {
      onAudioTrackChangeRef.current?.(activeAudioTrackIndex);
      return;
    }

    void requestFallbackAudioSource(videoRef.current?.currentTime ?? 0);
  }, [activeAudioTrackIndex, requestFallbackAudioSource, selectedAudioTrackIndex, usingFallbackAudio, videoRef]);

  useEffect(() => {
    const audio = fallbackAudioRef.current;
    if (!audio) {
      return;
    }

    audio.volume = volume;
    audio.muted = false;

    if (!usingFallbackAudio) {
      audio.pause();
      audio.removeAttribute("src");
      audio.load();
      return;
    }

    if (!fallbackAudioSourceUrl) {
      audio.pause();
      audio.removeAttribute("src");
      audio.load();
      return;
    }

    audio.load();
    if (videoRef.current && !videoRef.current.paused) {
      void audio.play().catch(() => undefined);
    }
  }, [fallbackAudioSourceUrl, usingFallbackAudio, videoRef, volume]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) {
      return;
    }

    if (usingFallbackAudio) {
      video.defaultMuted = true;
      video.muted = true;
      video.volume = 0;
      return;
    }

    video.defaultMuted = false;
    video.muted = false;
    video.volume = volume;
  }, [usingFallbackAudio, videoRef, volume]);

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
    setAudioTracksSupported(detectAudioTracksSupport());
    setFallbackAudioSourceUrl(null);
    fallbackAudioRequestIdRef.current += 1;
    fallbackAudioRef.current?.pause();
  }, [mediaLabel]);

  useEffect(() => {
    if (audioTracksSupported) {
      syncAudioTracks();
    }
  }, [audioTracksSupported, syncAudioTracks]);

  const hasSelectedMedia = Boolean(mediaLabel);
  const audioTrackStatusText = audioTracksSupported
    ? audioTracks.length > 0
      ? `${audioTracks.length} available`
      : "Waiting for media metadata"
    : usingFallbackAudio
      ? `${fallbackAudioTrackSnapshots.length} available via ffmpeg`
      : "Unavailable in this runtime";
  const audioTracksUnavailableMessage = (
    <>
      This runtime does not expose <code>audioTracks</code>, and no ffmpeg fallback track is available for this file.
    </>
  );

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
        onSeeked={(event) => {
          if (usingFallbackAudio) {
            void requestFallbackAudioSource(event.currentTarget.currentTime);
          }
        }}
        onPause={() => {
          fallbackAudioRef.current?.pause();
          setIsPlaying(false);
        }}
        onPlay={() => {
          setIsPlaying(true);
          onPlaybackStart?.();
          if (usingFallbackAudio && fallbackAudioRef.current && fallbackAudioSourceUrl) {
            void fallbackAudioRef.current.play().catch(() => undefined);
          }
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
            <span className="audio-track-status">{audioTrackStatusText}</span>
          </div>

          {visibleAudioTracks.length > 0 ? (
            <div className="audio-track-list">
              {visibleAudioTracks.map((track) => (
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
          ) : audioTracksSupported ? (
            <p className="audio-track-empty">
              No internal audio tracks are visible yet. Load a muxed MKV/MP4 and wait for media metadata.
            </p>
          ) : usingFallbackAudio ? (
            <p className="audio-track-empty">
              FFmpeg could not expose audio tracks for this file yet. Try another source or wait for the probe to
              finish.
            </p>
          ) : (
            <p className="audio-track-empty">{audioTracksUnavailableMessage}</p>
          )}
        </div>
      )}

      <audio ref={fallbackAudioRef} hidden preload="auto" src={fallbackAudioSourceUrl ?? undefined} />

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
            if (usingFallbackAudio && fallbackAudioRef.current) {
              fallbackAudioRef.current.volume = value;
            }
            if (videoRef.current && !usingFallbackAudio) {
              videoRef.current.volume = value;
            } else if (videoRef.current && usingFallbackAudio) {
              videoRef.current.volume = 0;
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
