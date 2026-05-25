import { type ChangeEvent, type FormEvent, useCallback, useState } from "react";
import { isValidMagnetLink } from "../utils/torrent";

interface TorrentControlsPanelProps {
  magnetLink: string;
  torrentFileName: string | null;
  sharedSourceLabel: string | null;
  isLoadingTorrent: boolean;
  torrentError: string | null;
  syncToleranceSeconds: number;
  onMagnetLinkChange: (value: string) => void;
  onTorrentFileChange: (file: File | null) => void;
  onLoadMagnet: () => void;
  onLoadTorrentFile: () => void;
  onSyncToleranceChange: (value: number) => void;
  onResetTorrentInRoom?: () => void;
  onShowResetConfirm?: () => void;
}

function TorrentControlsPanel({
  magnetLink,
  torrentFileName,
  sharedSourceLabel,
  isLoadingTorrent,
  torrentError,
  syncToleranceSeconds,
  onMagnetLinkChange,
  onTorrentFileChange,
  onLoadMagnet,
  onLoadTorrentFile,
  onSyncToleranceChange,
  onResetTorrentInRoom,
  onShowResetConfirm,
}: TorrentControlsPanelProps) {
  const [magnetValidation, setMagnetValidation] = useState<{ valid: boolean; message: string } | null>(null);

  const handleMagnetChange = useCallback((event: ChangeEvent<HTMLTextAreaElement>) => {
    const value = event.target.value.slice(0, 2000);
    onMagnetLinkChange(value);
    if (value.trim().length > 0) {
      const valid = isValidMagnetLink(value.trim());
      setMagnetValidation(valid ? { valid: true, message: "Valid magnet link format" } : { valid: false, message: "Invalid magnet link format" });
    } else {
      setMagnetValidation(null);
    }
  }, [onMagnetLinkChange]);

  const handleSyncToleranceChange = useCallback((event: ChangeEvent<HTMLInputElement>) => {
    const val = Number(event.target.value);
    onSyncToleranceChange(Number.isFinite(val) ? val : 0);
  }, [onSyncToleranceChange]);

  const handleTorrentFileInputChange = useCallback((event: FormEvent<HTMLInputElement>) => {
    const target = event.currentTarget as HTMLInputElement & { files: FileList | null };
    onTorrentFileChange(target.files?.[0] ?? null);
    event.currentTarget.value = "";
  }, [onTorrentFileChange]);

  return (
    <div className="panel">
      <div className="room-controls-header">
        <div>
          <h3>Room source</h3>
          <p className="hint">
            Host loads the torrent once, then the room follows the same media source.
          </p>
        </div>
        <span className="room-role-badge host">Host controls</span>
      </div>

      <label htmlFor="room-magnet">Magnet link</label>
      <textarea
        id="room-magnet"
        rows={2}
        value={magnetLink}
        onChange={handleMagnetChange}
        placeholder="magnet:?xt=urn:btih:..."
        className={magnetValidation ? (magnetValidation.valid ? "valid" : "invalid") : ""}
      />
      {magnetValidation && (
        <p className={`hint ${magnetValidation.valid ? "valid-text" : "error-text"}`}>
          {magnetValidation.message}
        </p>
      )}
      <div className="torrent-actions">
        <button
          type="button"
          onClick={onLoadMagnet}
          disabled={isLoadingTorrent || !magnetLink.trim()}
        >
          {isLoadingTorrent ? "Loading metadata..." : "Load Magnet"}
        </button>
        <label className={`file-picker ${!isLoadingTorrent ? "" : "disabled"}`}>
          <input
            type="file"
            accept=".torrent,application/x-bittorrent"
            onChange={handleTorrentFileInputChange}
            disabled={isLoadingTorrent}
          />
          <span>Choose .torrent</span>
        </label>
      </div>
      <div className="torrent-file-row">
        <span className="torrent-file-name">
          {sharedSourceLabel
            ? `Shared source: ${sharedSourceLabel.length > 120 ? sharedSourceLabel.slice(0, 120) + '…' : sharedSourceLabel}`
            : torrentFileName
              ? `Selected file: ${torrentFileName.length > 120 ? torrentFileName.slice(0, 120) + '…' : torrentFileName}`
              : "No torrent file selected"}
        </span>
        <button
          type="button"
          onClick={onLoadTorrentFile}
          disabled={isLoadingTorrent || !torrentFileName}
        >
          {isLoadingTorrent ? "Loading metadata..." : "Load File"}
        </button>
      </div>
      {sharedSourceLabel && onResetTorrentInRoom && onShowResetConfirm && (
        <button
          type="button"
          onClick={() => onShowResetConfirm()}
          disabled={isLoadingTorrent}
        >
          {isLoadingTorrent ? <><span className="spinner" /> Changing...</> : "Change Source"}
        </button>
      )}
      {isLoadingTorrent && (
        <p className="hint">
          Fetching torrent metadata and public peer count...
        </p>
      )}
      {torrentError && <p className="error-text">{torrentError}</p>}
      <div className="sync-tolerance-row">
        <label htmlFor="sync-tolerance">Sync tolerance, seconds</label>
        <input
          id="sync-tolerance"
          type="number"
          min="0"
          max="30"
          step="0.1"
          value={syncToleranceSeconds}
          onChange={handleSyncToleranceChange}
        />
        <p className="hint">
          Guests stay within this drift window before the player is corrected back to the host timecode.
        </p>
      </div>
    </div>
  );
}

export default TorrentControlsPanel;
