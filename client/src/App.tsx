import { useCallback, useEffect, useRef, useState } from "react";
import HomePage from "./components/HomePage";
import RoomPage from "./components/RoomPage";
import { uiLogger } from "./utils/logger";
import ConfirmModal from "./components/ConfirmModal";
import ErrorBoundary from "./components/ErrorBoundary";
import { useP2PConnection } from "./hooks/useP2PConnection";
import { useTorrentLoader } from "./hooks/useTorrentLoader";
import { useSyncPlayback } from "./hooks/useSyncPlayback";
import { useRoomStateContext } from "./hooks/useRoomStateContext";
import "./App.css";

export type View = "home" | "room";

function App() {
  const [currentView, setCurrentView] = useState<View>("home");
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [ffmpegAvailable, setFfmpegAvailable] = useState<boolean | null>(null);
  const [showLeaveConfirm, setShowLeaveConfirm] = useState(false);
  const [showResetConfirm, setShowResetConfirm] = useState(false);
  const [showCloseConfirm, setShowCloseConfirm] = useState(false);
  const [magnetLink, setMagnetLink] = useState("");
  const torrentFileRef = useRef<File | null>(null);
  const [bufferWindowMB, setBufferWindowMB] = useState(50);
  const [maxBufferMB, setMaxBufferMB] = useState(500);
  const [browserModeWarning, setBrowserModeWarning] = useState(false);
  const [isPlayerReady, setIsPlayerReady] = useState(false);

  const roomState = useRoomStateContext();

  const p2p = useP2PConnection();
  const broadcastTimeoutRef = useRef<number | null>(null);
  const clearBroadcastTimeout = () => {
    if (broadcastTimeoutRef.current !== null) {
      clearTimeout(broadcastTimeoutRef.current);
      broadcastTimeoutRef.current = null;
    }
  };
  const debouncedBroadcast = useCallback((targetPeerId?: string) => {
    clearBroadcastTimeout();
    broadcastTimeoutRef.current = window.setTimeout(() => {
      p2p.scheduleBroadcast(targetPeerId);
      broadcastTimeoutRef.current = null;
    }, 500);
  }, [p2p]);
  const torrent = useTorrentLoader(videoRef, currentView, debouncedBroadcast);
  const sync = useSyncPlayback(videoRef, { current: p2p.p2pService }, currentView, debouncedBroadcast);

  useEffect(() => {
    const offSync = p2p.onSync((msg) => {
      if (roomState.state.peerRole === "slave") {
        roomState.setPendingSync(msg);
        if (isPlayerReady && sync.syncServiceRef.current && roomState.state.selectedMediaFile) {
          roomState.setPendingSync(null);
          sync.syncServiceRef.current.applyRemoteSync(msg, p2p.p2pService?.getLastRttMs());
        }
      }
    });
    const offTorrent = p2p.onTorrentSource((msg) => {
      if (roomState.state.peerRole === "slave") {
        torrent.loadTorrent({
          source: msg.source,
          selectedMediaIndex: msg.selectedMediaIndex,
          selectedAudioTrackIndex: msg.selectedAudioTrackIndex,
          selectedSubtitleIndex: msg.selectedSubtitleIndex,
          autoplay: true, broadcast: false,
        });
      }
    });
    const offConfig = p2p.onRoomConfig((msg) => {
      sync.setSyncTolerance(msg.syncToleranceSeconds);
    });
    return () => { offSync(); offTorrent(); offConfig(); };
  }, [p2p, roomState, isPlayerReady, torrent, sync]);

  useEffect(() => {
    const torrentSvc = torrent.getTorrentService();
    const offErr = torrentSvc.on("error", (e: Error) => {
      uiLogger.error("Torrent error", e);
    });
    const offProg = torrentSvc.on("progress", (_p: number) => {});
    const offSpeed = torrentSvc.on("speed", (s: number) => { void s; });
    const offPeers = torrentSvc.on("peerCount", (c: number) => { void c; });
    return () => { offErr(); offProg(); offSpeed(); offPeers(); };
  }, [torrent, roomState]);

  useEffect(() => {
    const win = window as Window & { torrsyncElectronWindow?: { onCloseRequest: (cb: () => void) => void; closeConfirmed: () => void; closeCancelled: () => void } };
    if (win.torrsyncElectronWindow) {
      win.torrsyncElectronWindow.onCloseRequest(() => setShowCloseConfirm(true));
    }
    return () => {};
  }, []);

  useEffect(() => {
    const win = window as Window & { torrsyncElectronTorrent?: { isFfmpegAvailable?: () => Promise<boolean> } };
    if (win.torrsyncElectronTorrent?.isFfmpegAvailable) {
      void win.torrsyncElectronTorrent.isFfmpegAvailable().then(setFfmpegAvailable)
        .catch(() => setFfmpegAvailable(false));
    }
  }, []);

  const handleCloseConfirmed = useCallback(() => {
    setShowCloseConfirm(false);
    const win = window as Window & { torrsyncElectronWindow?: { closeConfirmed: () => void } };
    win.torrsyncElectronWindow?.closeConfirmed();
  }, []);

  const handleCloseCancelled = useCallback(() => {
    setShowCloseConfirm(false);
    const win = window as Window & { torrsyncElectronWindow?: { closeCancelled: () => void } };
    win.torrsyncElectronWindow?.closeCancelled();
  }, []);

  const handleCreateRoom = async () => {
    setBrowserModeWarning(!torrent.getTorrentService().isElectronBackendEnabled());
    await p2p.createRoom();
    setCurrentView("room");
  };

  const handleJoinRoom = async (code: string) => {
    await p2p.joinRoom(code);
    setCurrentView("room");
  };

  const handleLeaveRoom = async () => {
    clearBroadcastTimeout();
    await p2p.disconnect();
    sync.syncServiceRef.current?.dispose();
    try { await torrent.getTorrentService().destroy(); } catch (e) { uiLogger.warn("Torrent cleanup:", e); }
    videoRef.current?.pause();
    if (videoRef.current) { videoRef.current.removeAttribute("src"); videoRef.current.load(); }
    roomState.reset();
    setCurrentView("home");
    setMagnetLink("");
    torrentFileRef.current = null;
    setShowLeaveConfirm(false); setIsPlayerReady(false);
  };

  const handleResetTorrent = async () => {
    await torrent.resetTorrent();
    setShowResetConfirm(false);
  };

  const handleLoadMagnet = async () => {
    if (roomState.state.peerRole !== "master" || !magnetLink.trim()) return;
    try {
      const { createMagnetSource } = await import("./utils/torrent");
      torrent.loadTorrent({
        source: createMagnetSource(magnetLink),
        selectedMediaIndex: null, selectedAudioTrackIndex: null, selectedSubtitleIndex: null,
        autoplay: true, broadcast: true,
      });
    } catch (e) {
      uiLogger.error("Magnet:", e);
    }
  };

  const currentMediaFile = roomState.state.selectedMediaFile;
  const bufferProgress = Math.min(100, Math.round(
    ((currentMediaFile?.file.progress && currentMediaFile.file.progress > 0)
      ? currentMediaFile.file.progress : torrent.torrentProgress / 100) * 100,
  ));

  const torrentPeerHint = torrent.torrentPeerCount > 0
    ? `${torrent.torrentPeerCount} public peer${torrent.torrentPeerCount === 1 ? "" : "s"} discovered via tracker, DHT, and PEX`
    : "Looking for public peers via tracker, DHT, and PEX";

  const bufferHint = currentMediaFile
    ? bufferProgress >= 100 ? "Selected file is fully buffered."
      : bufferProgress > 0 ? "Buffering — loading data around current position..."
      : "Selected file is buffering from the swarm."
    : "Load a torrent and pick a movie to see file buffering progress.";

  const sharedLabel = roomState.state.currentTorrentSource
    ? roomState.state.currentTorrentSource.kind === "magnet" ? "Magnet link" : roomState.state.currentTorrentSource.fileName
    : null;

  return (
    <ErrorBoundary
      onReset={() => {
        roomState.reset();
        setCurrentView("home");
      }}
    >
      <main className="app-shell">
        {currentView === "home" ? (
          <HomePage
            peerId={p2p.peerId}
            onCreateRoom={handleCreateRoom}
            onJoinRoom={handleJoinRoom}
            isConnecting={p2p.isConnecting}
            connectionError={p2p.connectionError}
          />
        ) : (
          <RoomPage
            connection={{
              peerId: p2p.peerId, peerRole: p2p.peerRole, peers: p2p.peers,
              isConnected: p2p.isConnected, connectionQuality: p2p.connectionQuality, rttMs: p2p.rttMs,
            }}
            torrent={{
              magnetLink, torrentFileName: roomState.state.currentTorrentSource?.kind === "file" ? roomState.state.currentTorrentSource.fileName : null, sharedSourceLabel: sharedLabel,
              mediaFiles: roomState.state.mediaFiles ?? [],
              selectedMediaIndex: roomState.state.selectedMediaIndex,
              selectedMediaLabel: roomState.state.selectedMediaLabel,
              selectedMediaKind: roomState.state.selectedMediaKind,
              selectedMediaAudioTracks: roomState.state.selectedMediaAudioTracks,
              selectedAudioTrackIndex: roomState.state.selectedAudioTrackIndex,
              selectedSubtitles: roomState.state.selectedSubtitles,
              selectedSubtitleIndex: roomState.state.selectedSubtitleIndex,
              isLoadingTorrent: torrent.isLoadingTorrent, downloadSpeed: torrent.downloadSpeed,
              bufferingProgress: bufferProgress, torrentPeerCount: torrent.torrentPeerCount,
              torrentError: torrent.torrentError, torrentPeerHint, bufferHint,
              trackerLost: torrent.trackerLost, bufferWindowMB, maxBufferMB,
            }}
            player={{
              videoRef, playbackNotice: torrent.playbackNotice,
              syncToleranceSeconds: roomState.state.syncToleranceSeconds,
              canControl: roomState.state.peerRole === "master",
            }}
            chat={{ chatMessages: p2p.chatMessages, onSendChat: p2p.sendChat }}
            ffmpegAvailable={ffmpegAvailable}
            browserModeWarning={browserModeWarning}
            onSubtitleTrackChange={(idx) => { roomState.setSubtitleIndex(idx); debouncedBroadcast(); }}
            onSyncToleranceChange={sync.setSyncTolerance}
            onMagnetLinkChange={setMagnetLink}
            onTorrentFileChange={(f) => { torrentFileRef.current = f; if (f) torrent.loadTorrentFile(f); }}
            onPlaybackStarted={() => {}}
            onAudioTrackChange={(idx) => { roomState.setAudioTrackIndex(idx); debouncedBroadcast(); }}
            onPlayerReady={(ready) => {
              setIsPlayerReady(ready);
              if (ready) sync.tryApplyPendingRemoteSync();
            }}
            onLoadMagnet={() => void handleLoadMagnet()}
            onLoadTorrentFile={() => { if (torrentFileRef.current) torrent.loadTorrentFile(torrentFileRef.current); }}
            onSelectMediaFile={(mf) => {
              if (roomState.state.peerRole !== "master" || !roomState.state.currentTorrentSource) return;
              torrent.loadTorrent({
                source: roomState.state.currentTorrentSource,
                selectedMediaIndex: mf.index, selectedAudioTrackIndex: null, selectedSubtitleIndex: null,
                autoplay: true, broadcast: true,
              });
              roomState.setAudioTrackIndex(null);
            }}
            onLeaveRoom={handleLeaveRoom}
            onResetTorrentInRoom={handleResetTorrent}
            onTimeUpdate={(time, dur) => sync.handleTimeUpdate(time, dur ?? 0, torrent.getTorrentService())}
            onBufferingChange={() => {}}
            onBufferSettingsChange={(bw, mx) => {
              torrent.getTorrentService().setBufferSettings(bw, mx);
              setBufferWindowMB(bw); setMaxBufferMB(mx);
            }}
            onSeek={sync.seek}
            onShowLeaveConfirm={() => setShowLeaveConfirm(true)}
            onShowResetConfirm={() => setShowResetConfirm(true)}
            onReturnHome={() => setCurrentView("home")}
            reconnectFailed={p2p.reconnectFailed}
            onRequestResend={() => { if (p2p.p2pService?.isConnected()) p2p.p2pService.sendChat("/resend"); }}
          />
        )}
        <ConfirmModal isOpen={showLeaveConfirm} title="Leave room?" message="Are you sure you want to leave the room? All connections will be closed." confirmLabel="Leave" danger onConfirm={handleLeaveRoom} onCancel={() => setShowLeaveConfirm(false)} />
        <ConfirmModal isOpen={showResetConfirm} title="Change torrent source?" message="Current playback will stop and the room will be reset." confirmLabel="Change" danger onConfirm={handleResetTorrent} onCancel={() => setShowResetConfirm(false)} />
        <ConfirmModal isOpen={showCloseConfirm} title="Close application?" message="Are you sure you want to close TorrSyncPlayer? All connections will be lost." confirmLabel="Close" danger onConfirm={handleCloseConfirmed} onCancel={handleCloseCancelled} />
      </main>
    </ErrorBoundary>
  );
}

export default App;
