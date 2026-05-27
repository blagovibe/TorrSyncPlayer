import { type FormEvent, useEffect, useRef, useState } from "react";
import { uiLogger } from "../utils/logger";

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
  const [joinCodeError, setJoinCodeError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);

  const isValidJoinCode = (code: string): boolean => {
    return /^[A-Z0-9]{6}$/.test(code);
  };
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
    const code = joinCode.trim().toUpperCase();
    if (code.length === 0) {
      setJoinCodeError("Please enter a 6-character code");
    } else if (code.length !== 6) {
      setJoinCodeError("Code must be exactly 6 characters");
    } else if (!isValidJoinCode(code)) {
      setJoinCodeError("Code must contain only letters A-Z and digits 0-9");
    } else {
      setJoinCodeError(null);
      onJoinRoom(code);
    }
  };

  const handleJoinCodeChange = (value: string) => {
    setJoinCode(value.toUpperCase());
    if (joinCodeError) {
      const upper = value.toUpperCase();
      if (upper.length === 6 && !isValidJoinCode(upper)) {
        setJoinCodeError("Code must contain only letters A-Z and digits 0-9");
      } else {
        setJoinCodeError(null);
      }
    }
  };

  const copyPeerId = async () => {
    if (!peerId) return;
    try {
      await navigator.clipboard.writeText(peerId);
      setCopied(true);
      if (copiedTimerRef.current !== null) clearTimeout(copiedTimerRef.current);
      copiedTimerRef.current = window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopyFailed(true);
      setCopied(false);
      uiLogger.error("Failed to copy peer ID — please copy manually");
      window.setTimeout(() => setCopyFailed(false), 4000);
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
          {copyFailed && <p className="error-text" role="alert">Failed to copy. Please select and copy the ID manually.</p>}
        </div>
      )}

      <form className="panel" onSubmit={handleJoin}>
        <label htmlFor="join-code">Connect to Friend</label>
        <div className="row">
          <input
            id="join-code"
            value={joinCode}
            onChange={(event) => handleJoinCodeChange(event.target.value)}
            placeholder="Enter friend's ID"
            maxLength={6}
            disabled={isConnecting}
            pattern="[A-Za-z0-9]*"
            title="Only letters A-Z and digits 0-9 are allowed"
          />
          <button type="submit" disabled={isConnecting || !isValidJoinCode(joinCode)}>
            {isConnecting ? "Connecting..." : "Connect"}
          </button>
        </div>
        {joinCodeError && <p className="error-text">{joinCodeError}</p>}
        {connectionError && <p className="error-text">{connectionError}</p>}
      </form>
    </section>
  );
}

export default HomePage;
