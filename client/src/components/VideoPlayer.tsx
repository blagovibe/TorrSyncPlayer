import { type RefObject, useCallback, useEffect, useMemo, useRef, useState } from "react";
import "./VideoPlayer.css";
import type { AudioTrackInfo } from "../services/types";

const HIDE_DELAY_MS = 3000;
const VIDEO_SCALE_STORAGE_KEY = "torrsyncplayer.videoScale";

const VIDEO_SCALE_OPTIONS = [
  {
    value: "fit",
    label: "Fit",
    description: "Show the whole frame without cropping.",
  },
  {
    value: "fill",
    label: "Fill",
    description: "Fill the player and crop edges if needed.",
  },
  {
    value: "stretch",
    label: "Stretch",
    description: "Stretch video to the player bounds.",
  },
  {
    value: "original",
    label: "Original",
    description: "Keep source pixels centered in the player.",
  },
] as const;

type VideoScaleMode = (typeof VIDEO_SCALE_OPTIONS)[number]["value"];

function formatTime(timeInSeconds: number): string {
  const safe = Number.isFinite(timeInSeconds) ? Math.max(0, timeInSeconds) : 0;
  const minutes = Math.floor(safe / 60);
  const seconds = Math.floor(safe % 60);
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

function isInteractiveTarget(element: EventTarget | null): boolean {
  if (!(element instanceof HTMLElement)) {
    return false;
  }
  return ["INPUT", "TEXTAREA", "SELECT"].includes(element.tagName) || element.isContentEditable;
}

function readInitialVideoScale(): VideoScaleMode {
  if (typeof window === "undefined") {
    return "fit";
  }

  try {
    const storedScale = window.localStorage.getItem(VIDEO_SCALE_STORAGE_KEY);
    return VIDEO_SCALE_OPTIONS.some((option) => option.value === storedScale) ? (storedScale as VideoScaleMode) : "fit";
  } catch {
    return "fit";
  }
}

interface VideoPlayerProps {
  videoRef?: RefObject<HTMLVideoElement | null>;
  mediaLabel?: string | null;
  mediaKind?: "video" | "audio" | null;
  statusMessage?: string | null;
  canControlPlayback?: boolean;
  canControlSeek?: boolean;
  canControlAudioTracks?: boolean;
  fallbackAudioTracks?: AudioTrackInfo[];
  selectedAudioTrackIndex?: number | null;
  onPlaybackStart?: () => void;
  onAudioTrackChange?: (trackIndex: number | null) => void;
  resolveFallbackAudioTrackSource?: (trackIndex: number, startSeconds: number) => Promise<string | null>;
  onPlayerReady?: (ready: boolean) => void;
  onBufferingChange?: (isBuffering: boolean) => void;
  onTimeUpdate?: (currentTime: number, duration: number) => void;
  bufferWindowMB?: number;
  maxBufferMB?: number;
  onBufferSettingsChange?: (bufferWindowMB: number, maxBufferMB: number) => void;
  onSeek?: (timestamp: number) => void;
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
  canControlSeek = true,
  canControlAudioTracks = true,
  fallbackAudioTracks = [],
  selectedAudioTrackIndex = null,
  onPlaybackStart,
  onAudioTrackChange,
  resolveFallbackAudioTrackSource,
  onPlayerReady,
  onBufferingChange,
  onTimeUpdate,
  bufferWindowMB = 50,
  maxBufferMB = 500,
  onBufferSettingsChange,
  onSeek,
}: VideoPlayerProps) {
  const internalVideoRef = useRef<HTMLVideoElement | null>(null);
  const fallbackAudioRef = useRef<HTMLAudioElement | null>(null);
  const videoRef = externalVideoRef ?? internalVideoRef;
  const hideTimerRef = useRef<number | null>(null);
  const onPlayerReadyRef = useRef(onPlayerReady);
  const onAudioTrackChangeRef = useRef(onAudioTrackChange);
  const onBufferingChangeRef = useRef(onBufferingChange);
  const fallbackAudioRequestIdRef = useRef(0);
  const hasMediaMetadataRef = useRef(false);
  const settingsMenuRef = useRef<HTMLDivElement | null>(null);
  const settingsButtonRef = useRef<HTMLButtonElement | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const [showControls, setShowControls] = useState(true);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [videoScale, setVideoScale] = useState<VideoScaleMode>(() => readInitialVideoScale());
  const [audioTracksSupported, setAudioTracksSupported] = useState(() => detectAudioTracksSupport());
  const [audioTracks, setAudioTracks] = useState<AudioTrackSnapshot[]>([]);
  const [fallbackAudioSourceUrl, setFallbackAudioSourceUrl] = useState<string | null>(null);
  const [isBuffering, setIsBuffering] = useState(false);
  const [isStalled, setIsStalled] = useState(false);
  const [editBufferWindowMB, setEditBufferWindowMB] = useState(bufferWindowMB);
  const [editMaxBufferMB, setEditMaxBufferMB] = useState(maxBufferMB);
  // Track whether video was playing before a seek — so we can auto-resume.
  const wasPlayingBeforeSeekRef = useRef(false);
  // Track whether we are currently waiting for data after a seek.
  const isWaitingAfterSeekRef = useRef(false);
  // Track individual ready states during seek recovery.
  const videoReadyRef = useRef(false);
  const audioReadyRef = useRef(false);

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

  // Helper: try to resume playback after seek once video is ready.
  // For fallback audio, we don't wait — video starts immediately and audio
  // will sync when its source loads (via onPlay handler).
  const tryResumeAfterSeek = useCallback(() => {
    if (!isWaitingAfterSeekRef.current) return;
    const videoOk = videoReadyRef.current;
    if (videoOk) {
      isWaitingAfterSeekRef.current = false;
      videoReadyRef.current = false;
      audioReadyRef.current = false;
      setIsBuffering(false);
      setIsStalled(false);
      onBufferingChangeRef.current?.(false);
      if (wasPlayingBeforeSeekRef.current && videoRef.current) {
        void videoRef.current.play().catch(() => undefined);
        setIsPlaying(true);
      }
    }
  }, [videoRef]);

  useEffect(() => {
    setEditBufferWindowMB(bufferWindowMB);
  }, [bufferWindowMB]);

  useEffect(() => {
    setEditMaxBufferMB(maxBufferMB);
  }, [maxBufferMB]);

  const progress = useMemo(() => {
    if (!duration) {
      return 0;
    }
    return (currentTime / duration) * 100;
  }, [currentTime, duration]);

  const activeVideoScaleLabel =
    VIDEO_SCALE_OPTIONS.find((option) => option.value === videoScale)?.label ?? VIDEO_SCALE_OPTIONS[0].label;

  const resetHideTimer = useCallback(() => {
    setShowControls(true);
    if (hideTimerRef.current) {
      window.clearTimeout(hideTimerRef.current);
    }
    if (settingsOpen) {
      return;
    }
    hideTimerRef.current = window.setTimeout(() => {
      setShowControls(false);
    }, HIDE_DELAY_MS);
  }, [settingsOpen]);

  useEffect(() => {
    onPlayerReadyRef.current = onPlayerReady;
  }, [onPlayerReady]);

  useEffect(() => {
    onAudioTrackChangeRef.current = onAudioTrackChange;
  }, [onAudioTrackChange]);

  useEffect(() => {
    onBufferingChangeRef.current = onBufferingChange;
  }, [onBufferingChange]);

  useEffect(() => {
    resetHideTimer();
    return () => {
      if (hideTimerRef.current) {
        window.clearTimeout(hideTimerRef.current);
      }
    };
  }, [resetHideTimer]);

  useEffect(() => {
    try {
      window.localStorage.setItem(VIDEO_SCALE_STORAGE_KEY, videoScale);
    } catch {
      // Persisting the scale is optional; playback settings still work for the session.
    }
  }, [videoScale]);

  useEffect(() => {
    if (!settingsOpen) {
      resetHideTimer();
      return;
    }

    setShowControls(true);
    if (hideTimerRef.current) {
      window.clearTimeout(hideTimerRef.current);
    }
  }, [resetHideTimer, settingsOpen]);

  useEffect(() => {
    onPlayerReadyRef.current?.(true);
    return () => onPlayerReadyRef.current?.(false);
  }, []);

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
        // Sync audio position to video before playing to avoid drift
        fallbackAudioRef.current.currentTime = video.currentTime;
        void fallbackAudioRef.current.play().catch(() => undefined);
      }
      setIsPlaying(true);
    } else {
      video.pause();
      fallbackAudioRef.current?.pause();
      setIsPlaying(false);
    }
  }, [canControlPlayback, fallbackAudioSourceUrl, usingFallbackAudio, videoRef]);

  // Apply volume changes directly without reloading audio element.
  const applyVolume = useCallback(
    (nextVolume: number) => {
      const normalizedVolume = Number.isFinite(nextVolume) ? Math.min(1, Math.max(0, nextVolume)) : 1;
      setVolume(normalizedVolume);
      if (usingFallbackAudio && fallbackAudioRef.current) {
        fallbackAudioRef.current.volume = normalizedVolume;
      }
      if (videoRef.current && !usingFallbackAudio) {
        videoRef.current.volume = normalizedVolume;
      } else if (videoRef.current && usingFallbackAudio) {
        videoRef.current.volume = 0;
      }
    },
    [usingFallbackAudio, videoRef],
  );

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

  const previousFallbackAudioSourceUrlRef = useRef<string | null>(null);
  const savedVolumeRef = useRef(1);

  const requestFallbackAudioSource = useCallback(
    async (startSeconds: number) => {
      if (!usingFallbackAudio || activeAudioTrackIndex === null || !resolveFallbackAudioTrackSource) {
        fallbackAudioRequestIdRef.current += 1;
        // Revoke previous object URL before clearing.
        if (previousFallbackAudioSourceUrlRef.current) {
          URL.revokeObjectURL(previousFallbackAudioSourceUrlRef.current);
          previousFallbackAudioSourceUrlRef.current = null;
        }
        setFallbackAudioSourceUrl(null);
        fallbackAudioRef.current?.pause();
        return;
      }

      const requestId = ++fallbackAudioRequestIdRef.current;
      const audio = fallbackAudioRef.current;
      audio?.pause();
      // Revoke previous object URL before loading new one.
      if (previousFallbackAudioSourceUrlRef.current) {
        URL.revokeObjectURL(previousFallbackAudioSourceUrlRef.current);
        previousFallbackAudioSourceUrlRef.current = null;
      }
      setFallbackAudioSourceUrl(null);

      try {
        const nextSourceUrl = await resolveFallbackAudioTrackSource(activeAudioTrackIndex, Math.max(0, startSeconds));
        if (requestId !== fallbackAudioRequestIdRef.current) {
          return;
        }

        previousFallbackAudioSourceUrlRef.current = nextSourceUrl;
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

  // Handle fallback audio source URL changes (load/play/pause).
  // NOTE: volume is NOT in the dependency array — it is applied via a separate effect.
  useEffect(() => {
    const audio = fallbackAudioRef.current;
    if (!audio) {
      return;
    }

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

    // Sync audio position to video before loading new source.
    const video = videoRef.current;
    if (video) {
      audio.currentTime = video.currentTime;
    }

    audio.load();
    // If video is already playing (e.g. after seek when video loaded before audio),
    // start audio immediately to maintain sync.
    if (video && !video.paused) {
      void audio.play().catch(() => undefined);
    }
  }, [fallbackAudioSourceUrl, usingFallbackAudio, videoRef]);

  // Apply volume changes to fallback audio independently — without calling audio.load().
  useEffect(() => {
    const audio = fallbackAudioRef.current;
    if (!audio) return;

    if (usingFallbackAudio) {
      audio.volume = volume;
    }
  }, [volume, usingFallbackAudio]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) {
      return;
    }

    if (usingFallbackAudio) {
      // Save current volume before muting for fallback audio.
      if (!video.muted && video.volume > 0) {
        savedVolumeRef.current = video.volume;
      }
      video.defaultMuted = true;
      video.muted = true;
      video.volume = 0;
      return;
    }

    video.defaultMuted = false;
    video.muted = false;
    // Restore saved volume when leaving fallback mode.
    video.volume = savedVolumeRef.current > 0 ? savedVolumeRef.current : volume;
  }, [usingFallbackAudio, videoRef, volume]);

  // Buffering / stalled state listeners.
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    let stalledTimer: number | null = null;

    const onWaiting = () => {
      // Delay to avoid false positives from tiny network hiccups.
      stalledTimer = window.setTimeout(() => {
        setIsBuffering(true);
        setIsStalled(true);
        onBufferingChangeRef.current?.(true);
      }, 300);
    };

    const onCanPlay = () => {
      if (stalledTimer !== null) {
        window.clearTimeout(stalledTimer);
        stalledTimer = null;
      }
      setIsBuffering(false);
      setIsStalled(false);
      onBufferingChangeRef.current?.(false);
    };

    const onPlaying = () => {
      if (stalledTimer !== null) {
        window.clearTimeout(stalledTimer);
        stalledTimer = null;
      }
      setIsBuffering(false);
      setIsStalled(false);
      onBufferingChangeRef.current?.(false);
    };

    const onStalled = () => {
      setIsStalled(true);
    };

    const onTimeUpdate = () => {
      // If we're getting time updates, data is arriving.
      if (stalledTimer !== null) {
        window.clearTimeout(stalledTimer);
        stalledTimer = null;
      }
      setIsBuffering(false);
      setIsStalled(false);
      onBufferingChangeRef.current?.(false);
    };

    video.addEventListener("waiting", onWaiting);
    video.addEventListener("canplay", onCanPlay);
    video.addEventListener("playing", onPlaying);
    video.addEventListener("stalled", onStalled);
    video.addEventListener("timeupdate", onTimeUpdate);

    return () => {
      video.removeEventListener("waiting", onWaiting);
      video.removeEventListener("canplay", onCanPlay);
      video.removeEventListener("playing", onPlaying);
      video.removeEventListener("stalled", onStalled);
      video.removeEventListener("timeupdate", onTimeUpdate);
      if (stalledTimer !== null) {
        window.clearTimeout(stalledTimer);
      }
    };
  }, [videoRef]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && settingsOpen) {
        setSettingsOpen(false);
        return;
      }

      if (!canControlPlayback) {
        return;
      }

      const video = videoRef.current;
      if (!video) {
        return;
      }

      if (isInteractiveTarget(event.target)) {
        return;
      }

      if (event.code === "Space") {
        event.preventDefault();
        void togglePlay();
      }
      if (event.key.toLowerCase() === "f") {
        if (document.fullscreenElement) {
          try {
            void document.exitFullscreen();
          } catch {
            // exitFullscreen may throw in some environments
          }
        } else {
          try {
            void video.requestFullscreen();
          } catch {
            // requestFullscreen may throw if denied
          }
        }
      }
      if (event.key === "ArrowRight" && canControlSeek) {
        if (Number.isFinite(video.duration)) {
          video.currentTime = Math.min(video.duration, video.currentTime + 5);
        }
        onSeek?.(video.currentTime);
      }
      if (event.key === "ArrowLeft" && canControlSeek) {
        video.currentTime = Math.max(0, video.currentTime - 5);
        onSeek?.(video.currentTime);
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [canControlPlayback, canControlSeek, settingsOpen, togglePlay, videoRef, onSeek]);

  // Pointer-down handler for closing settings menu — separate effect to avoid
  // re-registering keyboard listener when settingsOpen changes.
  useEffect(() => {
    if (!settingsOpen) {
      return;
    }

    const onPointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (
        target instanceof Node &&
        (settingsMenuRef.current?.contains(target) || settingsButtonRef.current?.contains(target))
      ) {
        return;
      }

      setSettingsOpen(false);
    };

    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [settingsOpen]);

  useEffect(() => {
    hasMediaMetadataRef.current = false;
    setAudioTracks([]);
    setAudioTracksSupported(detectAudioTracksSupport());
    // Revoke previous fallback audio object URL when media changes.
    if (previousFallbackAudioSourceUrlRef.current) {
      URL.revokeObjectURL(previousFallbackAudioSourceUrlRef.current);
      previousFallbackAudioSourceUrlRef.current = null;
    }
    setFallbackAudioSourceUrl(null);
    fallbackAudioRequestIdRef.current += 1;
    fallbackAudioRef.current?.pause();
    setSettingsOpen(false);
    setIsBuffering(false);
    setIsStalled(false);
  }, [mediaLabel]);

  // Revoke fallback audio object URL on unmount.
  useEffect(() => {
    return () => {
      if (previousFallbackAudioSourceUrlRef.current) {
        URL.revokeObjectURL(previousFallbackAudioSourceUrlRef.current);
        previousFallbackAudioSourceUrlRef.current = null;
      }
    };
  }, []);

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
    <section
      className={`video-player ${mediaKind === "audio" ? "audio-mode" : "video-mode"} scale-${videoScale}`}
      onMouseMove={resetHideTimer}
    >
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
        onTimeUpdate={(event) => {
          setCurrentTime(event.currentTarget.currentTime);
          onTimeUpdate?.(event.currentTarget.currentTime, event.currentTarget.duration);

          // Periodically sync fallback audio to prevent drift.
          if (usingFallbackAudio && fallbackAudioRef.current && !fallbackAudioRef.current.paused) {
            const drift = Math.abs(fallbackAudioRef.current.currentTime - event.currentTarget.currentTime);
            if (drift > 0.3) {
              fallbackAudioRef.current.currentTime = event.currentTarget.currentTime;
            }
          }
        }}
        onLoadedMetadata={(event) => {
          hasMediaMetadataRef.current = true;
          setDuration(event.currentTarget.duration);
          syncAudioTracks();
        }}
        onSeeking={() => {
          // User started seeking — remember if we were playing.
          wasPlayingBeforeSeekRef.current = !videoRef.current?.paused;
        }}
        onSeeked={(event) => {
          if (!canControlPlayback) return;

          // After a seek, pause both video and audio and wait for both to be ready.
          const video = videoRef.current;
          if (video) {
            video.pause();
            isWaitingAfterSeekRef.current = true;
            videoReadyRef.current = false;
            audioReadyRef.current = false;
            setIsPlaying(false);
            setIsBuffering(true);
            onBufferingChangeRef.current?.(true);
          }
          // Also pause fallback audio — it will resume when both are ready.
          fallbackAudioRef.current?.pause();

          if (usingFallbackAudio) {
            void requestFallbackAudioSource(event.currentTarget.currentTime);
          } else if (audioTracksSupported && fallbackAudioRef.current) {
            // For native audio tracks, sync fallback audio element if it exists.
            fallbackAudioRef.current.currentTime = event.currentTarget.currentTime;
          }
        }}
        onCanPlay={() => {
          if (isWaitingAfterSeekRef.current) {
            videoReadyRef.current = true;
            void tryResumeAfterSeek();
          }
        }}
        onPause={() => {
          fallbackAudioRef.current?.pause();
          setIsPlaying(false);
        }}
        onPlay={() => {
          setIsPlaying(true);
          onPlaybackStart?.();
          // Sync fallback audio to video position and start playing.
          // This handles: initial play, resume after seek, and drift correction.
          if (usingFallbackAudio && fallbackAudioRef.current && fallbackAudioSourceUrl) {
            const videoEl = videoRef.current;
            if (videoEl) {
              // Sync audio position to video to prevent drift after seek
              const drift = Math.abs(fallbackAudioRef.current.currentTime - videoEl.currentTime);
              if (drift > 0.1) {
                fallbackAudioRef.current.currentTime = videoEl.currentTime;
              }
            }
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

      <audio
        ref={fallbackAudioRef}
        hidden
        preload="auto"
        src={fallbackAudioSourceUrl ?? undefined}
        onCanPlay={() => {
          if (isWaitingAfterSeekRef.current) {
            audioReadyRef.current = true;
            void tryResumeAfterSeek();
          }
        }}
      />

      {statusMessage && (
        <div className="playback-message" role="status" aria-live="polite">
          {statusMessage}
        </div>
      )}

      {(isBuffering || isStalled) && (
        <div className="buffering-indicator" role="status" aria-live="polite">
          {isStalled ? "Network stalled — waiting for data..." : "Buffering..."}
        </div>
      )}

      <div className={`video-controls ${showControls ? "visible" : "hidden"}`}>
        <button type="button" onClick={() => void togglePlay()} disabled={!canControlPlayback}>
          {isPlaying ? "Pause" : "Play"}
        </button>

        <input
          type="range"
          min={0}
          max={duration || 100}
          step={0.1}
          value={currentTime}
          disabled={!canControlSeek}
          onChange={(event) => {
            if (!canControlSeek) {
              return;
            }
            const value = Number(event.target.value);
            setCurrentTime(value);
            if (videoRef.current) {
              videoRef.current.currentTime = value;
            }
          }}
          onMouseUp={(event) => {
            if (!canControlSeek) return;
            const value = Number((event.target as HTMLInputElement).value);
            onSeek?.(value);
          }}
          onTouchEnd={(event) => {
            if (!canControlSeek) return;
            const value = Number((event.target as HTMLInputElement).value);
            onSeek?.(value);
          }}
        />

        <span className="time-label">
          {formatTime(currentTime)} / {formatTime(duration)}
        </span>

        <div className="volume-control">
          <span className="volume-icon" aria-hidden="true">
            {volume === 0 ? "🔇" : volume < 0.5 ? "🔉" : "🔊"}
          </span>
          <input
            type="range"
            min={0}
            max={1}
            step={0.01}
            value={volume}
            onChange={(event) => {
              applyVolume(Number(event.target.value));
            }}
            aria-label="Volume"
          />
          <span className="volume-label">{Math.round(volume * 100)}%</span>
        </div>

        <div className="settings-menu-anchor">
          <button
            ref={settingsButtonRef}
            type="button"
            className={`settings-toggle ${settingsOpen ? "active" : ""}`}
            aria-haspopup="dialog"
            aria-expanded={settingsOpen}
            onClick={() => setSettingsOpen((open) => !open)}
          >
            Settings
          </button>

          {settingsOpen && (
            <div ref={settingsMenuRef} className="player-settings-menu" role="dialog" aria-label="Player settings">
              <div className="settings-menu-header">
                <div>
                  <span className="settings-kicker">Player settings</span>
                  <strong>{activeVideoScaleLabel} video scale</strong>
                </div>
                <button
                  type="button"
                  className="settings-close"
                  onClick={() => setSettingsOpen(false)}
                  aria-label="Close settings"
                >
                  Close
                </button>
              </div>

              <div className="settings-section">
                <div className="settings-section-header">
                  <span>Video scale</span>
                  <span>{activeVideoScaleLabel}</span>
                </div>
                <div className="scale-option-list" role="radiogroup" aria-label="Video scale">
                  {VIDEO_SCALE_OPTIONS.map((option) => (
                    <button
                      key={option.value}
                      type="button"
                      className={`scale-option ${videoScale === option.value ? "active" : ""}`}
                      role="radio"
                      aria-checked={videoScale === option.value}
                      onClick={() => setVideoScale(option.value)}
                    >
                      <span className="scale-option-name">{option.label}</span>
                      <span className="scale-option-copy">{option.description}</span>
                    </button>
                  ))}
                </div>
              </div>

              <div className="settings-section">
                <div className="settings-section-header">
                  <span>Audio tracks</span>
                  <span>{audioTrackStatusText}</span>
                </div>
                {hasSelectedMedia && visibleAudioTracks.length > 0 ? (
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
                ) : hasSelectedMedia && audioTracksSupported ? (
                  <p className="settings-empty">
                    No internal audio tracks are visible yet. Load a muxed MKV/MP4 and wait for media metadata.
                  </p>
                ) : hasSelectedMedia && usingFallbackAudio ? (
                  <p className="settings-empty">
                    FFmpeg could not expose audio tracks for this file yet. Try another source or wait for the probe to
                    finish.
                  </p>
                ) : hasSelectedMedia ? (
                  <p className="settings-empty">{audioTracksUnavailableMessage}</p>
                ) : (
                  <p className="settings-empty">Load media to inspect audio tracks.</p>
                )}
              </div>

              <div className="settings-section">
                <div className="settings-section-header">
                  <span>Buffer</span>
                  <span>{editBufferWindowMB} MB window</span>
                </div>
                <div className="buffer-settings-row">
                  <label htmlFor="buffer-window-mb">Window (MB)</label>
                  <input
                    id="buffer-window-mb"
                    type="number"
                    min={1}
                    max={1000}
                    step={10}
                    value={editBufferWindowMB}
                    onChange={(event) => {
                      const v = Math.max(1, Math.min(1000, Number(event.target.value) || 50));
                      setEditBufferWindowMB(v);
                    }}
                  />
                </div>
                <div className="buffer-settings-row">
                  <label htmlFor="max-buffer-mb">Max buffer (MB)</label>
                  <input
                    id="max-buffer-mb"
                    type="number"
                    min={10}
                    max={2000}
                    step={10}
                    value={editMaxBufferMB}
                    onChange={(event) => {
                      const v = Math.max(10, Math.min(2000, Number(event.target.value) || 500));
                      setEditMaxBufferMB(v);
                    }}
                  />
                </div>
                <button
                  type="button"
                  className="secondary-btn"
                  onClick={() => onBufferSettingsChange?.(editBufferWindowMB, editMaxBufferMB)}
                >
                  Apply buffer settings
                </button>
                <p className="settings-hint">
                  Larger window = smoother seeking, more bandwidth. Smaller window = less wasted data.
                </p>
              </div>
            </div>
          )}
        </div>

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
              try {
                void document.exitFullscreen();
              } catch {
                // exitFullscreen may throw in some environments
              }
            } else {
              try {
                void video.requestFullscreen();
              } catch {
                // requestFullscreen may throw if denied
              }
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
