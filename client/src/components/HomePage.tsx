import { FormEvent, useState } from "react";

interface HomePageProps {
  peerId: string;
  onCreateRoom: () => void;
  onJoinRoom: (code: string) => void;
  isConnecting: boolean;
  connectionError: string | null;
}

function HomePage({
  peerId,
  onCreateRoom,
  onJoinRoom,
  isConnecting,
  connectionError,
}: HomePageProps) {
  const [joinCode, setJoinCode] = useState("");
  const [copied, setCopied] = useState(false);

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
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error("Failed to copy:", err);
    }
  };

  return (
    <section className="home-page">
      <h1 className="app-logo">TorrSyncPlayer</h1>
      <p className="hint">Watch torrents together with friends</p>

      <div className="panel">
        <button className="primary-btn" type="button" onClick={onCreateRoom}>
          Create Room (Host)
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