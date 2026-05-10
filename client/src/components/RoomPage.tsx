import { useState } from "react";
import { Peer, PeerRole } from "../App";
import RoomInfo from "./RoomInfo";
import StatusBar from "./StatusBar";
import VideoPlayer from "./VideoPlayer";
import { RefObject } from "react";

interface RoomPageProps {
  peerId: string;
  peerRole: PeerRole | null;
  peers: Peer[];
  isConnected: boolean;
  magnetLink: string;
  onMagnetLinkChange: (value: string) => void;
  videoRef: RefObject<HTMLVideoElement | null>;
  onLoadMagnet: () => void;
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
  onMagnetLinkChange,
  videoRef,
  onLoadMagnet,
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
          <VideoPlayer videoRef={videoRef} />
          <div className="panel">
            <label htmlFor="room-magnet">Magnet link</label>
            <textarea
              id="room-magnet"
              rows={2}
              value={magnetLink}
              onChange={(event) => onMagnetLinkChange(event.target.value)}
              placeholder="magnet:?xt=urn:btih:..."
            />
            <button type="button" onClick={onLoadMagnet} disabled={isLoadingTorrent}>
              {isLoadingTorrent ? "Loading..." : "Load Torrent"}
            </button>
            {torrentError && <p className="error-text">{torrentError}</p>}
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