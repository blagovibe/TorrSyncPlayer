import { type RefObject, useEffect } from "react";

interface PlayerSettingsMenuProps {
  isOpen: boolean;
  hasSelectedMedia: boolean;
  videoScale: string;
  activeVideoScaleLabel: string;
  audioTracks: AudioTrackSnapshot[];
  audioTracksSupported: boolean;
  fallbackAudioTrackSnapshots: Array<{ sourceIndex: number; label: string; language: string; enabled: boolean }>;
  selectedAudioTrackIndex: number | null;
  fallbackSubtitleSnapshots: Array<{ sourceIndex: number; label: string; language: string }>;
  selectedSubtitleIndex: number | null;
  canControlAudioTracks: boolean;
  canControlSubtitleTracks: boolean;
  editBufferWindowMB: number;
  editMaxBufferMB: number;
  onVideoScaleChange: (scale: string) => void;
  onAudioTrackActivate: (index: number) => void;
  onSubtitleTrackActivate: (index: number | null) => void;
  onBufferWindowChange: (v: number) => void;
  onBufferMaxChange: (v: number) => void;
  onBufferApply: (windowMB: number, maxBufferMB: number) => void;
  onClose: () => void;
  menuRef: RefObject<HTMLDivElement>;
}

export interface AudioTrackSnapshot {
  sourceIndex: number;
  label: string;
  language: string;
  enabled: boolean;
}

const VIDEO_SCALE_OPTIONS = [
  { value: "fit", label: "Fit", description: "Show the whole frame without cropping." },
  { value: "fill", label: "Fill", description: "Fill the player and crop edges if needed." },
  { value: "stretch", label: "Stretch", description: "Stretch video to the player bounds." },
  { value: "original", label: "Original", description: "Keep source pixels centered in the player." },
] as const;

export function PlayerSettingsMenu({
  isOpen,
  hasSelectedMedia,
  videoScale,
  activeVideoScaleLabel,
  audioTracks,
  audioTracksSupported,
  fallbackAudioTrackSnapshots,
  selectedAudioTrackIndex,
  selectedSubtitleIndex,
  fallbackSubtitleSnapshots,
  canControlAudioTracks,
  canControlSubtitleTracks,
  editBufferWindowMB,
  editMaxBufferMB,
  onVideoScaleChange,
  onAudioTrackActivate,
  onSubtitleTrackActivate,
  onBufferWindowChange,
  onBufferMaxChange,
  onBufferApply,
  onClose,
  menuRef,
}: PlayerSettingsMenuProps) {
  useEffect(() => {
    if (isOpen && menuRef.current) {
      const f = menuRef.current.querySelector<HTMLElement>('button, [href], input, [tabindex]:not([tabindex="-1"])');
      f?.focus();
    }
  }, [isOpen, menuRef]);

  const visibleAudioTracks = audioTracksSupported ? audioTracks : fallbackAudioTrackSnapshots;
  const activeAudioTrackIndex = (() => {
    const sel = visibleAudioTracks.find((t) => t.sourceIndex === selectedAudioTrackIndex);
    if (sel) return sel.sourceIndex;
    const en = visibleAudioTracks.find((t) => t.enabled) ?? visibleAudioTracks[0] ?? null;
    return en?.sourceIndex ?? null;
  })();

  const audioTrackStatusText = audioTracksSupported
    ? audioTracks.length > 0 ? `${audioTracks.length} available` : "Waiting for media metadata"
    : fallbackAudioTrackSnapshots.length > 0 ? `${fallbackAudioTrackSnapshots.length} available via ffmpeg` : "Unavailable in this runtime";
  const subtitleStatusText = fallbackSubtitleSnapshots.length > 0 ? `${fallbackSubtitleSnapshots.length} available` : "No subtitles";

  if (!isOpen) return null;

  return (
    <div ref={menuRef} className="player-settings-menu" role="dialog" aria-label="Player settings">
      <div className="settings-menu-header">
        <div><span className="settings-kicker">Player settings</span><strong>{activeVideoScaleLabel} video scale</strong></div>
        <button type="button" className="settings-close" onClick={onClose} aria-label="Close settings">Close</button>
      </div>
      <div className="settings-section">
        <div className="settings-section-header"><span>Video scale</span><span>{activeVideoScaleLabel}</span></div>
        <div className="scale-option-list" role="radiogroup" aria-label="Video scale">
          {VIDEO_SCALE_OPTIONS.map((opt) => (
            <button key={opt.value} type="button" className={`scale-option ${videoScale === opt.value ? "active" : ""}`} role="radio" aria-checked={videoScale === opt.value} onClick={() => onVideoScaleChange(opt.value)}>
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
              <button key={t.sourceIndex} type="button" className={`audio-track-button ${activeAudioTrackIndex === t.sourceIndex ? "active" : ""}`} onClick={() => onAudioTrackActivate(t.sourceIndex)} disabled={!canControlAudioTracks}>
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
          <p className="settings-empty">This runtime does not expose <code>audioTracks</code>, and no ffmpeg fallback track is available for this file.</p>
        ) : (
          <p className="settings-empty">Load media to inspect audio tracks.</p>
        )}
      </div>
      <div className="settings-section">
        <div className="settings-section-header"><span>Subtitles</span><span>{subtitleStatusText}</span></div>
        {hasSelectedMedia && fallbackSubtitleSnapshots.length > 0 ? (
          <div className="audio-track-list">
            {fallbackSubtitleSnapshots.map((t) => (
              <button key={t.sourceIndex} type="button" className={`audio-track-button ${selectedSubtitleIndex === t.sourceIndex ? "active" : ""}`} onClick={() => onSubtitleTrackActivate(t.sourceIndex)} disabled={!canControlSubtitleTracks}>
                <span className="audio-track-name">{t.label}</span>
                <span className="audio-track-meta">{t.language ? t.language.toUpperCase() : "Unknown language"}</span>
              </button>
            ))}
            <button type="button" className={`audio-track-button ${selectedSubtitleIndex === null ? "active" : ""}`} onClick={() => onSubtitleTrackActivate(null)} disabled={!canControlSubtitleTracks}>
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
          <input id="buffer-window-mb" type="number" min={1} max={1000} step={10} value={editBufferWindowMB} onChange={(e) => { const v = Number(e.target.value); onBufferWindowChange(Number.isFinite(v) ? Math.max(1, Math.min(1000, v)) : 50); }} />
        </div>
        <div className="buffer-settings-row">
          <label htmlFor="max-buffer-mb">Max buffer (MB)</label>
          <input id="max-buffer-mb" type="number" min={10} max={2000} step={10} value={editMaxBufferMB} onChange={(e) => { const v = Number(e.target.value); onBufferMaxChange(Number.isFinite(v) ? Math.max(10, Math.min(2000, v)) : 500); }} />
        </div>
        <button type="button" className="secondary-btn" onClick={() => {
          const win = Number.isFinite(editBufferWindowMB) ? Math.max(1, Math.min(1000, editBufferWindowMB)) : 50;
          const max = Number.isFinite(editMaxBufferMB) ? Math.max(10, Math.min(2000, editMaxBufferMB)) : 500;
          onBufferApply(win, max);
        }}>Apply buffer settings</button>
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
  );
}
