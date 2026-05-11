import { useEffect, useMemo, useRef, useState } from "react";
import HomePage from "./components/HomePage";
import RoomPage from "./components/RoomPage";
import P2PService from "./services/P2PService";
import SyncService from "./services/SyncService";
import TorrentService, { type TorrentMediaFile } from "./services/TorrentService";
import { type SharedTorrentSource, type SyncMessage } from "./services/types";

import "./App.css";

export type View = "home" | "room";
export type PeerRole = "master" | "slave";
export type PeerConnectionState = "connected" | "connecting" | "disconnected" | "error";

export interface Peer {
  id: string;
  name: string;
  role: PeerRole;
  connectionState: PeerConnectionState;
}

type TorrentLoadRequest = {
  source: SharedTorrentSource;
  selectedMediaIndex: number | null;
  autoplay: boolean;
  broadcast: boolean;
};

const DEFAULT_SYNC_TOLERANCE_SECONDS = 0.5;

function formatSpeed(bytesPerSecond: number): string {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) {
    return "0 B/s";
  }

  const units = ["B/s", "KB/s", "MB/s", "GB/s"];
  let value = bytesPerSecond;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  const precision = value >= 100 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(precision)} ${units[unitIndex]}`;
}

function isPlaybackBlockedError(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }

  const maybeDomException = error as { name?: string; message?: string };
  return (
    maybeDomException.name === "NotAllowedError" ||
    maybeDomException.message?.includes("play() failed because the user didn't interact") === true
  );
}

function clampSyncTolerance(value: number): number {
  if (!Number.isFinite(value) || value < 0) {
    return DEFAULT_SYNC_TOLERANCE_SECONDS;
  }
  return value;
}

function hashBytes(bytes: Uint8Array): string {
  let hash = 0x811c9dc5;
  for (const byte of bytes) {
    hash ^= byte;
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash.toString(16).padStart(8, "0");
}

function createMagnetSource(magnetLink: string): SharedTorrentSource {
  const normalizedMagnetLink = magnetLink.trim();
  return {
    kind: "magnet",
    magnetLink: normalizedMagnetLink,
    sourceKey: `magnet:${normalizedMagnetLink}`,
  };
}

function createTorrentFileSource(fileName: string, bytes: Uint8Array): SharedTorrentSource {
  const normalizedFileName = fileName.trim() || "shared.torrent";
  return {
    kind: "file",
    fileName: normalizedFileName,
    bytes: Array.from(bytes),
    sourceKey: `file:${normalizedFileName}:${bytes.length}:${hashBytes(bytes)}`,
  };
}

function App() {
  const [currentView, setCurrentView] = useState<View>("home");
  const [peerId, setPeerId] = useState("");
  const [peerRole, setPeerRole] = useState<PeerRole | null>(null);
  const [peers, setPeers] = useState<Peer[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [connectionError, setConnectionError] = useState<string | null>(null);
  const [magnetLink, setMagnetLink] = useState("");
  const [torrentFile, setTorrentFile] = useState<File | null>(null);
  const [isLoadingTorrent, setIsLoadingTorrent] = useState(false);
  const [torrentProgress, setTorrentProgress] = useState(0);
  const [downloadSpeed, setDownloadSpeed] = useState("0 B/s");
  const [torrentError, setTorrentError] = useState<string | null>(null);
  const [mediaFiles, setMediaFiles] = useState<TorrentMediaFile[]>([]);
  const [selectedMediaIndex, setSelectedMediaIndex] = useState<number | null>(null);
  const [selectedMediaLabel, setSelectedMediaLabel] = useState<string | null>(null);
  const [selectedMediaKind, setSelectedMediaKind] = useState<TorrentMediaFile["kind"] | null>(null);
  const [selectedMediaFile, setSelectedMediaFile] = useState<TorrentMediaFile | null>(null);
  const [torrentPeerCount, setTorrentPeerCount] = useState(0);
  const [playbackNotice, setPlaybackNotice] = useState<string | null>(null);
  const [syncToleranceSeconds, setSyncToleranceSeconds] = useState(DEFAULT_SYNC_TOLERANCE_SECONDS);

  const videoRef = useRef<HTMLVideoElement | null>(null);
  const p2pServiceRef = useRef<P2PService | null>(null);
  const torrentService = useMemo(() => new TorrentService(), []);
  const syncServiceRef = useRef<SyncService | null>(null);
  const currentTorrentSourceRef = useRef<SharedTorrentSource | null>(null);
  const selectedMediaIndexRef = useRef<number | null>(null);
  const selectedMediaFileRef = useRef<TorrentMediaFile | null>(null);
  const pendingTorrentLoadRef = useRef<TorrentLoadRequest | null>(null);
  const isProcessingTorrentLoadRef = useRef(false);
  const isLoadingTorrentRef = useRef(false);
  const pendingRemoteSyncRef = useRef<SyncMessage | null>(null);

  const disposeSyncService = () => {
    syncServiceRef.current?.dispose();
    syncServiceRef.current = null;
  };

  const resetTorrentState = () => {
    setIsLoadingTorrent(false);
    setTorrentProgress(0);
    setDownloadSpeed("0 B/s");
    setTorrentError(null);
    setTorrentFile(null);
    setMediaFiles([]);
    setSelectedMediaIndex(null);
    setSelectedMediaLabel(null);
    setSelectedMediaKind(null);
    setSelectedMediaFile(null);
    setTorrentPeerCount(0);
    setPlaybackNotice(null);
    selectedMediaIndexRef.current = null;
    selectedMediaFileRef.current = null;
    currentTorrentSourceRef.current = null;
    pendingTorrentLoadRef.current = null;
    pendingRemoteSyncRef.current = null;
    isProcessingTorrentLoadRef.current = false;
    isLoadingTorrentRef.current = false;
  };

  const broadcastCurrentRoomState = (targetPeerId?: string) => {
    if (!p2pServiceRef.current?.isHost()) {
      return;
    }

    const p2pService = p2pServiceRef.current;
    const currentSource = currentTorrentSourceRef.current;
    if (!p2pService) {
      return;
    }

    if (currentSource) {
      p2pService.sendTorrentSource(
        {
          source: currentSource,
          selectedMediaIndex: selectedMediaIndexRef.current,
        },
        targetPeerId,
      );
    }

    p2pService.sendRoomConfig({ syncToleranceSeconds }, targetPeerId);

    const playbackSnapshot = syncServiceRef.current?.createSnapshot();
    if (playbackSnapshot) {
      p2pService.sendSync(playbackSnapshot, targetPeerId);
    }
  };

  const tryApplyPendingRemoteSync = () => {
    if (peerRole !== "slave") {
      return;
    }
    if (isLoadingTorrentRef.current || !selectedMediaFileRef.current || !syncServiceRef.current) {
      return;
    }

    const pendingSync = pendingRemoteSyncRef.current;
    if (!pendingSync) {
      return;
    }

    pendingRemoteSyncRef.current = null;
    syncServiceRef.current.applyRemoteSync(pendingSync);
  };

  const playMediaFile = async (mediaFile: TorrentMediaFile, autoplay = true) => {
    const mediaElement = videoRef.current;
    if (!mediaElement) {
      throw new Error("Media player is not ready");
    }

    await torrentService.streamToMedia(mediaFile.file, mediaElement);
    mediaElement.defaultMuted = false;
    mediaElement.muted = false;
    setSelectedMediaFile(mediaFile);
    selectedMediaFileRef.current = mediaFile;
    setSelectedMediaIndex(mediaFile.index);
    selectedMediaIndexRef.current = mediaFile.index;
    setSelectedMediaLabel(mediaFile.name);
    setSelectedMediaKind(mediaFile.kind);

    if (!autoplay) {
      return;
    }

    try {
      await mediaElement.play();
      setPlaybackNotice(null);
    } catch (error) {
      if (isPlaybackBlockedError(error)) {
        setPlaybackNotice("Autoplay was blocked. Press Play in the player to start the movie.");
      } else {
        throw error;
      }
    }
  };

  const loadTorrentFile = async (file: File) => {
    if (peerRole !== "master" || !videoRef.current) {
      return;
    }

    try {
      const torrentBytes = new Uint8Array(await file.arrayBuffer());
      setMagnetLink("");
      requestTorrentLoad({
        source: createTorrentFileSource(file.name, torrentBytes),
        selectedMediaIndex: null,
        autoplay: true,
        broadcast: true,
      });
    } catch (error) {
      console.error("Torrent file load failed:", error);
    }
  };

  const loadTorrentRequest = async (request: TorrentLoadRequest) => {
    if (!videoRef.current) {
      throw new Error("Media player is not ready");
    }

    const currentSource = currentTorrentSourceRef.current;
    const currentSelectedIndex = selectedMediaIndexRef.current;

    if (currentSource?.sourceKey === request.source.sourceKey) {
      const desiredIndex =
        request.selectedMediaIndex !== null ? request.selectedMediaIndex : currentSelectedIndex;
      const currentMediaFile =
        selectedMediaFileRef.current ??
        (currentSelectedIndex !== null
          ? mediaFiles.find((file) => file.index === currentSelectedIndex) ?? null
          : null);

      if (desiredIndex !== null && desiredIndex !== currentSelectedIndex) {
        const nextMediaFile = mediaFiles.find((file) => file.index === desiredIndex);
        if (!nextMediaFile) {
          throw new Error("Requested media file is not available in the current torrent");
        }

        await playMediaFile(nextMediaFile, request.autoplay);
      } else if (request.autoplay && currentMediaFile && videoRef.current?.paused) {
        await playMediaFile(currentMediaFile, true);
      }

      if (request.broadcast && peerRole === "master") {
        broadcastCurrentRoomState();
      }

      tryApplyPendingRemoteSync();
      return;
    }

    setMediaFiles([]);
    setSelectedMediaIndex(null);
    setSelectedMediaLabel(null);
    setSelectedMediaKind(null);
    setSelectedMediaFile(null);
    selectedMediaIndexRef.current = null;
    selectedMediaFileRef.current = null;

    const torrent =
      request.source.kind === "magnet"
        ? await torrentService.addMagnet(request.source.magnetLink)
        : await torrentService.addTorrentFile(new Uint8Array(request.source.bytes));

    const playableMediaFiles = torrentService.getPlayableMediaFiles(torrent);
    setMediaFiles(playableMediaFiles);

    if (playableMediaFiles.length === 0) {
      throw new Error("No supported video or audio file found in torrent");
    }

    const preferredMediaFile =
      request.selectedMediaIndex !== null
        ? playableMediaFiles.find((file) => file.index === request.selectedMediaIndex) ??
          torrentService.getPreferredMediaFile(torrent)
        : torrentService.getPreferredMediaFile(torrent);

    await playMediaFile(preferredMediaFile, request.autoplay);

    currentTorrentSourceRef.current = request.source;

    if (request.broadcast && peerRole === "master") {
      broadcastCurrentRoomState();
    }

    tryApplyPendingRemoteSync();
  };

  const processTorrentLoadQueue = async () => {
    if (isProcessingTorrentLoadRef.current) {
      return;
    }

    const nextRequest = pendingTorrentLoadRef.current;
    if (!nextRequest) {
      return;
    }

    pendingTorrentLoadRef.current = null;
    isProcessingTorrentLoadRef.current = true;
    isLoadingTorrentRef.current = true;
    setIsLoadingTorrent(true);
    setTorrentError(null);
    setTorrentProgress(0);
    setDownloadSpeed("0 B/s");
    setTorrentPeerCount(0);
    setPlaybackNotice(null);

    try {
      await loadTorrentRequest(nextRequest);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unable to load torrent";
      setTorrentError(message);
      console.error("Torrent load failed:", error);
    } finally {
      isLoadingTorrentRef.current = false;
      isProcessingTorrentLoadRef.current = false;
      setIsLoadingTorrent(false);
      if (pendingTorrentLoadRef.current) {
        void processTorrentLoadQueue();
      }
    }
  };

  const requestTorrentLoad = (request: TorrentLoadRequest) => {
    pendingTorrentLoadRef.current = request;
    if (currentView !== "room" || !videoRef.current) {
      return;
    }

    if (!isProcessingTorrentLoadRef.current) {
      void processTorrentLoadQueue();
    }
  };

  const initializeP2PService = async (role: "host" | "guest") => {
    p2pServiceRef.current?.disconnect();
    disposeSyncService();
    pendingRemoteSyncRef.current = null;

    const p2pService = new P2PService();
    if (role === "host") {
      p2pService.setHost();
    } else {
      p2pService.setGuest();
    }
    p2pServiceRef.current = p2pService;

    p2pService.on("connected", () => {
      setIsConnected(true);
      setIsConnecting(false);
      setPeers((prev) =>
        prev.map((peer) =>
          peer.id !== "self" ? { ...peer, connectionState: "connected" as PeerConnectionState } : peer,
        ),
      );
    });

    p2pService.on("disconnected", () => {
      setIsConnected(false);
      setPeers((prev) =>
        prev.map((peer) => ({ ...peer, connectionState: "disconnected" as PeerConnectionState })),
      );
    });

    p2pService.on("peer_connected", (connectedPeerId) => {
      setPeers((prev) => {
        if (prev.some((peer) => peer.id === connectedPeerId)) {
          return prev;
        }
        return [...prev, { id: connectedPeerId, name: "Peer", role: "slave", connectionState: "connected" }];
      });

      if (role === "host") {
        broadcastCurrentRoomState(connectedPeerId);
      }
    });

    p2pService.on("peer_disconnected", (disconnectedPeerId) => {
      setPeers((prev) => prev.filter((peer) => peer.id !== disconnectedPeerId));
    });

    p2pService.on("sync", (message) => {
      if (role !== "guest") {
        return;
      }
      pendingRemoteSyncRef.current = message;
      tryApplyPendingRemoteSync();
    });

    p2pService.on("torrent_source", (message) => {
      if (role !== "guest") {
        return;
      }

      pendingRemoteSyncRef.current = null;
      requestTorrentLoad({
        source: message.source,
        selectedMediaIndex: message.selectedMediaIndex,
        autoplay: true,
        broadcast: false,
      });
    });

    p2pService.on("room_config", (message) => {
      const nextTolerance = clampSyncTolerance(message.syncToleranceSeconds);
      setSyncToleranceSeconds(nextTolerance);
      syncServiceRef.current?.setSyncToleranceSeconds(nextTolerance);
    });

    p2pService.on("error", (error) => {
      console.error("P2P error:", error);
      setConnectionError(error.message);
      setIsConnecting(false);
    });

    await p2pService.initialize();
    setPeerId(p2pService.getPeerId());
    return p2pService;
  };

  const handleCreateRoom = async () => {
    if (isConnecting) {
      return;
    }

    setIsConnecting(true);
    setConnectionError(null);

    try {
      await initializeP2PService("host");
      setPeerRole("master");
      setIsConnected(true);
      setPeers([{ id: "self", name: "You", role: "master", connectionState: "connected" }]);
      setCurrentView("room");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unable to initialize room";
      p2pServiceRef.current?.disconnect();
      p2pServiceRef.current = null;
      setConnectionError(message);
      setPeerRole(null);
      setPeers([]);
      setIsConnected(false);
    } finally {
      setIsConnecting(false);
    }
  };

  const handleConnectToPeer = async (targetPeerId: string) => {
    if (isConnecting) {
      return;
    }

    const normalizedId = targetPeerId.trim().toUpperCase();
    if (!normalizedId || normalizedId.length !== 6) {
      setConnectionError("Invalid peer ID");
      return;
    }

    setIsConnecting(true);
    setConnectionError(null);
    setPeerRole("slave");
    setPeers([
      { id: "self", name: "You", role: "slave", connectionState: "connected" },
      { id: normalizedId, name: "Host", role: "master", connectionState: "connecting" },
    ]);

    try {
      const p2pService = await initializeP2PService("guest");
      await p2pService.connect(`torrsync-${normalizedId}`);
      setIsConnected(true);
      setPeers([
        { id: "self", name: "You", role: "slave", connectionState: "connected" },
        { id: normalizedId, name: "Host", role: "master", connectionState: "connected" },
      ]);
      setCurrentView("room");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Connection failed";
      p2pServiceRef.current?.disconnect();
      p2pServiceRef.current = null;
      setConnectionError(message);
      setPeerRole(null);
      setPeers([]);
      setIsConnected(false);
      setIsConnecting(false);
    }
  };

  const handleJoinRoom = (code: string) => {
    handleConnectToPeer(code);
  };

  const handleLeaveRoom = () => {
    if (p2pServiceRef.current) {
      p2pServiceRef.current.disconnect();
      p2pServiceRef.current = null;
    }
    disposeSyncService();
    torrentService.clearActiveTorrent();
    if (videoRef.current) {
      videoRef.current.pause();
      videoRef.current.removeAttribute("src");
      videoRef.current.load();
    }
    setCurrentView("home");
    setPeerId("");
    setPeerRole(null);
    setPeers([]);
    setIsConnected(false);
    setIsConnecting(false);
    setConnectionError(null);
    setPlaybackNotice(null);
    pendingRemoteSyncRef.current = null;
    resetTorrentState();
  };

  const handleLoadMagnet = async () => {
    if (peerRole !== "master" || !magnetLink.trim() || !videoRef.current) {
      return;
    }

    setTorrentFile(null);
    requestTorrentLoad({
      source: createMagnetSource(magnetLink),
      selectedMediaIndex: null,
      autoplay: true,
      broadcast: true,
    });
  };

  const handleTorrentFileChange = (file: File | null) => {
    setTorrentFile(file);
    if (!file) {
      return;
    }

    void loadTorrentFile(file);
  };

  const handleLoadTorrentFile = async () => {
    if (!torrentFile) {
      return;
    }

    await loadTorrentFile(torrentFile);
  };

  const handleSelectMediaFile = (mediaFile: TorrentMediaFile) => {
    if (peerRole !== "master" || !currentTorrentSourceRef.current) {
      return;
    }

    if (!videoRef.current) {
      return;
    }

    requestTorrentLoad({
      source: currentTorrentSourceRef.current,
      selectedMediaIndex: mediaFile.index,
      autoplay: true,
      broadcast: true,
    });
  };

  const handleSyncToleranceChange = (value: number) => {
    const nextTolerance = clampSyncTolerance(value);
    setSyncToleranceSeconds(nextTolerance);
    syncServiceRef.current?.setSyncToleranceSeconds(nextTolerance);

    if (p2pServiceRef.current?.isHost() && p2pServiceRef.current.isConnected()) {
      p2pServiceRef.current.sendRoomConfig({ syncToleranceSeconds: nextTolerance });
    }
  };

  const selectedMediaBufferProgress = Math.round(
    (selectedMediaFile?.file.progress ?? (selectedMediaIndex !== null ? 0 : torrentProgress / 100)) * 100,
  );

  const torrentPeerHint =
    torrentService.isElectronBackendEnabled()
      ? torrentPeerCount > 0
        ? `${torrentPeerCount} public peer${torrentPeerCount === 1 ? "" : "s"} discovered via tracker, DHT, and PEX`
        : "Looking for public peers via tracker, DHT, and PEX"
      : torrentPeerCount > 0
        ? `${torrentPeerCount} public WebRTC peer${torrentPeerCount === 1 ? "" : "s"} discovered via trackers`
        : "Looking for public WebRTC peers";

  const bufferHint = selectedMediaFile
    ? selectedMediaBufferProgress >= 100
      ? "Selected file is fully buffered."
      : "Selected file is buffering from the swarm."
    : "Load a torrent and pick a movie to see file buffering progress.";

  const sharedTorrentLabel = currentTorrentSourceRef.current
    ? currentTorrentSourceRef.current.kind === "magnet"
      ? "Magnet link"
      : currentTorrentSourceRef.current.fileName
    : torrentFile?.name ?? null;

  useEffect(() => {
    const offTorrentError = torrentService.on("error", (error) => {
      console.error("Torrent error", error);
      setTorrentError(error.message);
      setIsLoadingTorrent(false);
      isLoadingTorrentRef.current = false;
    });

    const offTorrentProgress = torrentService.on("progress", (progress) => {
      setTorrentProgress(Math.round(progress * 100));
    });

    const offTorrentSpeed = torrentService.on("speed", (speed) => {
      setDownloadSpeed(formatSpeed(speed));
    });

    const offTorrentPeerCount = torrentService.on("peerCount", (peerCount) => {
      setTorrentPeerCount(peerCount);
    });

    return () => {
      offTorrentError();
      offTorrentProgress();
      offTorrentSpeed();
      offTorrentPeerCount();
      torrentService.destroy();
    };
  }, [torrentService]);

  useEffect(() => {
    disposeSyncService();

    if (currentView !== "room" || !videoRef.current || !peerRole || !p2pServiceRef.current) {
      return;
    }

    const p2pSyncTransport = {
      sendSync: (message: SyncMessage) => {
        if (p2pServiceRef.current?.isConnected()) {
          p2pServiceRef.current.sendSync(message);
        }
      },
    };

    const syncService = new SyncService(
      p2pSyncTransport,
      videoRef.current,
      peerRole,
      syncToleranceSeconds,
    );
    syncServiceRef.current = syncService;
    tryApplyPendingRemoteSync();
    return () => {
      syncService.dispose();
      if (syncServiceRef.current === syncService) {
        syncServiceRef.current = null;
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentView, peerRole]);

  useEffect(() => {
    if (currentView !== "room" || !videoRef.current || isProcessingTorrentLoadRef.current) {
      return;
    }

    if (!pendingTorrentLoadRef.current) {
      return;
    }

    void processTorrentLoadQueue();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentView]);

  useEffect(() => {
    syncServiceRef.current?.setSyncToleranceSeconds(syncToleranceSeconds);
  }, [syncToleranceSeconds]);

  return (
    <main className="app-shell">
      {currentView === "home" ? (
        <HomePage
          peerId={peerId}
          onCreateRoom={handleCreateRoom}
          onJoinRoom={handleJoinRoom}
          isConnecting={isConnecting}
          connectionError={connectionError}
        />
      ) : (
        <RoomPage
          peerId={peerId}
          peerRole={peerRole}
          peers={peers}
          isConnected={isConnected}
          canControlTorrent={peerRole === "master"}
          magnetLink={magnetLink}
          torrentFileName={torrentFile?.name ?? null}
          sharedSourceLabel={sharedTorrentLabel}
          mediaFiles={mediaFiles}
          selectedMediaIndex={selectedMediaIndex}
          selectedMediaLabel={selectedMediaLabel}
          selectedMediaKind={selectedMediaKind}
          torrentPeerCount={torrentPeerCount}
          syncToleranceSeconds={syncToleranceSeconds}
          onSyncToleranceChange={handleSyncToleranceChange}
          onMagnetLinkChange={setMagnetLink}
          onTorrentFileChange={handleTorrentFileChange}
          videoRef={videoRef}
          playbackNotice={playbackNotice}
          onPlaybackStarted={() => setPlaybackNotice(null)}
          onLoadMagnet={() => void handleLoadMagnet()}
          onLoadTorrentFile={() => void handleLoadTorrentFile()}
          onSelectMediaFile={handleSelectMediaFile}
          onLeaveRoom={handleLeaveRoom}
          isLoadingTorrent={isLoadingTorrent}
          downloadSpeed={downloadSpeed}
          bufferingProgress={selectedMediaBufferProgress}
          torrentError={torrentError}
          torrentPeerHint={torrentPeerHint}
          bufferHint={bufferHint}
        />
      )}
    </main>
  );
}

export default App;
