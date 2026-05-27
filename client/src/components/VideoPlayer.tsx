import { type RefObject, useCallback, useEffect, useMemo, useRef, useState } from "react";
import "./VideoPlayer.css";
import type { AudioTrackInfo, SubtitleTrackInfo } from "../services/types";
import { UI_CONFIG } from "../config";
import { uiLogger } from "../utils/logger";
import { PlayerSettingsMenu } from "./PlayerSettingsMenu";

const HIDE_DELAY_MS = UI_CONFIG.hideControlsDelayMs;
const VIDEO_SCALE_STORAGE_KEY = "torrsyncplayer.videoScale";

function formatTime(timeInSeconds: number): string {
  const safe = Number.isFinite(timeInSeconds) ? Math.max(0, timeInSeconds) : 0;
  const hours = Math.floor(safe / 3600);
  const minutes = Math.floor((safe % 3600) / 60);
  const seconds = Math.floor(safe % 60);
  if (hours > 0) {
    return `${hours}:${minutes.toString().padStart(2, "0")}:${seconds.toString().padStart(2, "0")}`;
  }
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

function isInteractiveTarget(element: EventTarget | null): boolean {
  if (!(element instanceof HTMLElement)) return false;
  return ["INPUT", "TEXTAREA", "SELECT"].includes(element.tagName) || element.isContentEditable;
}

function readInitialVideoScale(): string {
  if (typeof window === "undefined") return "fit";
  try {
    const stored = window.localStorage.getItem(VIDEO_SCALE_STORAGE_KEY);
    if (["fit", "fill", "stretch", "original"].includes(stored ?? "")) return stored!;
  } catch { /* ok */ }
  return "fit";
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

type VideoWithAudioTracks = HTMLVideoElement & { audioTracks?: AudioTrackListLike };

function detectAudioTracksSupport(): boolean {
  if (typeof document === "undefined") return false;
  return "audioTracks" in document.createElement("video");
}

interface VideoPlayerProps {
  videoRef?: RefObject<HTMLVideoElement | null>;
  mediaLabel?: string | null;
  mediaKind?: "video" | "audio" | null;
  statusMessage?: string | null;
  canControlPlayback?: boolean;
  canControlSeek?: boolean;
  canControlAudioTracks?: boolean;
  canControlSubtitleTracks?: boolean;
  fallbackAudioTracks?: AudioTrackInfo[];
  selectedAudioTrackIndex?: number | null;
  fallbackSubtitles?: SubtitleTrackInfo[];
  selectedSubtitleIndex?: number | null;
  onPlaybackStart?: () => void;
  onAudioTrackChange?: (trackIndex: number | null) => void;
  onSubtitleTrackChange?: (trackIndex: number | null) => void;
  onPlayerReady?: (ready: boolean) => void;
  onBufferingChange?: (isBuffering: boolean) => void;
  onTimeUpdate?: (currentTime: number, duration: number | undefined) => void;
  bufferWindowMB?: number;
  maxBufferMB?: number;
  onBufferSettingsChange?: (bufferWindowMB: number, maxBufferMB: number) => void;
  onSeek?: (timestamp: number) => void;
}

function VideoPlayer({
  videoRef: externalVideoRef,
  mediaLabel,
  mediaKind,
  statusMessage,
  canControlPlayback = true,
  canControlSeek = true,
  canControlAudioTracks = true,
  canControlSubtitleTracks = true,
  fallbackAudioTracks = [],
  selectedAudioTrackIndex = null,
  fallbackSubtitles = [],
  selectedSubtitleIndex = null,
  onPlaybackStart,
  onAudioTrackChange,
  onSubtitleTrackChange,
  onPlayerReady,
  onBufferingChange,
  onTimeUpdate,
  bufferWindowMB = 50,
  maxBufferMB = 500,
  onBufferSettingsChange,
  onSeek,
}: VideoPlayerProps) {
  const internalVideoRef = useRef<HTMLVideoElement | null>(null);
  const videoRef = externalVideoRef ?? internalVideoRef;
  const hideTimerRef = useRef<number | null>(null);
  const onPlayerReadyRef = useRef(onPlayerReady);
  const onAudioTrackChangeRef = useRef(onAudioTrackChange);
  const onSubtitleTrackChangeRef = useRef(onSubtitleTrackChange);
  const onBufferingChangeRef = useRef(onBufferingChange);
  const onSeekRef = useRef(onSeek);
  const canControlPlaybackRef = useRef(canControlPlayback);
  const canControlSeekRef = useRef(canControlSeek);
  const togglePlayRef = useRef<() => Promise<void>>(null!);
  const hasMediaMetadataRef = useRef(false);
  const settingsMenuRef = useRef<HTMLDivElement | null>(null);
  const settingsButtonRef = useRef<HTMLButtonElement | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const [showControls, setShowControls] = useState(true);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [videoScale, setVideoScale] = useState<string>(() => readInitialVideoScale());
  const [audioTracksSupported, setAudioTracksSupported] = useState(() => detectAudioTracksSupport());
  const [audioTracks, setAudioTracks] = useState<AudioTrackSnapshot[]>([]);
  const [isBuffering, setIsBuffering] = useState(false);
  const [isStalled, setIsStalled] = useState(false);
  const [subtitleUrl, setSubtitleUrl] = useState<string | null>(null);
  const [editBufferWindowMB, setEditBufferWindowMB] = useState(bufferWindowMB);
  const [editMaxBufferMB, setEditMaxBufferMB] = useState(maxBufferMB);

  const fallbackAudioTrackSnapshots = useMemo(
    () => fallbackAudioTracks.map((track, index) => ({
      sourceIndex: index,
      label: track.label || `Audio ${index + 1}`,
      language: track.language || "",
      enabled: false,
    })),
    [fallbackAudioTracks],
  );

  const fallbackSubtitleSnapshots = useMemo(
    () => fallbackSubtitles.map((track, index) => ({
      sourceIndex: index,
      label: track.label || `Subtitle ${index + 1}`,
      language: track.language || "",
    })),
    [fallbackSubtitles],
  );

  useEffect(() => { onSubtitleTrackChangeRef.current = onSubtitleTrackChange; }, [onSubtitleTrackChange]);
  useEffect(() => { setEditBufferWindowMB(bufferWindowMB); }, [bufferWindowMB]);
  useEffect(() => { setEditMaxBufferMB(maxBufferMB); }, [maxBufferMB]);

  const progress = useMemo(() => duration ? (currentTime / duration) * 100 : 0, [currentTime, duration]);
  const activeVideoScaleLabel = ["fit", "fill", "stretch", "original"].includes(videoScale) ? videoScale : "fit";

  const resetHideTimer = useCallback(() => {
    setShowControls(true);
    if (hideTimerRef.current) window.clearTimeout(hideTimerRef.current);
    if (settingsOpen) return;
    hideTimerRef.current = window.setTimeout(() => setShowControls(false), HIDE_DELAY_MS);
  }, [settingsOpen]);

  useEffect(() => { onPlayerReadyRef.current = onPlayerReady; }, [onPlayerReady]);
  useEffect(() => { onAudioTrackChangeRef.current = onAudioTrackChange; }, [onAudioTrackChange]);
  useEffect(() => { onBufferingChangeRef.current = onBufferingChange; }, [onBufferingChange]);
  useEffect(() => { onSeekRef.current = onSeek; }, [onSeek]);
  useEffect(() => { canControlPlaybackRef.current = canControlPlayback; }, [canControlPlayback]);
  useEffect(() => { canControlSeekRef.current = canControlSeek; }, [canControlSeek]);

  useEffect(() => {
    if (selectedSubtitleIndex === null || selectedSubtitleIndex === undefined) {
      setSubtitleUrl(null);
      return;
    }
    const track = fallbackSubtitles.find((t) => t.index === selectedSubtitleIndex);
    if (track?.streamUrl) setSubtitleUrl(track.streamUrl);
  }, [selectedSubtitleIndex, fallbackSubtitles]);

  useEffect(() => { try { window.localStorage.setItem(VIDEO_SCALE_STORAGE_KEY, videoScale); } catch { /* ok */ } }, [videoScale]);
  useEffect(() => { onPlayerReadyRef.current?.(true); return () => onPlayerReadyRef.current?.(false); }, []);

  const syncAudioTracks = useCallback(() => {
    const v = videoRef.current;
    const tl = v ? (v as VideoWithAudioTracks).audioTracks : undefined;
    if (!mediaLabel) { setAudioTracks([]); return; }
    if (!hasMediaMetadataRef.current) { setAudioTracks([]); return; }
    if (!tl) { setAudioTracksSupported(false); setAudioTracks([]); return; }
    setAudioTracksSupported(true);
    const snap = Array.from(tl).map((t, i) => ({ sourceIndex: i, label: t.label || `Audio ${i + 1}`, language: t.language || "", enabled: t.enabled }));
    const req = selectedAudioTrackIndex !== null ? snap.find((t) => t.sourceIndex === selectedAudioTrackIndex) ?? null : null;
    const en = snap.find((t) => t.enabled) ?? snap[0] ?? null;
    const res = req ?? en;
    if (res) { for (const [i, t] of Array.from(tl).entries()) { t.enabled = i === res.sourceIndex; } }
    setAudioTracks(snap.map((t) => ({ ...t, enabled: res ? t.sourceIndex === res.sourceIndex : t.enabled })));
  }, [mediaLabel, selectedAudioTrackIndex, videoRef]);

  useEffect(() => { hasMediaMetadataRef.current = false; setAudioTracks([]); setAudioTracksSupported(detectAudioTracksSupport()); setSettingsOpen(false); setIsBuffering(false); setIsStalled(false); }, [mediaLabel]);
  useEffect(() => { if (audioTracksSupported) syncAudioTracks(); }, [audioTracksSupported, syncAudioTracks]);

  const togglePlay = useCallback(async () => {
    if (!canControlPlayback) return;
    const v = videoRef.current;
    if (!v) return;
    if (v.paused) {
      try { await v.play(); setIsPlaying(true); }
      catch (err) { uiLogger.warn("Play failed:", err); }
    } else {
      try { v.pause(); setIsPlaying(false); }
      catch (err) { uiLogger.warn("Pause failed:", err); }
    }
  }, [canControlPlayback, videoRef]);
  togglePlayRef.current = togglePlay;

  const applyVolume = useCallback((nv: number) => {
    const n = Number.isFinite(nv) ? Math.min(1, Math.max(0, nv)) : 1;
    setVolume(n);
    if (videoRef.current) videoRef.current.volume = n;
  }, [videoRef]);

  const activateAudioTrack = useCallback((idx: number) => {
    const v = videoRef.current;
    const tl = v ? (v as VideoWithAudioTracks).audioTracks : undefined;
    if (tl) {
      const sel = audioTracks.find((t) => t.sourceIndex === idx);
      if (!sel) return;
      for (const [i, t] of Array.from(tl).entries()) { t.enabled = i === sel.sourceIndex; }
      setAudioTracks(audioTracks.map((t) => ({ ...t, enabled: t.sourceIndex === sel.sourceIndex })));
      onAudioTrackChangeRef.current?.(sel.sourceIndex);
      return;
    }
    const sel = fallbackAudioTrackSnapshots.find((t) => t.sourceIndex === idx);
    if (sel) onAudioTrackChangeRef.current?.(sel.sourceIndex);
  }, [audioTracks, fallbackAudioTrackSnapshots, videoRef]);

  const activateSubtitleTrack = useCallback((idx: number | null) => {
    onSubtitleTrackChangeRef.current?.(idx);
  }, []);

  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    let timer: number | null = null;
    const onWait = () => { timer = window.setTimeout(() => { setIsBuffering(true); setIsStalled(true); onBufferingChangeRef.current?.(true); }, 300); };
    const onCan = () => { if (timer !== null) { window.clearTimeout(timer); timer = null; } setIsBuffering(false); setIsStalled(false); onBufferingChangeRef.current?.(false); };
    const onPlay = () => { if (timer !== null) { window.clearTimeout(timer); timer = null; } setIsBuffering(false); setIsStalled(false); onBufferingChangeRef.current?.(false); };
    const onStall = () => { setIsStalled(true); };
    const onTU = () => { if (timer !== null) { window.clearTimeout(timer); timer = null; } setIsBuffering(false); setIsStalled(false); onBufferingChangeRef.current?.(false); };
    v.addEventListener("waiting", onWait);
    v.addEventListener("canplay", onCan);
    v.addEventListener("playing", onPlay);
    v.addEventListener("stalled", onStall);
    v.addEventListener("timeupdate", onTU);
    return () => { v.removeEventListener("waiting", onWait); v.removeEventListener("canplay", onCan); v.removeEventListener("playing", onPlay); v.removeEventListener("stalled", onStall); v.removeEventListener("timeupdate", onTU); if (timer !== null) window.clearTimeout(timer); };
  }, [videoRef]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && settingsOpen) { setSettingsOpen(false); return; }
      if (!canControlPlaybackRef.current) return;
      const v = videoRef.current; if (!v) return;
      if (isInteractiveTarget(e.target)) return;
      if (e.code === "Space") { e.preventDefault(); void togglePlayRef.current(); }
      if (e.key.toLowerCase() === "f") {
        if (document.fullscreenElement) { void document.exitFullscreen(); } else { void v.requestFullscreen(); }
      }
      if (e.key === "ArrowRight" && canControlSeekRef.current) { if (Number.isFinite(v.duration)) v.currentTime = Math.min(v.duration, v.currentTime + 5); onSeekRef.current?.(v.currentTime); }
      if (e.key === "ArrowLeft" && canControlSeekRef.current) { v.currentTime = Math.max(0, v.currentTime - 5); onSeekRef.current?.(v.currentTime); }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [settingsOpen, videoRef, canControlSeek, canControlPlayback]);

  useEffect(() => {
    if (!settingsOpen) return;
    const handler = (e: PointerEvent) => {
      const t = e.target;
      if (t instanceof Node && (settingsMenuRef.current?.contains(t) || settingsButtonRef.current?.contains(t))) return;
      setSettingsOpen(false);
    };
    document.addEventListener("pointerdown", handler);
    return () => document.removeEventListener("pointerdown", handler);
  }, [settingsOpen]);

  const hasSelectedMedia = Boolean(mediaLabel);

  return (
    <section className={`video-player ${mediaKind === "audio" ? "audio-mode" : "video-mode"} scale-${videoScale}`} onMouseMove={resetHideTimer}>
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
        onClick={() => { if (canControlPlayback) void togglePlay(); }}
        onTimeUpdate={(e) => { setCurrentTime(e.currentTarget.currentTime); onTimeUpdate?.(e.currentTarget.currentTime, e.currentTarget.duration); }}
        onLoadedMetadata={(e) => { hasMediaMetadataRef.current = true; setDuration(e.currentTarget.duration); syncAudioTracks(); }}
        onSeeking={(e) => { setCurrentTime(e.currentTarget.currentTime); }}
        onSeeked={(e) => {
          if (!canControlPlayback) return;
          const targetTime = e.currentTarget.currentTime;
          onSeek?.(targetTime);
        }}
        onCanPlay={() => { setIsBuffering(false); setIsStalled(false); onBufferingChangeRef.current?.(false); }}
        onPause={() => setIsPlaying(false)}
        onPlay={() => { setIsPlaying(true); onPlaybackStart?.(); }}
      >
        {subtitleUrl && (
          <track
            kind="subtitles"
            src={subtitleUrl}
            label={fallbackSubtitles.find((t) => t.streamUrl === subtitleUrl)?.label ?? "Subtitles"}
            srcLang={fallbackSubtitles.find((t) => t.streamUrl === subtitleUrl)?.language ?? "en"}
            default
          />
        )}
      </video>
      {hasSelectedMedia && (
        <div className="video-legend">
          <span className={`video-kind ${mediaKind ?? "video"}`}>{mediaKind === "audio" ? "Audio" : "Video"}</span>
          <span className="video-title">{mediaLabel}</span>
        </div>
      )}
      {statusMessage && <div className="playback-message" role="status" aria-live="polite">{statusMessage}</div>}
      {(isBuffering || isStalled) && <div className="buffering-indicator" role="status" aria-live="polite">{isStalled ? "Network stalled — waiting for data..." : "Buffering..."}</div>}
      <div className={`video-controls ${showControls ? "visible" : "hidden"}`}>
        <button type="button" onClick={() => void togglePlay()} disabled={!canControlPlayback} aria-label={isPlaying ? "Pause" : "Play"} aria-pressed={isPlaying}>{isPlaying ? "Pause" : "Play"}</button>
        <input type="range" min={0} max={duration || 100} step={0.1} value={currentTime} disabled={!canControlSeek}
          aria-label="Seek" aria-valuenow={Math.round(currentTime)} aria-valuemin={0} aria-valuemax={Math.round(duration || 100)}
          onChange={(e) => { if (!canControlSeek) return; const val = Number(e.target.value); setCurrentTime(val); if (videoRef.current) videoRef.current.currentTime = val; onSeek?.(val); }}
          onMouseUp={(e) => { if (!canControlSeek) return; onSeek?.(Number((e.target as HTMLInputElement).value)); }}
          onTouchEnd={(e) => { if (!canControlSeek) return; onSeek?.(Number((e.target as HTMLInputElement).value)); }}
        />
        <span className="time-label">{formatTime(currentTime)} / {formatTime(duration)}</span>
        <div className="volume-control">
          <span className="volume-icon" aria-hidden="true">{volume === 0 ? "🔇" : volume < 0.5 ? "🔉" : "🔊"}</span>
          <input type="range" min={0} max={1} step={0.01} value={volume} onChange={(e) => applyVolume(Number(e.target.value))} aria-label="Volume" />
          <span className="volume-label">{Math.round(volume * 100)}%</span>
        </div>
        <div className="settings-menu-anchor">
          <button ref={settingsButtonRef} type="button" className={`settings-toggle ${settingsOpen ? "active" : ""}`} aria-haspopup="dialog" aria-expanded={settingsOpen} aria-label="Player settings" onClick={() => setSettingsOpen((o) => !o)}>Settings</button>
          <PlayerSettingsMenu
            isOpen={settingsOpen}
            hasSelectedMedia={hasSelectedMedia}
            videoScale={videoScale}
            activeVideoScaleLabel={activeVideoScaleLabel}
            audioTracks={audioTracks}
            audioTracksSupported={audioTracksSupported}
            fallbackAudioTrackSnapshots={fallbackAudioTrackSnapshots}
            selectedAudioTrackIndex={selectedAudioTrackIndex ?? null}
            selectedSubtitleIndex={selectedSubtitleIndex ?? null}
            fallbackSubtitleSnapshots={fallbackSubtitleSnapshots}
            canControlAudioTracks={canControlAudioTracks}
            canControlSubtitleTracks={canControlSubtitleTracks}
            editBufferWindowMB={editBufferWindowMB}
            editMaxBufferMB={editMaxBufferMB}
            onVideoScaleChange={setVideoScale}
            onAudioTrackActivate={activateAudioTrack}
            onSubtitleTrackActivate={activateSubtitleTrack}
            onBufferWindowChange={setEditBufferWindowMB}
            onBufferMaxChange={setEditMaxBufferMB}
            onBufferApply={(win: number, max: number) => { const w = Math.max(1, Math.min(1000, win)); const m = Math.max(10, Math.min(2000, max)); onBufferSettingsChange?.(w, m); }}
            onClose={() => setSettingsOpen(false)}
            menuRef={settingsMenuRef}
          />
        </div>
        <button type="button" disabled={!canControlPlayback} onClick={() => { if (!canControlPlayback) return; const v = videoRef.current; if (!v) return; if (document.fullscreenElement) { void document.exitFullscreen(); } else { void v.requestFullscreen(); } }} aria-label={document.fullscreenElement ? "Exit fullscreen" : "Enter fullscreen"} aria-pressed={!!document.fullscreenElement}>Fullscreen</button>
      </div>
      <div className="video-progress" style={{ width: `${progress}%` }} />
    </section>
  );
}

export default VideoPlayer;
