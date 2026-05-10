import { Peer, PeerRole } from "../App";

interface RoomInfoProps {
  roomCode: string;
  peerRole: PeerRole | null;
  peers: Peer[];
  onLeaveRoom: () => void;
}

function RoomInfo({ roomCode, peerRole, peers, onLeaveRoom }: RoomInfoProps) {
  const copyRoomCode = async () => {
    try {
      await navigator.clipboard.writeText(roomCode);
    } catch (error) {
      console.error("Failed to copy room code", error);
    }
  };

  return (
    <aside className="room-info panel">
      <h2>Room</h2>
      <div className="room-code-block">
        <span className="room-code">{roomCode || "------"}</span>
        <button type="button" onClick={copyRoomCode}>
          Copy
        </button>
      </div>
      <p className="hint">Your role: {peerRole ?? "unknown"}</p>

      <h3>Peers ({peers.length})</h3>
      <ul className="peer-list">
        {peers.map((peer) => (
          <li key={peer.id}>
            <span>{peer.name}</span>
            <span className={`peer-role ${peer.role}`}>{peer.role}</span>
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
