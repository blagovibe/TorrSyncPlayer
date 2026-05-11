import { useState } from "react";
import { Peer, PeerRole } from "../App";
import RoomInfo from "./RoomInfo";
import StatusBar from "./StatusBar";
import VideoPlayer from "./VideoPlayer";
import { RefObject } from "react";
import type { TorrentMediaFile } from "../services/TorrentService";

interface RoomPageProps {
  peerId: string;
  peerRole: PeerRole | null;
  peers: Peer[];
  isConnected: boolean;
  magnetLink: string;
  torrentFileName: string | null;
  mediaFiles: TorrentMediaFile[];
  selectedMediaIndex: number | null;
  selectedMediaLabel: string | null;
  selectedMediaKind: TorrentMediaFile["kind"] | null;
  onMagnetLinkChange: (value: string) => void;
  onTorrentFileChange: (file: File | null) => void;
  videoRef: RefObject<HTMLVideoElement | null>;
  onLoadMagnet: () => void;
  onLoadTorrentFile: () => void;
  onSelectMediaFile: (file: TorrentMediaFile) => void;
  onLeaveRoom: () => void;
  isLoadingTorrent: boolean;
  downloadSpeed: string;
  bufferingProgress: number;
  torrentError: string | null;
}

function RoomPage({
  peerId,
  peerRole,
  peers,
  isConnected,
  magnetLink,
  torrentFileName,
  mediaFiles,
  selectedMediaIndex,
  selectedMediaLabel,
  selectedMediaKind,
  onMagnetLinkChange,
  onTorrentFileChange,
  videoRef,
  onLoadMagnet,
  onLoadTorrentFile,
  onSelectMediaFile,
  onLeaveRoom,
  isLoadingTorrent,
  downloadSpeed,
  bufferingProgress,
  torrentError,
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
          />
          <div className="panel">
            <label htmlFor="room-magnet">Magnet link</label>
            <textarea
              id="room-magnet"
              rows={2}
              value={magnetLink}
              onChange={(event) => onMagnetLinkChange(event.target.value)}
              placeholder="magnet:?xt=urn:btih:..."
            />
            <div className="torrent-actions">
              <button type="button" onClick={onLoadMagnet} disabled={isLoadingTorrent || !magnetLink.trim()}>
                {isLoadingTorrent ? "Loading..." : "Load Magnet"}
              </button>
              <label className="file-picker">
                <input
                  type="file"
                  accept=".torrent,application/x-bittorrent"
                  onChange={(event) => onTorrentFileChange(event.target.files?.[0] ?? null)}
                />
                <span>Choose .torrent</span>
              </label>
            </div>
            <div className="torrent-file-row">
              <span className="torrent-file-name">
                {torrentFileName ? `Selected file: ${torrentFileName}` : "No torrent file selected"}
              </span>
              <button type="button" onClick={onLoadTorrentFile} disabled={isLoadingTorrent || !torrentFileName}>
                Load File
              </button>
            </div>
            {torrentError && <p className="error-text">{torrentError}</p>}
          </div>

          <div className="panel media-library">
            <div className="media-library-header">
              <div>
                <h3>Playable files</h3>
                <p className="hint">
                  Torrent media is auto-picked for convenience. Switch here if you want a different file.
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
                      disabled={isLoadingTorrent}
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
        peerCount={peers.length}
        downloadSpeed={downloadSpeed}
        bufferingProgress={bufferingProgress}
      />
    </section>
  );
}

export default RoomPage;
