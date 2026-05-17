import { FormEvent, useEffect, useRef, useState } from "react";

interface HomePageProps {
  peerId: string;
  onCreateRoom: () => void;
  onJoinRoom: (code: string) => void;
  isConnecting: boolean;
  connectionError: string | null;
  roomPassword: string;
  onRoomPasswordChange: (value: string) => void;
}

function HomePage({
  peerId,
  onCreateRoom,
  onJoinRoom,
  isConnecting,
  connectionError,
  roomPassword,
  onRoomPasswordChange,
}: HomePageProps) {
  const [joinCode, setJoinCode] = useState("");
  const [copied, setCopied] = useState(false);
  const copiedTimerRef = useRef<number | null>(null);

  useEffect(() => {
    return () => {
      if (copiedTimerRef.current !== null) {
        clearTimeout(copiedTimerRef.current);
      }
    };
  }, []);

  const handleJoin = (event: FormEvent) => {
    event.preventDefault();
    if (joinCode.trim().length === 6) {
      onJoinRoom(joinCode.trim().toUpperCase());
    }
  };

  const copyPeerId = async () => {
    if (!peerId) return;
    try {
      await navigator.clipboard.writeText(peerId);
      setCopied(true);
      if (copiedTimerRef.current !== null) {
        clearTimeout(copiedTimerRef.current);
      }
      copiedTimerRef.current = window.setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error("Failed to copy:", err);
    }
  };

  return (
    <section className="home-page">
      <h1 className="app-logo">TorrSyncPlayer</h1>
      <p className="hint">Watch torrents together with friends</p>

      <div className="panel">
        <button className="primary-btn" type="button" onClick={onCreateRoom} disabled={isConnecting}>
          {isConnecting ? (
            <>
              <span className="spinner" aria-hidden="true" /> Creating…
            </>
          ) : (
            "Create Room (Host)"
          )}
        </button>
      </div>

      {peerId && (
        <div className="panel">
          <label>Your Peer ID</label>
          <div className="row">
            <span className="peer-id-display">{peerId}</span>
            <button type="button" onClick={copyPeerId} className="secondary-btn">
              {copied ? "Copied!" : "Copy"}
            </button>
          </div>
          <p className="hint">Share this ID with your friend to connect</p>
        </div>
      )}

      <div className="panel">
        <label htmlFor="room-password">Room password (optional)</label>
        <input
          id="room-password"
          type="password"
          value={roomPassword}
          onChange={(event) => onRoomPasswordChange(event.target.value)}
          placeholder="Leave empty for no password"
          maxLength={32}
        />
        <p className="hint">Guests will need this password to join your room</p>
      </div>

      <form className="panel" onSubmit={handleJoin}>
        <label htmlFor="join-code">Connect to Friend</label>
        <div className="row">
          <input
            id="join-code"
            value={joinCode}
            onChange={(event) => setJoinCode(event.target.value.toUpperCase())}
            placeholder="Enter friend's ID"
            maxLength={6}
            disabled={isConnecting}
          />
          <button type="submit" disabled={isConnecting || joinCode.length !== 6}>
            {isConnecting ? "Connecting..." : "Connect"}
          </button>
        </div>
        {connectionError && <p className="error-text">{connectionError}</p>}
      </form>
    </section>
  );
}

export default HomePage;
