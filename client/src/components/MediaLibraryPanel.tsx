import type { TorrentMediaFile } from "../services/TorrentService";
import { formatBytes } from "../utils/format";

interface MediaLibraryPanelProps {
  mediaFiles: TorrentMediaFile[];
  selectedMediaIndex: number | null;
  selectedMediaLabel: string | null;
  selectedMediaKind: TorrentMediaFile["kind"] | null;
  isLoadingTorrent: boolean;
  isHost: boolean;
  onSelectMediaFile: (file: TorrentMediaFile) => void;
}

function MediaLibraryPanel({
  mediaFiles,
  selectedMediaIndex,
  selectedMediaLabel,
  selectedMediaKind,
  isLoadingTorrent,
  isHost,
  onSelectMediaFile,
}: MediaLibraryPanelProps) {
  if (isHost) {
    return (
      <div className="panel media-library">
        <div className="media-library-header">
          <div>
            <h3>Playable files</h3>
            <p className="hint">
              Torrent media is auto-picked for convenience. Switch here if you want a different file.
            </p>
          </div>
          <span className="media-count">{mediaFiles.length} found</span>
        </div>

        {mediaFiles.length > 0 ? (
          <div className="media-list" role="list">
            {mediaFiles.map((file, index) => {
              const isActive = selectedMediaIndex === file.index || (selectedMediaIndex === null && index === 0);
              return (
                <button
                  key={`${file.index}-${file.name}`}
                  type="button"
                  className={`media-item ${isActive ? "active" : ""}`}
                  onClick={() => onSelectMediaFile(file)}
                  disabled={isLoadingTorrent}
                >
                  <div className="media-item-main">
                    <span className="media-item-title">{file.name}</span>
                    <span className="media-item-subtitle">
                      {file.kind === "video" ? "Video" : "Audio"} file
                    </span>
                  </div>
                  <div className="media-item-meta">
                    <span className="media-pill">{file.kind}</span>
                    <span className="media-size">
                      {file.length > 0 ? formatBytes(file.length) : "Unknown size"}
                    </span>
                  </div>
                </button>
              );
            })}
          </div>
        ) : (
          <div className="media-empty">
            <p>No playable video or audio files found yet.</p>
            <p className="hint">Load a torrent and this list will populate automatically.</p>
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="panel media-library">
      <div className="media-library-header">
        <div>
          <h3>Now playing</h3>
          <p className="hint">The host controls which file is playing.</p>
        </div>
      </div>
      {selectedMediaLabel ? (
        <div className="media-list" role="list">
          <div className="media-item active">
            <div className="media-item-main">
              <span className="media-item-title">{selectedMediaLabel}</span>
              <span className="media-item-subtitle">
                {selectedMediaKind === "video" ? "Video" : "Audio"} file
              </span>
            </div>
            <div className="media-item-meta">
              <span className="media-pill">{selectedMediaKind ?? "video"}</span>
            </div>
          </div>
        </div>
      ) : (
        <div className="media-empty">
          <p>No file selected yet.</p>
          <p className="hint">The host will choose a file from the torrent.</p>
        </div>
      )}
    </div>
  );
}

export default MediaLibraryPanel;
