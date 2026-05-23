import { type Peer, type PeerRole } from "../services/types";
import { useState } from "react";

interface RoomInfoProps {
  peerId: string;
  peerRole: PeerRole | null;
  peers: Peer[];
  isConnected: boolean;
  onLeaveRoom: () => void;
  onCopyPeerId: () => void;
  copied: boolean;
  chatMessages?: { id?: string; sender: string; text: string; timestamp: number }[];
  onSendChat?: (text: string) => void;
}

function RoomInfo({
  peerId,
  peerRole,
  peers,
  isConnected,
  onLeaveRoom,
  onCopyPeerId,
  copied,
  chatMessages = [],
  onSendChat,
}: RoomInfoProps) {
  const isGuest = peerRole === "slave";
  const [newMsg, setNewMsg] = useState("");
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

      <h3>Chat</h3>
      <div
        className="chat-messages-scroll"
        style={{ maxHeight: "200px", overflowY: "auto", marginTop: "0.5rem", marginBottom: "0.5rem" }}
      >
        {chatMessages.map((msg) => (
          <div
            key={msg.id || `${msg.sender}-${msg.timestamp}`}
            className={msg.sender === peerId ? "chat-message self" : "chat-message peer"}
            style={{ textAlign: msg.sender === peerId ? "right" : "left", margin: "0.25rem 0" }}
          >
            <span
              className="msg-sender"
              style={{ fontSize: "0.75rem", opacity: 0.7, display: "block" }}
            >
              {msg.sender === peerId ? "You" : msg.sender}
            </span>
            <p
              className="msg-text"
              style={{
                display: "inline-block",
                background: "var(--bg-secondary, #1e293b)",
                borderRadius: "0.5rem",
                padding: "0.25rem 0.5rem",
                margin: 0,
                wordBreak: "break-word",
              }}
            >
              {msg.text}
            </p>
          </div>
        ))}
      </div>
      <form
        className="chat-input-row"
        onSubmit={(e) => {
          e.preventDefault();
          if (newMsg.trim() && onSendChat) {
            onSendChat(newMsg.trim());
            setNewMsg("");
          }
        }}
        style={{ display: "flex", gap: "0.5rem", marginBottom: "1rem" }}
      >
        <input
          type="text"
          className="chat-input"
          placeholder="Type a message"
          value={newMsg}
          onChange={(e) => setNewMsg(e.target.value)}
          disabled={!isConnected}
          style={{ flex: 1, padding: "0.35rem 0.5rem", borderRadius: "0.25rem", border: "1px solid #475569", background: "var(--bg-primary, #0f172a)", color: "inherit" }}
        />
        <button type="submit" className="chat-send" disabled={!newMsg.trim() || !isConnected} style={{ padding: "0.35rem 0.75rem", borderRadius: "0.25rem" }}>
          Send
        </button>
      </form>

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