import { type RefObject, useState } from "react";
import { Peer, PeerRole } from "../App";
import RoomInfo from "./RoomInfo";
import StatusBar from "./StatusBar";
import VideoPlayer from "./VideoPlayer";
import type { TorrentMediaFile } from "../services/TorrentService";
import type { AudioTrackInfo } from "../services/types";

interface RoomPageProps {
  peerId: string;
  peerRole: PeerRole | null;
  peers: Peer[];
  isConnected: boolean;
  canControlTorrent: boolean;
  magnetLink: string;
  torrentFileName: string | null;
  sharedSourceLabel: string | null;
  mediaFiles: TorrentMediaFile[];
  selectedMediaIndex: number | null;
  selectedMediaLabel: string | null;
  selectedMediaKind: TorrentMediaFile["kind"] | null;
  selectedMediaAudioTracks: AudioTrackInfo[];
  torrentPeerCount: number;
  syncToleranceSeconds: number;
  onSyncToleranceChange: (value: number) => void;
  onMagnetLinkChange: (value: string) => void;
  onTorrentFileChange: (file: File | null) => void;
  videoRef: RefObject<HTMLVideoElement | null>;
  playbackNotice: string | null;
  selectedAudioTrackIndex: number | null;
  onPlaybackStarted: () => void;
  onPlayerReady: (ready: boolean) => void;
  onAudioTrackChange: (trackIndex: number | null) => void;
  resolveFallbackAudioTrackSource: (trackIndex: number, startSeconds: number) => Promise<string | null>;
  onLoadMagnet: () => void;
  onLoadTorrentFile: () => void;
  onSelectMediaFile: (file: TorrentMediaFile) => void;
  onLeaveRoom: () => void;
  isLoadingTorrent: boolean;
  downloadSpeed: string;
  bufferingProgress: number;
  torrentError: string | null;
  torrentPeerHint: string;
  bufferHint: string;
  isBuffering: boolean;
  onBufferingChange?: (isBuffering: boolean) => void;
}

function RoomPage({
  peerId,
  peerRole,
  peers,
  isConnected,
  canControlTorrent,
  magnetLink,
  torrentFileName,
  sharedSourceLabel,
  mediaFiles,
  selectedMediaIndex,
  selectedMediaLabel,
  selectedMediaKind,
  selectedMediaAudioTracks,
  torrentPeerCount,
  syncToleranceSeconds,
  onSyncToleranceChange,
  onMagnetLinkChange,
  onTorrentFileChange,
  videoRef,
  playbackNotice,
  selectedAudioTrackIndex,
  onPlaybackStarted,
  onPlayerReady,
  onAudioTrackChange,
  resolveFallbackAudioTrackSource,
  onLoadMagnet,
  onLoadTorrentFile,
  onSelectMediaFile,
  onLeaveRoom,
  isLoadingTorrent,
  downloadSpeed,
  bufferingProgress,
  torrentError,
  torrentPeerHint,
  bufferHint,
  isBuffering: _isBuffering,
  onBufferingChange,
}: RoomPageProps) {
  const [copied, setCopied] = useState(false);

  const copyPeerId = async () => {
    if (!peerId) return;
    try {
      await navigator.clipboard.writeText(peerId);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error("Failed to copy:", err);
    }
  };

  return (
    <section className="room-page">
      <div className="room-layout">
        <div className="player-column">
          <VideoPlayer
            videoRef={videoRef}
            mediaLabel={selectedMediaLabel}
            mediaKind={selectedMediaKind}
            statusMessage={playbackNotice}
            canControlPlayback={canControlTorrent || playbackNotice !== null}
            canControlAudioTracks={canControlTorrent}
            fallbackAudioTracks={selectedMediaAudioTracks}
            selectedAudioTrackIndex={selectedAudioTrackIndex}
            onPlaybackStart={onPlaybackStarted}
            onAudioTrackChange={onAudioTrackChange}
            resolveFallbackAudioTrackSource={resolveFallbackAudioTrackSource}
            onPlayerReady={onPlayerReady}
            onBufferingChange={onBufferingChange}
          />
          <div className="panel">
            <div className="room-controls-header">
              <div>
                <h3>Room source</h3>
                <p className="hint">
                  {canControlTorrent
                    ? "Host loads the torrent once, then the room follows the same media source."
                    : "The host will load the torrent and everyone else will mirror it automatically."}
                </p>
              </div>
              <span className={`room-role-badge ${canControlTorrent ? "host" : "guest"}`}>
                {canControlTorrent ? "Host controls" : "Guest view"}
              </span>
            </div>

            <label htmlFor="room-magnet">Magnet link</label>
            <textarea
              id="room-magnet"
              rows={2}
              value={magnetLink}
              onChange={(event) => onMagnetLinkChange(event.target.value)}
              placeholder="magnet:?xt=urn:btih:..."
              disabled={!canControlTorrent}
            />
            <div className="torrent-actions">
              <button
                type="button"
                onClick={onLoadMagnet}
                disabled={!canControlTorrent || isLoadingTorrent || !magnetLink.trim()}
              >
                {isLoadingTorrent ? "Loading metadata..." : "Load Magnet"}
              </button>
              <label className={`file-picker ${canControlTorrent && !isLoadingTorrent ? "" : "disabled"}`}>
                <input
                  type="file"
                  accept=".torrent,application/x-bittorrent"
                  onChange={(event) => {
                    onTorrentFileChange(event.target.files?.[0] ?? null);
                    event.currentTarget.value = "";
                  }}
                  disabled={!canControlTorrent || isLoadingTorrent}
                />
                <span>Choose .torrent</span>
              </label>
            </div>
            <div className="torrent-file-row">
              <span className="torrent-file-name">
                {sharedSourceLabel
                  ? `Shared source: ${sharedSourceLabel}`
                  : torrentFileName
                    ? `Selected file: ${torrentFileName}`
                    : canControlTorrent
                      ? "No torrent file selected"
                      : "Waiting for host to load the shared source"}
              </span>
              <button
                type="button"
                onClick={onLoadTorrentFile}
                disabled={!canControlTorrent || isLoadingTorrent || !torrentFileName}
              >
                {isLoadingTorrent ? "Loading metadata..." : "Load File"}
              </button>
            </div>
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
                step="0.1"
                value={syncToleranceSeconds}
                onChange={(event) => onSyncToleranceChange(Number(event.target.value))}
                disabled={!canControlTorrent}
              />
              <p className="hint">
                Guests stay within this drift window before the player is corrected back to the host timecode.
              </p>
            </div>
          </div>

          <div className="panel media-library">
            <div className="media-library-header">
              <div>
                <h3>Playable files</h3>
                <p className="hint">
                  {canControlTorrent
                    ? "Torrent media is auto-picked for convenience. Switch here if you want a different file."
                    : "The host controls the selected file, and this list mirrors the shared torrent state."}
                </p>
              </div>
              <span className="media-count">{mediaFiles.length} found</span>
            </div>

            {mediaFiles.length > 0 ? (
              <div className="media-list" role="list">
                {mediaFiles.map((file, index) => {
                  const isActive = selectedMediaIndex === file.index || (selectedMediaIndex === null && index === 0);
                  return (
                    <button
                      key={`${file.index}-${file.name}`}
                      type="button"
                      className={`media-item ${isActive ? "active" : ""}`}
                      onClick={() => onSelectMediaFile(file)}
                      disabled={!canControlTorrent || isLoadingTorrent}
                    >
                      <div className="media-item-main">
                        <span className="media-item-title">{file.name}</span>
                        <span className="media-item-subtitle">
                          {file.kind === "video" ? "Video" : "Audio"} file
                        </span>
                      </div>
                      <div className="media-item-meta">
                        <span className="media-pill">{file.kind}</span>
                        <span className="media-size">
                          {file.length > 0 ? `${(file.length / 1024 / 1024).toFixed(file.length >= 1024 * 1024 * 100 ? 0 : file.length >= 1024 * 1024 * 10 ? 1 : 2)} MB` : "Unknown size"}
                        </span>
                      </div>
                    </button>
                  );
                })}
              </div>
            ) : (
              <div className="media-empty">
                <p>No playable video or audio files found yet.</p>
                <p className="hint">Load a torrent and this list will populate automatically.</p>
              </div>
            )}
          </div>
        </div>
        <RoomInfo
          peerId={peerId}
          peerRole={peerRole}
          peers={peers}
          isConnected={isConnected}
          onLeaveRoom={onLeaveRoom}
          onCopyPeerId={copyPeerId}
          copied={copied}
        />
      </div>
      <StatusBar
        isConnected={isConnected}
        torrentPeerCount={torrentPeerCount}
        downloadSpeed={downloadSpeed}
        bufferingProgress={bufferingProgress}
        torrentPeerHint={torrentPeerHint}
        bufferHint={bufferHint}
      />
      </section>
  );
}

export default RoomPage;
