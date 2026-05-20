import { useCallback, useEffect, useRef, useState } from "react";
import HomePage from "./components/HomePage";
import RoomPage from "./components/RoomPage";
import P2PService from "./services/P2PService";
import SyncService from "./services/SyncService";
import TorrentService, { type TorrentMediaFile } from "./services/TorrentService";
import { type AudioTrackInfo, type SharedTorrentSource, type SubtitleTrackInfo, type SyncMessage } from "./services/types";
import { formatSpeed } from "./utils/format";
import { p2pLogger } from "./utils/logger";

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
  selectedAudioTrackIndex: number | null;
  selectedSubtitleIndex: number | null;
  autoplay: boolean;
  broadcast: boolean;
};

const DEFAULT_SYNC_TOLERANCE_SECONDS = 0.5;

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
  const [selectedMediaAudioTracks, setSelectedMediaAudioTracks] = useState<AudioTrackInfo[]>([]);
  const [selectedAudioTrackIndex, setSelectedAudioTrackIndex] = useState<number | null>(null);
  const [selectedSubtitleIndex, setSelectedSubtitleIndex] = useState<number | null>(null);
  const [selectedSubtitles, setSelectedSubtitles] = useState<SubtitleTrackInfo[]>([]);
  const [torrentPeerCount, setTorrentPeerCount] = useState(0);
  const [trackerLost, setTrackerLost] = useState(false);
  const [playbackNotice, setPlaybackNotice] = useState<string | null>(null);
  const [syncToleranceSeconds, setSyncToleranceSeconds] = useState(DEFAULT_SYNC_TOLERANCE_SECONDS);
  const [isPlayerReady, setIsPlayerReady] = useState(false);
  const [roomPassword, setRoomPassword] = useState("");
  const [bufferWindowMB, setBufferWindowMB] = useState(50);
  const [maxBufferMB, setMaxBufferMB] = useState(500);

  const videoRef = useRef<HTMLVideoElement | null>(null);
  const p2pServiceRef = useRef<P2PService | null>(null);
  const torrentServiceRef = useRef<TorrentService | null>(null);
  const [torrentServiceVersion, setTorrentServiceVersion] = useState(0);
  const getTorrentService = useCallback(() => {
    if (!torrentServiceRef.current) {
      torrentServiceRef.current = new TorrentService();
    }
    return torrentServiceRef.current;
  }, []);
  const syncServiceRef = useRef<SyncService | null>(null);
  const currentTorrentSourceRef = useRef<SharedTorrentSource | null>(null);
  const selectedMediaIndexRef = useRef<number | null>(null);
  const selectedMediaFileRef = useRef<TorrentMediaFile | null>(null);
  const selectedAudioTrackIndexRef = useRef<number | null>(null);
  const selectedSubtitleIndexRef = useRef<number | null>(null);
  const pendingTorrentLoadRef = useRef<TorrentLoadRequest | null>(null);
  const isProcessingTorrentLoadRef = useRef(false);
  const isLoadingTorrentRef = useRef(false);
  const isPlayerReadyRef = useRef(false);
  const pendingRemoteSyncRef = useRef<SyncMessage | null>(null);
  const peerRoleRef = useRef<PeerRole | null>(null);

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
    setSelectedMediaAudioTracks([]);
    setSelectedAudioTrackIndex(null);
    setSelectedSubtitleIndex(null);
    setSelectedSubtitles([]);
    setTorrentPeerCount(0);
    setPlaybackNotice(null);
    setIsPlayerReady(false);
    isPlayerReadyRef.current = false;
    selectedMediaIndexRef.current = null;
    selectedMediaFileRef.current = null;
    selectedAudioTrackIndexRef.current = null;
    selectedSubtitleIndexRef.current = null;
    currentTorrentSourceRef.current = null;
    pendingTorrentLoadRef.current = null;
    pendingRemoteSyncRef.current = null;
    isProcessingTorrentLoadRef.current = false;
    isLoadingTorrentRef.current = false;
  };

  const setSelectedAudioTrackSelection = (trackIndex: number | null) => {
    setSelectedAudioTrackIndex(trackIndex);
    selectedAudioTrackIndexRef.current = trackIndex;
  };

  const setSelectedSubtitleSelection = (trackIndex: number | null) => {
    setSelectedSubtitleIndex(trackIndex);
    selectedSubtitleIndexRef.current = trackIndex;
  };

  const getCurrentSourceKey = () => currentTorrentSourceRef.current?.sourceKey ?? null;

  const enrichSyncMessage = useCallback((message: SyncMessage): SyncMessage => {
    const sourceKey = getCurrentSourceKey();
    if (!sourceKey || message.sourceKey === sourceKey) {
      return message;
    }

    return {
      ...message,
      sourceKey,
    };
  }, []);

  const broadcastTimeoutRef = useRef<number | null>(null);

  const clearBroadcastTimeout = () => {
    if (broadcastTimeoutRef.current !== null) {
      window.clearTimeout(broadcastTimeoutRef.current);
      broadcastTimeoutRef.current = null;
    }
  };

  const broadcastCurrentRoomState = useCallback((targetPeerId?: string) => {
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
          selectedAudioTrackIndex: selectedAudioTrackIndexRef.current,
          selectedSubtitleIndex: selectedSubtitleIndexRef.current,
        },
        targetPeerId,
      );
    }

    p2pService.sendRoomConfig({ syncToleranceSeconds, roomPassword }, targetPeerId);

    const playbackSnapshot = syncServiceRef.current?.createSnapshot();
    if (playbackSnapshot) {
      p2pService.sendSync(enrichSyncMessage(playbackSnapshot), targetPeerId);
    }
  }, [syncToleranceSeconds, enrichSyncMessage, roomPassword]);

  const scheduleBroadcast = useCallback((targetPeerId?: string) => {
    clearBroadcastTimeout();
    broadcastTimeoutRef.current = window.setTimeout(() => {
      broadcastCurrentRoomState(targetPeerId);
      broadcastTimeoutRef.current = null;
    }, 500);
  }, [broadcastCurrentRoomState]);

  const tryApplyPendingRemoteSync = useCallback(() => {
    if (peerRole !== "slave") {
      return;
    }
    if (
      !isPlayerReadyRef.current ||
      isLoadingTorrentRef.current ||
      !selectedMediaFileRef.current ||
      !syncServiceRef.current
    ) {
      return;
    }

    const pendingSync = pendingRemoteSyncRef.current;
    if (!pendingSync) {
      return;
    }

    if (!pendingSync.sourceKey) {
      return;
    }

    const currentSourceKey = getCurrentSourceKey();
    if (currentSourceKey && pendingSync.sourceKey !== currentSourceKey) {
      return;
    }

    pendingRemoteSyncRef.current = null;
    syncServiceRef.current.applyRemoteSync(pendingSync);
  }, [peerRole]);

  const playMediaFile = useCallback(async (mediaFile: TorrentMediaFile, autoplay = true) => {
    const mediaElement = videoRef.current;
    if (!mediaElement) {
      throw new Error("Media player is not ready");
    }

await getTorrentService().streamToMedia(mediaFile.file, mediaElement);
     selectedMediaFileRef.current = mediaFile;
     setSelectedMediaAudioTracks([]);
     setSelectedSubtitles([]);
     void getTorrentService()
       .probeAudioTracks(mediaFile.file)
       .then((audioTracks) => {
         if (selectedMediaFileRef.current === mediaFile) {
           setSelectedMediaAudioTracks(audioTracks);
         }
       })
       .catch(() => undefined);
     void getTorrentService()
       .probeSubtitles(mediaFile.file)
       .then((subtitles) => {
         if (selectedMediaFileRef.current === mediaFile) {
           setSelectedSubtitles(subtitles);
         }
       })
       .catch(() => undefined);
    mediaElement.defaultMuted = false;
    mediaElement.muted = false;
    if (mediaElement.volume <= 0) {
      mediaElement.volume = 1;
    }
    setSelectedMediaFile(mediaFile);
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
  }, [getTorrentService]);

  const loadTorrentFile = async (file: File) => {
    if (peerRole !== "master") {
      return;
    }

        try {
      const torrentBytes = new Uint8Array(await file.arrayBuffer());
      setMagnetLink("");
      requestTorrentLoad({
        source: createTorrentFileSource(file.name, torrentBytes),
        selectedMediaIndex: null,
        selectedAudioTrackIndex: null,
        selectedSubtitleIndex: null,
        autoplay: true,
        broadcast: true,
      });
    } catch (error) {
      console.error("Torrent file load failed:", error);
    }
  };

  const loadTorrentRequest = useCallback(async (request: TorrentLoadRequest) => {
    if (!videoRef.current) {
      throw new Error("Media player is not ready");
    }

    const currentSource = currentTorrentSourceRef.current;
    const currentSelectedIndex = selectedMediaIndexRef.current;
    const currentSelectedAudioTrackIndex = selectedAudioTrackIndexRef.current;
    const currentSelectedSubtitleIndex = selectedSubtitleIndexRef.current;

    if (currentSource?.sourceKey === request.source.sourceKey) {
      const desiredIndex =
        request.selectedMediaIndex !== null ? request.selectedMediaIndex : currentSelectedIndex;
      const desiredAudioTrackIndex =
        request.selectedAudioTrackIndex ?? currentSelectedAudioTrackIndex;
      const desiredSubtitleIndex =
        request.selectedSubtitleIndex ?? currentSelectedSubtitleIndex;
      const currentMediaFile =
        selectedMediaFileRef.current ??
        (currentSelectedIndex !== null
          ? mediaFiles.find((file) => file.index === currentSelectedIndex) ?? null
          : null);

      if (desiredAudioTrackIndex !== currentSelectedAudioTrackIndex) {
        setSelectedAudioTrackSelection(desiredAudioTrackIndex);
      }

      if (desiredSubtitleIndex !== currentSelectedSubtitleIndex) {
        setSelectedSubtitleSelection(desiredSubtitleIndex);
      }

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
        scheduleBroadcast();
      }

      tryApplyPendingRemoteSync();
      return;
    }

    setMediaFiles([]);
    setSelectedMediaIndex(null);
    setSelectedMediaLabel(null);
    setSelectedMediaKind(null);
    setSelectedMediaFile(null);
    setSelectedMediaAudioTracks([]);
    setSelectedAudioTrackSelection(request.selectedAudioTrackIndex);
    selectedMediaIndexRef.current = null;
    selectedMediaFileRef.current = null;

    const torrent =
      request.source.kind === "magnet"
        ? await getTorrentService().addMagnet(request.source.magnetLink)
        : await getTorrentService().addTorrentFile(new Uint8Array(request.source.bytes));

    const playableMediaFiles = getTorrentService().getPlayableMediaFiles(torrent);
    setMediaFiles(playableMediaFiles);

    if (playableMediaFiles.length === 0) {
      throw new Error("No supported video or audio file found in torrent");
    }

    const preferredMediaFile =
      request.selectedMediaIndex !== null
        ? playableMediaFiles.find((file) => file.index === request.selectedMediaIndex) ??
          getTorrentService().getPreferredMediaFile(torrent)
        : getTorrentService().getPreferredMediaFile(torrent);

    await playMediaFile(preferredMediaFile, request.autoplay);

    currentTorrentSourceRef.current = request.source;

    if (request.broadcast && peerRole === "master") {
      scheduleBroadcast();
    }

    tryApplyPendingRemoteSync();
  }, [mediaFiles, peerRole, getTorrentService, playMediaFile, scheduleBroadcast, tryApplyPendingRemoteSync]);

  const processTorrentLoadQueueRef = useRef<() => Promise<void>>(async () => {});

  const processTorrentLoadQueue = useCallback(async () => {
    if (isProcessingTorrentLoadRef.current) {
      return;
    }

    const nextRequest = pendingTorrentLoadRef.current;
    if (!nextRequest) {
      return;
    }

    if (!isPlayerReadyRef.current || !videoRef.current) {
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
        void processTorrentLoadQueueRef.current();
      }
    }
  }, [loadTorrentRequest]);

  processTorrentLoadQueueRef.current = processTorrentLoadQueue;

  const requestTorrentLoad = (request: TorrentLoadRequest) => {
    pendingTorrentLoadRef.current = request;
    if (currentView !== "room" || !isPlayerReadyRef.current || !videoRef.current) {
      return;
    }

    if (!isProcessingTorrentLoadRef.current) {
      void processTorrentLoadQueueRef.current();
    }
  };

  // Called when isPlayerReady changes — process any pending torrent load.
  useEffect(() => {
    if (isPlayerReady && currentView === "room" && !isProcessingTorrentLoadRef.current) {
      if (pendingTorrentLoadRef.current) {
        void processTorrentLoadQueueRef.current();
      }
    }
  }, [isPlayerReady, currentView]);

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
      // Clear stale pending sync — it may be from a previous connection.
      pendingRemoteSyncRef.current = null;
    });

    p2pService.on("peer_connected", (connectedPeerId) => {
      setPeers((prev) => {
        if (prev.some((peer) => peer.id === connectedPeerId)) {
          return prev;
        }
        return [...prev, { id: connectedPeerId, name: "Peer", role: "slave", connectionState: "connected" }];
      });

      if (role === "host") {
        // Delay broadcast to let the data channel fully initialize.
        // The channel may be "open" but not yet ready for large messages.
        scheduleBroadcast(connectedPeerId);
      }
    });

    p2pService.on("peer_disconnected", (disconnectedPeerId) => {
      setPeers((prev) => prev.filter((peer) => peer.id !== disconnectedPeerId));
    });

    p2pService.on("sync", (message) => {
      if (peerRoleRef.current !== "slave") {
        return;
      }
      pendingRemoteSyncRef.current = message;
      if (isPlayerReadyRef.current && syncServiceRef.current && selectedMediaFileRef.current) {
        pendingRemoteSyncRef.current = null;
        syncServiceRef.current.applyRemoteSync(message);
      }
    });

    p2pService.on("torrent_source", (message) => {
      if (peerRoleRef.current !== "slave") {
        return;
      }

      if (
        pendingRemoteSyncRef.current &&
        (!pendingRemoteSyncRef.current.sourceKey ||
          pendingRemoteSyncRef.current.sourceKey !== message.source.sourceKey)
      ) {
        pendingRemoteSyncRef.current = null;
      }
      requestTorrentLoad({
        source: message.source,
        selectedMediaIndex: message.selectedMediaIndex,
        selectedAudioTrackIndex: message.selectedAudioTrackIndex,
        selectedSubtitleIndex: message.selectedSubtitleIndex,
        autoplay: true,
        broadcast: false,
      });
    });

    p2pService.on("room_config", (message) => {
      const nextTolerance = clampSyncTolerance(message.syncToleranceSeconds);
      setSyncToleranceSeconds(nextTolerance);
      syncServiceRef.current?.setSyncToleranceSeconds(nextTolerance);
    });

    p2pService.on("reconnecting", (attempt, delayMs) => {
      p2pLogger.info(`Reconnecting attempt ${attempt}, delay ${delayMs}ms`);
      setConnectionError(`Connection lost. Reconnecting in ${Math.round(delayMs / 1000)}s... (attempt ${attempt})`);
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
      peerRoleRef.current = "master";
      setIsConnected(true);
      setPeers([{ id: "self", name: "You", role: "master", connectionState: "connected" }]);
      setCurrentView("room");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unable to initialize room";
      try {
        p2pServiceRef.current?.disconnect();
      } catch {
        // Disconnect errors during cleanup are non-fatal
      }
      p2pServiceRef.current = null;
      setConnectionError(message);
      setPeerRole(null);
      peerRoleRef.current = null;
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

      try {
        const p2pService = await initializeP2PService("guest");
        await p2pService.connect(`torrsync-${normalizedId}`);
        setPeerRole("slave");
        peerRoleRef.current = "slave";
        setIsConnected(true);
        setPeers([
          { id: "self", name: "You", role: "slave", connectionState: "connected" },
          { id: normalizedId, name: "Host", role: "master", connectionState: "connected" },
        ]);
        setCurrentView("room");
      } catch (error) {
        const message = error instanceof Error ? error.message : "Connection failed";
        try {
          p2pServiceRef.current?.disconnect();
        } catch {
          // Disconnect errors during cleanup are non-fatal
        }
        p2pServiceRef.current = null;
        setConnectionError(message);
        setPeerRole(null);
      peerRoleRef.current = null;
        setPeers([]);
        setIsConnected(false);
      } finally {
        setIsConnecting(false);
      }
    };

  const handleJoinRoom = (code: string) => {
    handleConnectToPeer(code);
  };

  const handleLeaveRoom = async () => {
    const shouldLeave = window.confirm("Are you sure you want to leave the room?");
    if (!shouldLeave) {
      return;
    }
    clearBroadcastTimeout();
    if (p2pServiceRef.current) {
      p2pServiceRef.current.disconnect();
      p2pServiceRef.current = null;
    }
    disposeSyncService();
    // Destroy the current service instance before nulling the ref
    const torrentService = torrentServiceRef.current ?? getTorrentService();
    await torrentService.destroy().catch(() => undefined);
    torrentServiceRef.current = null;
    videoRef.current?.pause();
    videoRef.current?.removeAttribute("src");
    videoRef.current?.load();
    setCurrentView("home");
    setPeerId("");
    setPeerRole(null);
      peerRoleRef.current = null;
    setPeers([]);
    setIsConnected(false);
    setIsConnecting(false);
    setIsPlayerReady(false);
    isPlayerReadyRef.current = false;
    setConnectionError(null);
    setPlaybackNotice(null);
    pendingRemoteSyncRef.current = null;
    setTrackerLost(false);
    resetTorrentState();
  };

  const handleResetTorrentInRoom = async () => {
    if (peerRole !== "master") {
      return;
    }
    // Track the old service for cleanup, then null the ref.
    const oldService = torrentServiceRef.current;
    torrentServiceRef.current = null;
    selectedMediaFileRef.current = null;
    currentTorrentSourceRef.current = null;
    resetTorrentState();
    setMagnetLink("");
    setTorrentFile(null);
    // Destroy the old service after nulling the ref so the effect doesn't double-destroy.
    if (oldService) {
      await oldService.destroy().catch(() => undefined);
    }
    // Force re-subscription on the new torrentService instance.
    setTorrentServiceVersion((v) => v + 1);
  };

  const handleLoadMagnet = async () => {
    if (peerRole !== "master" || !magnetLink.trim()) {
      return;
    }

    setTorrentFile(null);
    requestTorrentLoad({
      source: createMagnetSource(magnetLink),
      selectedMediaIndex: null,
      selectedAudioTrackIndex: null,
      selectedSubtitleIndex: null,
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

    requestTorrentLoad({
      source: currentTorrentSourceRef.current,
      selectedMediaIndex: mediaFile.index,
      selectedAudioTrackIndex: null,
      selectedSubtitleIndex: null,
      autoplay: true,
      broadcast: true,
    });
    setSelectedAudioTrackSelection(null);
  };

  const handleMuxStreamRequest = useCallback(
    async (startSeconds: number): Promise<string | null> => {
      const mediaFile = selectedMediaFileRef.current;
      if (!mediaFile) {
        return null;
      }

      return getTorrentService().createMuxStreamUrl(
        mediaFile.file,
        selectedAudioTrackIndex,
        startSeconds,
      );
    },
    [getTorrentService, selectedAudioTrackIndex],
  );

  const handleAudioTrackChange = (trackIndex: number | null) => {
    setSelectedAudioTrackSelection(trackIndex);

    if (
      peerRole !== "master" ||
      isLoadingTorrentRef.current ||
      !currentTorrentSourceRef.current ||
      !p2pServiceRef.current?.isHost() ||
      !p2pServiceRef.current.isConnected()
    ) {
      return;
    }

    broadcastCurrentRoomState();
  };

  const handleSubtitleTrackChange = (trackIndex: number | null) => {
    setSelectedSubtitleSelection(trackIndex);

    if (
      peerRole !== "master" ||
      isLoadingTorrentRef.current ||
      !currentTorrentSourceRef.current ||
      !p2pServiceRef.current?.isHost() ||
      !p2pServiceRef.current.isConnected()
    ) {
      return;
    }

    broadcastCurrentRoomState();
  };

  const handleTimeUpdate = useCallback(
    (currentTime: number, videoDuration: number) => {
      const mediaFile = selectedMediaFileRef.current;
      if (!mediaFile) return;
      getTorrentService().updatePlaybackPosition(currentTime, mediaFile.length, videoDuration);
    },
    [getTorrentService],
  );

  const handleSyncToleranceChange = (value: number) => {
    const nextTolerance = clampSyncTolerance(value);
    setSyncToleranceSeconds(nextTolerance);
    syncServiceRef.current?.setSyncToleranceSeconds(nextTolerance);

    if (p2pServiceRef.current?.isHost() && p2pServiceRef.current.isConnected()) {
      p2pServiceRef.current.sendRoomConfig({ syncToleranceSeconds: nextTolerance, roomPassword });
    }
  };

  const handleBufferSettingsChange = useCallback(
    (bufferWindowMB: number, maxBufferMB: number) => {
      getTorrentService().setBufferSettings(bufferWindowMB, maxBufferMB);
      setBufferWindowMB(bufferWindowMB);
      setMaxBufferMB(maxBufferMB);
    },
    [getTorrentService],
  );

  const handleSeek = useCallback(
    (timestamp: number) => {
      if (peerRole === "master" && syncServiceRef.current) {
        syncServiceRef.current.seek(timestamp);
      }
      // Immediately reprioritize buffer for the new position.
      getTorrentService().prioritizeNow();
    },
    [peerRole, getTorrentService],
  );

  // Use the selected file's individual progress when available;
  // fall back to overall torrent progress for the general buffer indicator.
  const selectedMediaBufferProgress = Math.round(
    ((selectedMediaFileRef.current?.file.progress != null && selectedMediaFileRef.current.file.progress > 0)
      ? selectedMediaFileRef.current.file.progress
      : torrentProgress / 100) * 100,
  );

  const torrentPeerHint =
    getTorrentService().isElectronBackendEnabled()
      ? torrentPeerCount > 0
        ? `${torrentPeerCount} public peer${torrentPeerCount === 1 ? "" : "s"} discovered via tracker, DHT, and PEX`
        : "Looking for public peers via tracker, DHT, and PEX"
      : torrentPeerCount > 0
        ? `${torrentPeerCount} public WebRTC peer${torrentPeerCount === 1 ? "" : "s"} discovered via trackers`
        : "Looking for public WebRTC peers";

  const bufferHint = selectedMediaFile
    ? selectedMediaBufferProgress >= 100
      ? "Selected file is fully buffered."
      : selectedMediaBufferProgress > 0
        ? "Buffering — loading data around current position..."
        : "Selected file is buffering from the swarm."
    : "Load a torrent and pick a movie to see file buffering progress.";

  const sharedTorrentLabel = currentTorrentSourceRef.current
    ? currentTorrentSourceRef.current.kind === "magnet"
      ? "Magnet link"
      : currentTorrentSourceRef.current.fileName
    : torrentFile?.name ?? null;

  const trackerLostRef = useRef(trackerLost);
  useEffect(() => {
    trackerLostRef.current = trackerLost;
  }, [trackerLost]);

  useEffect(() => {
    const torrentService = getTorrentService();
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
      if (peerCount > 0 && trackerLostRef.current) {
        setTrackerLost(false);
      } else if (peerCount === 0 && selectedMediaFileRef.current && !isLoadingTorrentRef.current && currentTorrentSourceRef.current) {
        setTrackerLost(true);
      }
    });

    return () => {
      offTorrentError();
      offTorrentProgress();
      offTorrentSpeed();
      offTorrentPeerCount();
    };
  }, [getTorrentService, torrentServiceVersion]);

  useEffect(() => {
    disposeSyncService();

    if (currentView !== "room" || !videoRef.current || !peerRole || !p2pServiceRef.current) {
      return;
    }

    const p2pSyncTransport = {
      sendSync: (message: SyncMessage) => {
        if (p2pServiceRef.current?.isConnected()) {
          p2pServiceRef.current.sendSync(enrichSyncMessage(message));
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
    if (peerRole === "master" && currentTorrentSourceRef.current) {
      broadcastCurrentRoomState();
    }
    return () => {
      syncService.dispose();
      if (syncServiceRef.current === syncService) {
        syncServiceRef.current = null;
      }
    };
    // syncServiceRef and videoRef are stable refs; enrichSyncMessage, broadcastCurrentRoomState,
    // and tryApplyPendingRemoteSync use only refs/stable callbacks — safe to exclude from deps.
    // Re-create SyncService when torrentServiceVersion or syncToleranceSeconds changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentView, peerRole, torrentServiceVersion, syncToleranceSeconds]);

  // Cleanup on unmount / window close
  useEffect(() => {
    const doCleanup = () => {
      p2pServiceRef.current?.disconnect();
      p2pServiceRef.current = null;
      disposeSyncService();
      torrentServiceRef.current?.destroy().catch(() => undefined);
      torrentServiceRef.current = null;
      const video = videoRef.current;
      video?.pause();
      video?.removeAttribute("src");
      video?.load();
    };
    const onBeforeUnload = () => { doCleanup(); };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => {
      window.removeEventListener("beforeunload", onBeforeUnload);
      doCleanup();
    };
  }, []);

  useEffect(() => {
    if (
      currentView !== "room" ||
      !isPlayerReady ||
      !videoRef.current ||
      isProcessingTorrentLoadRef.current
    ) {
      return;
    }

    if (pendingTorrentLoadRef.current) {
      void processTorrentLoadQueueRef.current();
    }

    tryApplyPendingRemoteSync();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentView, isPlayerReady]);

  return (
    <main className="app-shell">
      {currentView === "home" ? (
        <HomePage
          peerId={peerId}
          onCreateRoom={handleCreateRoom}
          onJoinRoom={handleJoinRoom}
          isConnecting={isConnecting}
          connectionError={connectionError}
          roomPassword={roomPassword}
          onRoomPasswordChange={setRoomPassword}
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
selectedMediaAudioTracks={selectedMediaAudioTracks}
           selectedAudioTrackIndex={selectedAudioTrackIndex}
           selectedSubtitles={selectedSubtitles}
           selectedSubtitleIndex={selectedSubtitleIndex}
           onSubtitleTrackChange={handleSubtitleTrackChange}
           torrentPeerCount={torrentPeerCount}
          syncToleranceSeconds={syncToleranceSeconds}
          onSyncToleranceChange={handleSyncToleranceChange}
          onMagnetLinkChange={setMagnetLink}
          onTorrentFileChange={handleTorrentFileChange}
          videoRef={videoRef}
          playbackNotice={playbackNotice}
          onPlaybackStarted={() => setPlaybackNotice(null)}
          onAudioTrackChange={handleAudioTrackChange}
          onPlayerReady={(ready) => {
            isPlayerReadyRef.current = ready;
            setIsPlayerReady(ready);
            if (ready) {
              tryApplyPendingRemoteSync();
            }
          }}
          onLoadMagnet={() => void handleLoadMagnet()}
          onLoadTorrentFile={() => void handleLoadTorrentFile()}
          onSelectMediaFile={handleSelectMediaFile}
          onLeaveRoom={handleLeaveRoom}
          onResetTorrentInRoom={handleResetTorrentInRoom}
          isLoadingTorrent={isLoadingTorrent}
          downloadSpeed={downloadSpeed}
          bufferingProgress={selectedMediaBufferProgress}
          torrentError={torrentError}
          torrentPeerHint={torrentPeerHint}
          bufferHint={bufferHint}
          trackerLost={trackerLost}
          onTimeUpdate={handleTimeUpdate}
          bufferWindowMB={bufferWindowMB}
          maxBufferMB={maxBufferMB}
          onBufferSettingsChange={handleBufferSettingsChange}
          onSeek={handleSeek}
          onMuxStreamRequest={handleMuxStreamRequest}
        />
      )}
    </main>
  );
}

export default App;
