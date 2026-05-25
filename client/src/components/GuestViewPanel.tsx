interface GuestViewPanelProps {
  sharedSourceLabel: string | null;
  torrentFileName: string | null;
  isLoadingTorrent: boolean;
  torrentError: string | null;
  onRequestResend?: () => void;
}

function GuestViewPanel({
  sharedSourceLabel,
  torrentFileName,
  isLoadingTorrent,
  torrentError,
  onRequestResend,
}: GuestViewPanelProps) {
  return (
    <div className="panel">
      <div className="room-controls-header">
        <div>
          <h3>Torrent source</h3>
          <p className="hint">
            The host controls the torrent source. You will automatically mirror their playback.
          </p>
        </div>
        <span className="room-role-badge guest">Guest view</span>
      </div>
      <div className="guest-source-info">
        <span className="torrent-file-name">
          {sharedSourceLabel
            ? `Connected to: ${sharedSourceLabel.length > 120 ? sharedSourceLabel.slice(0, 120) + '…' : sharedSourceLabel}`
            : torrentFileName
              ? `Selected file: ${torrentFileName.length > 120 ? torrentFileName.slice(0, 120) + '…' : torrentFileName}`
              : "Waiting for host to load the shared source"}
        </span>
        {isLoadingTorrent && (
          <p className="hint">Loading torrent metadata...</p>
        )}
        {torrentError && (
          <>
            <p className="error-text">{torrentError}</p>
            {onRequestResend && (
              <button type="button" onClick={onRequestResend}>
                Request Resend
              </button>
            )}
          </>
        )}
      </div>
    </div>
  );
}

export default GuestViewPanel;
