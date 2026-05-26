import { type RefObject, useCallback, useEffect, useRef, useState } from "react";
import { type Peer, type PeerRole } from "../services/types";
import { uiLogger } from "../utils/logger";
import { RoomErrorBoundary } from "./RoomErrorBoundary";
import RoomInfo from "./RoomInfo";
import StatusBar from "./StatusBar";
import VideoPlayer from "./VideoPlayer";
import TorrentControlsPanel from "./TorrentControlsPanel";
import GuestViewPanel from "./GuestViewPanel";
import MediaLibraryPanel from "./MediaLibraryPanel";
import type { TorrentMediaFile } from "../services/TorrentService";
import type { AudioTrackInfo, ConnectionQuality, SubtitleTrackInfo } from "../services/types";

interface ConnectionState {
  peerId: string;
  peerRole: PeerRole | null;
  peers: Peer[];
  isConnected: boolean;
  connectionQuality?: ConnectionQuality;
  rttMs?: number | null;
}

interface TorrentState {
  magnetLink: string;
  torrentFileName: string | null;
  sharedSourceLabel: string | null;
  mediaFiles: TorrentMediaFile[];
  selectedMediaIndex: number | null;
  selectedMediaLabel: string | null;
  selectedMediaKind: TorrentMediaFile["kind"] | null;
  selectedMediaAudioTracks: AudioTrackInfo[];
  selectedAudioTrackIndex: number | null;
  selectedSubtitles: SubtitleTrackInfo[];
  selectedSubtitleIndex: number | null;
  isLoadingTorrent: boolean;
  downloadSpeed: string;
  bufferingProgress: number;
  torrentPeerCount: number;
  torrentError: string | null;
  torrentPeerHint: string;
  bufferHint: string;
  trackerLost?: boolean;
  bufferWindowMB?: number;
  maxBufferMB?: number;
}

interface PlayerState {
  videoRef: RefObject<HTMLVideoElement | null>;
  playbackNotice: string | null;
  syncToleranceSeconds: number;
  canControl: boolean;
}

interface ChatState {
  chatMessages?: { id?: string; sender: string; text: string; timestamp: number }[];
  onSendChat?: (text: string) => void;
}

interface RoomPageProps {
  connection: ConnectionState;
  torrent: TorrentState;
  player: PlayerState;
  chat: ChatState;
  ffmpegAvailable?: boolean | null;
  browserModeWarning?: boolean;
  onMagnetLinkChange: (value: string) => void;
  onTorrentFileChange: (file: File | null) => void;
  onPlaybackStarted: () => void;
  onPlayerReady: (ready: boolean) => void;
  onAudioTrackChange: (trackIndex: number | null) => void;
  onSubtitleTrackChange: (trackIndex: number | null) => void;
  onLoadMagnet: () => void;
  onLoadTorrentFile: () => void;
  onSelectMediaFile: (file: TorrentMediaFile) => void;
  onLeaveRoom: () => void;
  onBufferingChange?: (isBuffering: boolean) => void;
  onResetTorrentInRoom?: () => void;
  onTimeUpdate?: (currentTime: number, duration: number) => void;
  onBufferSettingsChange?: (bufferWindowMB: number, maxBufferMB: number) => void;
  onSyncToleranceChange: (value: number) => void;
  onSeek?: (timestamp: number) => void;
  onMuxStreamRequest?: (startSeconds: number) => Promise<string | null>;
  onShowLeaveConfirm?: () => void;
  onShowResetConfirm?: () => void;
  onReturnHome?: () => void;
  onRequestResend?: () => void;
}

function RoomPage({
  connection,
  torrent,
  player,
  chat,
  ffmpegAvailable,
  browserModeWarning,
  onMagnetLinkChange,
  onTorrentFileChange,
  onPlaybackStarted,
  onPlayerReady,
  onAudioTrackChange,
  onSubtitleTrackChange,
  onLoadMagnet,
  onLoadTorrentFile,
  onSelectMediaFile,
  onLeaveRoom,
  onBufferingChange,
  onResetTorrentInRoom,
  onTimeUpdate,
  onBufferSettingsChange,
  onSyncToleranceChange,
  onSeek,
  onMuxStreamRequest,
  onShowLeaveConfirm,
  onShowResetConfirm,
  onReturnHome,
  onRequestResend,
}: RoomPageProps) {
  const {
    peerId, peerRole, peers, isConnected, connectionQuality, rttMs,
  } = connection;
  const {
    magnetLink, torrentFileName, sharedSourceLabel, mediaFiles,
    selectedMediaIndex, selectedMediaLabel, selectedMediaKind,
    selectedMediaAudioTracks, selectedAudioTrackIndex,
    selectedSubtitles, selectedSubtitleIndex,
    isLoadingTorrent, downloadSpeed, bufferingProgress,
    torrentPeerCount, torrentError, torrentPeerHint, bufferHint, trackerLost,
    bufferWindowMB, maxBufferMB,
  } = torrent;
  const {
    videoRef, playbackNotice, syncToleranceSeconds, canControl: canControlTorrent,
  } = player;
  const { chatMessages, onSendChat } = chat;
  const [copied, setCopied] = useState(false);
  const [isDragOver, setIsDragOver] = useState(false);
  const copiedTimerRef = useRef<number | null>(null);
  const dragCounterRef = useRef(0);
  const canControlTorrentRef = useRef(canControlTorrent);
  canControlTorrentRef.current = canControlTorrent;

  useEffect(() => {
    return () => {
      if (copiedTimerRef.current !== null) {
        clearTimeout(copiedTimerRef.current);
      }
    };
  }, []);

  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current++;
    if (e.dataTransfer.types.includes("Files")) {
      setIsDragOver(true);
    }
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current = Math.max(0, dragCounterRef.current - 1);
    if (dragCounterRef.current === 0) {
      setIsDragOver(false);
    }
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  }, []);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);
    dragCounterRef.current = 0;
    if (!canControlTorrentRef.current) return;
    const files = e.dataTransfer.files;
    if (files.length > 0) {
      const torrentFile = Array.from(files).find(
        (f) => f.name.endsWith(".torrent") || f.type === "application/x-bittorrent"
      );
      if (torrentFile) {
        onTorrentFileChange(torrentFile);
      }
    }
  }, [onTorrentFileChange]);

  const copyPeerId = async () => {
    if (!peerId) return;
    try {
      await navigator.clipboard.writeText(peerId);
      setCopied(true);
      if (copiedTimerRef.current !== null) {
        clearTimeout(copiedTimerRef.current);
      }
      copiedTimerRef.current = window.setTimeout(() => setCopied(false), 2000);
    } catch {
      try {
        const textArea = document.createElement("textarea");
        textArea.value = peerId;
        document.body.appendChild(textArea);
        textArea.select();
        textArea.setSelectionRange(0, 99999);
        await navigator.clipboard.writeText(peerId);
        setCopied(true);
        if (copiedTimerRef.current !== null) {
          clearTimeout(copiedTimerRef.current);
        }
        copiedTimerRef.current = window.setTimeout(() => setCopied(false), 2000);
        document.body.removeChild(textArea);
      } catch {
        uiLogger.error("Failed to copy peer ID — please copy manually");
      }
    }
  };

  return (
    <RoomErrorBoundary onReturnHome={onReturnHome ?? (() => undefined)}>
    <section
      className={`room-page ${isDragOver ? "drag-over" : ""}`}
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
    >
      {isDragOver && canControlTorrent && (
        <div className="drag-overlay">
          <div className="drag-overlay-content">
             <span className="drag-icon" aria-label="Drop torrent file">📥</span>
            <p>Drop .torrent file here</p>
          </div>
        </div>
      )}
      {ffmpegAvailable === false && (
          <div className="ffmpeg-warning-banner" role="alert">
            <span className="ffmpeg-warning-icon" aria-hidden="true">⚠️</span>
          <span>ffmpeg not detected — audio track selection, subtitle extraction, and format conversion features are unavailable.</span>
        </div>
      )}
      {browserModeWarning && (
          <div className="browser-mode-warning-banner" role="alert">
            <span className="browser-mode-warning-icon" aria-hidden="true">ℹ️</span>
          <span>Running in browser mode without Electron. Streaming may require downloading the entire file before playback. For the best experience, use the Electron desktop app.</span>
        </div>
      )}
      <div className="room-layout">
        <div className="player-column">
<VideoPlayer
             videoRef={videoRef}
             mediaLabel={selectedMediaLabel}
             mediaKind={selectedMediaKind}
             statusMessage={playbackNotice}
             canControlPlayback={canControlTorrent}
             canControlSeek={canControlTorrent}
             canControlAudioTracks={canControlTorrent}
             canControlSubtitleTracks={canControlTorrent}
             fallbackAudioTracks={selectedMediaAudioTracks}
             selectedAudioTrackIndex={selectedAudioTrackIndex}
             fallbackSubtitles={selectedSubtitles}
             selectedSubtitleIndex={selectedSubtitleIndex}
             onPlaybackStart={onPlaybackStarted}
             onAudioTrackChange={onAudioTrackChange}
             onSubtitleTrackChange={onSubtitleTrackChange}
             onPlayerReady={onPlayerReady}
            onBufferingChange={onBufferingChange}
            onTimeUpdate={onTimeUpdate}
            bufferWindowMB={bufferWindowMB}
            maxBufferMB={maxBufferMB}
            onBufferSettingsChange={onBufferSettingsChange}
            onSeek={onSeek}
            onMuxStreamRequest={onMuxStreamRequest}
          />
          {canControlTorrent ? (
            <TorrentControlsPanel
              magnetLink={magnetLink}
              torrentFileName={torrentFileName}
              sharedSourceLabel={sharedSourceLabel}
              isLoadingTorrent={isLoadingTorrent}
              torrentError={torrentError}
              syncToleranceSeconds={syncToleranceSeconds}
              onMagnetLinkChange={onMagnetLinkChange}
              onTorrentFileChange={onTorrentFileChange}
              onLoadMagnet={onLoadMagnet}
              onLoadTorrentFile={onLoadTorrentFile}
              onSyncToleranceChange={onSyncToleranceChange}
              onResetTorrentInRoom={onResetTorrentInRoom}
              onShowResetConfirm={onShowResetConfirm}
            />
          ) : (
            <GuestViewPanel
              sharedSourceLabel={sharedSourceLabel}
              torrentFileName={torrentFileName}
              isLoadingTorrent={isLoadingTorrent}
              torrentError={torrentError}
              onRequestResend={onRequestResend}
            />
          )}

          <MediaLibraryPanel
            mediaFiles={mediaFiles}
            selectedMediaIndex={selectedMediaIndex}
            selectedMediaLabel={selectedMediaLabel}
            selectedMediaKind={selectedMediaKind}
            isLoadingTorrent={isLoadingTorrent}
            isHost={canControlTorrent}
            onSelectMediaFile={onSelectMediaFile}
          />
        </div>
        <RoomInfo
          peerId={peerId}
          peerRole={peerRole}
          peers={peers}
          isConnected={isConnected}
          onLeaveRoom={onLeaveRoom}
          onRequestLeave={onShowLeaveConfirm}
          onCopyPeerId={copyPeerId}
          copied={copied}
          chatMessages={chatMessages}
          onSendChat={onSendChat}
        />
      </div>
      <StatusBar
        isConnected={isConnected}
        torrentPeerCount={torrentPeerCount}
        downloadSpeed={downloadSpeed}
        bufferingProgress={bufferingProgress}
        torrentPeerHint={torrentPeerHint}
        bufferHint={bufferHint}
        trackerLost={trackerLost ?? false}
        connectionQuality={connectionQuality}
        rttMs={rttMs}
      />
      </section>
    </RoomErrorBoundary>
  );
}

export default RoomPage;
