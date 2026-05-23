import { type RefObject, useCallback, useEffect, useMemo, useRef, useState } from "react";
import "./VideoPlayer.css";
import type { AudioTrackInfo, SubtitleTrackInfo } from "../services/types";

const HIDE_DELAY_MS = 3000;
const VIDEO_SCALE_STORAGE_KEY = "torrsyncplayer.videoScale";

const VIDEO_SCALE_OPTIONS = [
  { value: "fit", label: "Fit", description: "Show the whole frame without cropping." },
  { value: "fill", label: "Fill", description: "Fill the player and crop edges if needed." },
  { value: "stretch", label: "Stretch", description: "Stretch video to the player bounds." },
  { value: "original", label: "Original", description: "Keep source pixels centered in the player." },
] as const;

type VideoScaleMode = (typeof VIDEO_SCALE_OPTIONS)[number]["value"];

function formatTime(timeInSeconds: number): string {
  const safe = Number.isFinite(timeInSeconds) ? Math.max(0, timeInSeconds) : 0;
  const minutes = Math.floor(safe / 60);
  const seconds = Math.floor(safe % 60);
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

function isInteractiveTarget(element: EventTarget | null): boolean {
  if (!(element instanceof HTMLElement)) return false;
  return ["INPUT", "TEXTAREA", "SELECT"].includes(element.tagName) || element.isContentEditable;
}

function readInitialVideoScale(): VideoScaleMode {
  if (typeof window === "undefined") return "fit";
  try {
    const stored = window.localStorage.getItem(VIDEO_SCALE_STORAGE_KEY);
    return VIDEO_SCALE_OPTIONS.some((o) => o.value === stored) ? (stored as VideoScaleMode) : "fit";
  } catch { return "fit"; }
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
  onTimeUpdate?: (currentTime: number, duration: number) => void;
  bufferWindowMB?: number;
  maxBufferMB?: number;
  onBufferSettingsChange?: (bufferWindowMB: number, maxBufferMB: number) => void;
  onSeek?: (timestamp: number) => void;
  onMuxStreamRequest?: (startSeconds: number) => Promise<string | null>;
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
  onMuxStreamRequest: _onMuxStreamRequest,
}: VideoPlayerProps) {
  const internalVideoRef = useRef<HTMLVideoElement | null>(null);
  const videoRef = externalVideoRef ?? internalVideoRef;
  const hideTimerRef = useRef<number | null>(null);
  const onPlayerReadyRef = useRef(onPlayerReady);
  const onAudioTrackChangeRef = useRef(onAudioTrackChange);
  const onSubtitleTrackChangeRef = useRef(onSubtitleTrackChange);
  const onBufferingChangeRef = useRef(onBufferingChange);
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
  const activeVideoScaleLabel = VIDEO_SCALE_OPTIONS.find((o) => o.value === videoScale)?.label ?? VIDEO_SCALE_OPTIONS[0].label;

  const resetHideTimer = useCallback(() => {
    setShowControls(true);
    if (hideTimerRef.current) window.clearTimeout(hideTimerRef.current);
    if (settingsOpen) return;
    hideTimerRef.current = window.setTimeout(() => setShowControls(false), HIDE_DELAY_MS);
  }, [settingsOpen]);

  useEffect(() => { onPlayerReadyRef.current = onPlayerReady; }, [onPlayerReady]);
  useEffect(() => { onAudioTrackChangeRef.current = onAudioTrackChange; }, [onAudioTrackChange]);
  useEffect(() => { onBufferingChangeRef.current = onBufferingChange; }, [onBufferingChange]);

  useEffect(() => {
    if (selectedSubtitleIndex === null || selectedSubtitleIndex === undefined) {
      setSubtitleUrl(null);
      return;
    }
    const track = fallbackSubtitles.find((t) => t.index === selectedSubtitleIndex);
    if (track?.streamUrl) {
      setSubtitleUrl(track.streamUrl);
    }
  }, [selectedSubtitleIndex, fallbackSubtitles]);
  useEffect(() => { try { window.localStorage.setItem(VIDEO_SCALE_STORAGE_KEY, videoScale); } catch { /* ok */ } }, [videoScale]);
  useEffect(() => { onPlayerReadyRef.current?.(true); return () => onPlayerReadyRef.current?.(false); }, []);

  const visibleAudioTracks = useMemo(
    () => (audioTracksSupported ? audioTracks : fallbackAudioTrackSnapshots),
    [audioTracks, audioTracksSupported, fallbackAudioTrackSnapshots],
  );

  const activeAudioTrackIndex = useMemo(() => {
    const sel = visibleAudioTracks.find((t) => t.sourceIndex === selectedAudioTrackIndex);
    if (sel) return sel.sourceIndex;
    const en = visibleAudioTracks.find((t) => t.enabled) ?? visibleAudioTracks[0] ?? null;
    return en?.sourceIndex ?? null;
  }, [selectedAudioTrackIndex, visibleAudioTracks]);

  const togglePlay = useCallback(async () => {
    if (!canControlPlayback) return;
    const v = videoRef.current;
    if (!v) return;
    if (v.paused) { await v.play(); setIsPlaying(true); }
    else { v.pause(); setIsPlaying(false); }
  }, [canControlPlayback, videoRef]);

  const applyVolume = useCallback((nv: number) => {
    const n = Number.isFinite(nv) ? Math.min(1, Math.max(0, nv)) : 1;
    setVolume(n);
    if (videoRef.current) videoRef.current.volume = n;
  }, [videoRef]);

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

  // Buffering / stalled listeners
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

  // Keyboard shortcuts
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && settingsOpen) { setSettingsOpen(false); return; }
      if (!canControlPlayback) return;
      const v = videoRef.current; if (!v) return;
      if (isInteractiveTarget(e.target)) return;
      if (e.code === "Space") { e.preventDefault(); void togglePlay(); }
      if (e.key.toLowerCase() === "f") {
        if (document.fullscreenElement) {
          void document.exitFullscreen();
        } else {
          void v.requestFullscreen();
        }
      }
      if (e.key === "ArrowRight" && canControlSeek) { if (Number.isFinite(v.duration)) v.currentTime = Math.min(v.duration, v.currentTime + 5); onSeek?.(v.currentTime); }
      if (e.key === "ArrowLeft" && canControlSeek) { v.currentTime = Math.max(0, v.currentTime - 5); onSeek?.(v.currentTime); }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [canControlPlayback, canControlSeek, settingsOpen, togglePlay, videoRef, onSeek]);

  // Settings menu close on outside click
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

  useEffect(() => { hasMediaMetadataRef.current = false; setAudioTracks([]); setAudioTracksSupported(detectAudioTracksSupport()); setSettingsOpen(false); setIsBuffering(false); setIsStalled(false); }, [mediaLabel]);
  useEffect(() => { if (audioTracksSupported) syncAudioTracks(); }, [audioTracksSupported, syncAudioTracks]);



  const hasSelectedMedia = Boolean(mediaLabel);
  const audioTrackStatusText = audioTracksSupported
    ? audioTracks.length > 0 ? `${audioTracks.length} available` : "Waiting for media metadata"
    : fallbackAudioTrackSnapshots.length > 0 ? `${fallbackAudioTrackSnapshots.length} available via ffmpeg` : "Unavailable in this runtime";
  const subtitleStatusText = fallbackSubtitleSnapshots.length > 0 ? `${fallbackSubtitleSnapshots.length} available` : "No subtitles";
  const audioTracksUnavailableMessage = <>This runtime does not expose <code>audioTracks</code>, and no ffmpeg fallback track is available for this file.</>;

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
        crossOrigin="anonymous"
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
        <button type="button" onClick={() => void togglePlay()} disabled={!canControlPlayback}>{isPlaying ? "Pause" : "Play"}</button>
        <input type="range" min={0} max={duration || 100} step={0.1} value={currentTime} disabled={!canControlSeek}
          onChange={(e) => { if (!canControlSeek) return; const val = Number(e.target.value); setCurrentTime(val); if (videoRef.current) videoRef.current.currentTime = val; }}
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
          <button ref={settingsButtonRef} type="button" className={`settings-toggle ${settingsOpen ? "active" : ""}`} aria-haspopup="dialog" aria-expanded={settingsOpen} onClick={() => setSettingsOpen((o) => !o)}>Settings</button>
          {settingsOpen && (
            <div ref={settingsMenuRef} className="player-settings-menu" role="dialog" aria-label="Player settings">
              <div className="settings-menu-header">
                <div><span className="settings-kicker">Player settings</span><strong>{activeVideoScaleLabel} video scale</strong></div>
                <button type="button" className="settings-close" onClick={() => setSettingsOpen(false)} aria-label="Close settings">Close</button>
              </div>
              <div className="settings-section">
                <div className="settings-section-header"><span>Video scale</span><span>{activeVideoScaleLabel}</span></div>
                <div className="scale-option-list" role="radiogroup" aria-label="Video scale">
                  {VIDEO_SCALE_OPTIONS.map((opt) => (
                    <button key={opt.value} type="button" className={`scale-option ${videoScale === opt.value ? "active" : ""}`} role="radio" aria-checked={videoScale === opt.value} onClick={() => setVideoScale(opt.value)}>
                      <span className="scale-option-name">{opt.label}</span>
                      <span className="scale-option-copy">{opt.description}</span>
                    </button>
                  ))}
                </div>
              </div>
              <div className="settings-section">
                <div className="settings-section-header"><span>Audio tracks</span><span>{audioTrackStatusText}</span></div>
                {hasSelectedMedia && visibleAudioTracks.length > 0 ? (
                  <div className="audio-track-list">
                    {visibleAudioTracks.map((t) => (
                      <button key={t.sourceIndex} type="button" className={`audio-track-button ${activeAudioTrackIndex === t.sourceIndex ? "active" : ""}`} onClick={() => activateAudioTrack(t.sourceIndex)} disabled={!canControlAudioTracks}>
                        <span className="audio-track-name">{t.label}</span>
                        <span className="audio-track-meta">{t.language ? t.language.toUpperCase() : "Unknown language"}</span>
                      </button>
                    ))}
                  </div>
                ) : hasSelectedMedia && audioTracksSupported ? (
                  <p className="settings-empty">No internal audio tracks are visible yet. Load a muxed MKV/MP4 and wait for media metadata.</p>
) : hasSelectedMedia && fallbackAudioTrackSnapshots.length > 0 ? (
                   <p className="settings-empty">FFmpeg could not expose audio tracks for this file yet. Try another source or wait for the probe to finish.</p>
                 ) : hasSelectedMedia ? (
                   <p className="settings-empty">{audioTracksUnavailableMessage}</p>
                 ) : (
                   <p className="settings-empty">Load media to inspect audio tracks.</p>
                 )}
               </div>
               <div className="settings-section">
                 <div className="settings-section-header"><span>Subtitles</span><span>{subtitleStatusText}</span></div>
                 {hasSelectedMedia && fallbackSubtitleSnapshots.length > 0 ? (
                   <div className="audio-track-list">
                     {fallbackSubtitleSnapshots.map((t) => (
                       <button key={t.sourceIndex} type="button" className={`audio-track-button ${selectedSubtitleIndex === t.sourceIndex ? "active" : ""}`} onClick={() => activateSubtitleTrack(t.sourceIndex)} disabled={!canControlSubtitleTracks}>
                         <span className="audio-track-name">{t.label}</span>
                         <span className="audio-track-meta">{t.language ? t.language.toUpperCase() : "Unknown language"}</span>
                       </button>
                     ))}
                      <button type="button" className={`audio-track-button ${selectedSubtitleIndex === null ? "active" : ""}`} onClick={() => activateSubtitleTrack(null)} disabled={!canControlSubtitleTracks}>
                        <span className="audio-track-name">None</span>
                        <span className="audio-track-meta">Disable subtitles</span>
                      </button>
                    </div>
                  ) : hasSelectedMedia ? (
                    <p className="settings-empty">No subtitle tracks available for this file.</p>
                  ) : (
                    <p className="settings-empty">Load media to inspect subtitle tracks.</p>
                  )}
                </div>
                <div className="settings-section">
                  <div className="settings-section-header"><span>Buffer</span><span>{editBufferWindowMB} MB window</span></div>
                  <div className="buffer-settings-row">
                    <label htmlFor="buffer-window-mb">Window (MB)</label>
                    <input id="buffer-window-mb" type="number" min={1} max={1000} step={10} value={editBufferWindowMB} onChange={(e) => setEditBufferWindowMB(Math.max(1, Math.min(1000, Number(e.target.value) || 50)))} />
                  </div>
                  <div className="buffer-settings-row">
                    <label htmlFor="max-buffer-mb">Max buffer (MB)</label>
                    <input id="max-buffer-mb" type="number" min={10} max={2000} step={10} value={editMaxBufferMB} onChange={(e) => setEditMaxBufferMB(Math.max(10, Math.min(2000, Number(e.target.value) || 500)))} />
                  </div>
                  <button type="button" className="secondary-btn" onClick={() => onBufferSettingsChange?.(editBufferWindowMB, editMaxBufferMB)}>Apply buffer settings</button>
                  <p className="settings-hint">Larger window = smoother seeking, more bandwidth. Smaller window = less wasted data.</p>
                </div>
                <div className="settings-section">
                  <div className="settings-section-header"><span>Keyboard shortcuts</span></div>
                  <div className="shortcuts-list">
                    <div className="shortcut-row"><kbd>Space</kbd><span>Play / Pause</span></div>
                    <div className="shortcut-row"><kbd>F</kbd><span>Toggle fullscreen</span></div>
                    <div className="shortcut-row"><kbd>←</kbd><span>Seek back 5s</span></div>
                    <div className="shortcut-row"><kbd>→</kbd><span>Seek forward 5s</span></div>
                    <div className="shortcut-row"><kbd>Esc</kbd><span>Close settings</span></div>
                  </div>
                </div>
              </div>
            )}
          </div>
        <button type="button" disabled={!canControlPlayback} onClick={() => { if (!canControlPlayback) return; const v = videoRef.current; if (!v) return; if (document.fullscreenElement) { void document.exitFullscreen(); } else { void v.requestFullscreen(); } }}>Fullscreen</button>
      </div>
      <div className="video-progress" style={{ width: `${progress}%` }} />
    </section>
  );
}

export default VideoPlayer;
