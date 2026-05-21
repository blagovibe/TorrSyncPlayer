import { type Peer, type PeerRole } from "../services/types";

interface RoomInfoProps {
  peerId: string;
  peerRole: PeerRole | null;
  peers: Peer[];
  isConnected: boolean;
  onLeaveRoom: () => void;
  onCopyPeerId: () => void;
  copied: boolean;
}

function RoomInfo({ peerId, peerRole, peers, isConnected, onLeaveRoom, onCopyPeerId, copied }: RoomInfoProps) {
  const isGuest = peerRole === "slave";
  return (
    <aside className="room-info panel">
      <h2>Room</h2>
      <div className="room-code-block">
        <span className="room-code">{peerId || "------"}</span>
        <button type="button" onClick={onCopyPeerId}>
          {copied ? "Copied!" : "Copy"}
        </button>
      </div>
      <p className="hint">Your role: {peerRole ?? "unknown"}</p>
      <p className="hint">
        Status:{" "}
        <span className={`connection-status ${isConnected ? "connected" : "disconnected"}`}>
          {isConnected ? "Connected" : isGuest ? "Waiting for host..." : "Disconnected"}
        </span>
      </p>

      <h3>Peers ({peers.length})</h3>
      <ul className="peer-list">
        {peers.map((peer) => (
          <li key={peer.id}>
            <span>{peer.name}</span>
            <div className="peer-meta">
              <span className={`peer-state ${peer.connectionState}`}>
                {peer.connectionState}
              </span>
              <span className={`peer-role ${peer.role}`}>{peer.role}</span>
            </div>
          </li>
        ))}
      </ul>

      <button className="danger-btn" type="button" onClick={onLeaveRoom}>
        Leave Room
      </button>
    </aside>
  );
}

export default RoomInfo;