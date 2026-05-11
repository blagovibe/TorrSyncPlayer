interface StatusBarProps {
  isConnected: boolean;
  torrentPeerCount: number;
  downloadSpeed: string;
  bufferingProgress: number;
}

function StatusBar({
  isConnected,
  torrentPeerCount,
  downloadSpeed,
  bufferingProgress,
}: StatusBarProps) {
  return (
    <footer className="status-bar panel">
      <span>
        Connection:{" "}
        <strong className={isConnected ? "ok" : "error"}>
          {isConnected ? "connected" : "disconnected"}
        </strong>
      </span>
      <span>Torrent peers: {torrentPeerCount}</span>
      <span>Speed: {downloadSpeed}</span>
      <span>Buffer: {bufferingProgress}%</span>
      <div className="buffer-bar" aria-hidden>
        <div style={{ width: `${bufferingProgress}%` }} />
      </div>
    </footer>
  );
}

export default StatusBar;
