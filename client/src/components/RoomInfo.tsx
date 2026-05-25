import { type Peer, type PeerRole } from "../services/types";
import { useState, useMemo, useEffect, useRef } from "react";

const MAX_CHAT_MESSAGES = 200;

interface RoomInfoProps {
  peerId: string;
  peerRole: PeerRole | null;
  peers: Peer[];
  isConnected: boolean;
  onLeaveRoom: () => void;
  onRequestLeave?: () => void;
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
  onRequestLeave,
  onCopyPeerId,
  copied,
  chatMessages = [],
  onSendChat,
}: RoomInfoProps) {
  const [newMsg, setNewMsg] = useState("");
  const chatScrollRef = useRef<HTMLDivElement>(null);

  const displayMessages = useMemo(() => {
    if (chatMessages.length <= MAX_CHAT_MESSAGES) return chatMessages;
    return chatMessages.slice(-MAX_CHAT_MESSAGES);
  }, [chatMessages]);

  useEffect(() => {
    const el = chatScrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [displayMessages]);

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
          {isConnected ? "Connected" : peerRole === "slave" ? "Waiting for host..." : "Disconnected"}
        </span>
      </p>

      <h3>Chat</h3>
      <div className="chat-messages-scroll" ref={chatScrollRef} aria-live="polite" aria-label="Chat messages">
        {displayMessages.map((msg) => (
          <div
            key={msg.id || `${msg.sender}-${msg.timestamp}`}
            className={msg.sender === peerId ? "chat-message self" : "chat-message peer"}
          >
            <span className="msg-sender">
              {msg.sender === peerId ? "You" : msg.sender}
            </span>
            <p className="msg-text">
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
      >
        <input
          type="text"
          className="chat-input"
          placeholder="Type a message"
          value={newMsg}
          onChange={(e) => setNewMsg(e.target.value)}
          disabled={!isConnected}
        />
        <button type="submit" className="chat-send" disabled={!newMsg.trim() || !isConnected}>
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

      <button className="danger-btn" type="button" onClick={onRequestLeave ?? onLeaveRoom}>
        Leave Room
      </button>
    </aside>
  );
}

export default RoomInfo;