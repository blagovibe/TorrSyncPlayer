interface StatusBarProps {
  isConnected: boolean;
  torrentPeerCount: number;
  downloadSpeed: string;
  bufferingProgress: number;
  torrentPeerHint: string;
  bufferHint: string;
}

function StatusBar({
  isConnected,
  torrentPeerCount,
  downloadSpeed,
  bufferingProgress,
  torrentPeerHint,
  bufferHint,
}: StatusBarProps) {
  return (
    <footer className="status-bar panel">
      <span>
        Connection:{" "}
        <strong className={isConnected ? "ok" : "error"}>
          {isConnected ? "connected" : "disconnected"}
        </strong>
      </span>
      <span>Public peers seen: {torrentPeerCount}</span>
      <span>Speed: {downloadSpeed}</span>
      <span>Buffer: {bufferingProgress}%</span>
      <div className="buffer-bar" aria-hidden>
        <div style={{ width: `${bufferingProgress}%` }} />
      </div>
      <div className="status-notes">
        <span className="status-note">{torrentPeerHint}</span>
        <span className="status-note">{bufferHint}</span>
      </div>
    </footer>
  );
}

export default StatusBar;
