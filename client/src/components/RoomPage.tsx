import { Peer, PeerRole } from "../App";
import RoomInfo from "./RoomInfo";
import StatusBar from "./StatusBar";
import VideoPlayer from "./VideoPlayer";
import { RefObject } from "react";

interface RoomPageProps {
  roomCode: string;
  peerRole: PeerRole | null;
  peers: Peer[];
  isConnected: boolean;
  magnetLink: string;
  onMagnetLinkChange: (value: string) => void;
  videoRef: RefObject<HTMLVideoElement | null>;
  onLoadMagnet: () => void;
  onLeaveRoom: () => void;
}

function RoomPage({
  roomCode,
  peerRole,
  peers,
  isConnected,
  magnetLink,
  onMagnetLinkChange,
  videoRef,
  onLoadMagnet,
  onLeaveRoom,
}: RoomPageProps) {
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
            <button type="button" onClick={onLoadMagnet}>
              Load Torrent
            </button>
          </div>
        </div>
        <RoomInfo
          roomCode={roomCode}
          peerRole={peerRole}
          peers={peers}
          onLeaveRoom={onLeaveRoom}
        />
      </div>
      <StatusBar
        isConnected={isConnected}
        peerCount={peers.length}
        downloadSpeed="0 MB/s"
        bufferingProgress={12}
      />
    </section>
  );
}

export default RoomPage;
