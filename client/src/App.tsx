import { useEffect, useMemo, useRef, useState } from "react";
import HomePage from "./components/HomePage";
import RoomPage from "./components/RoomPage";
import P2PService from "./services/P2PService";
import SyncService from "./services/SyncService";
import TorrentService, { type TorrentMediaFile } from "./services/TorrentService";

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

function App() {
  const [currentView, setCurrentView] = useState<View>("home");
  const [peerId, setPeerId] = useState("");
  const [_remotePeerId, setRemotePeerId] = useState("");
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

  const videoRef = useRef<HTMLVideoElement | null>(null);
  const p2pServiceRef = useRef<P2PService | null>(null);
  const torrentService = useMemo(() => new TorrentService(), []);
  const syncServiceRef = useRef<SyncService | null>(null);

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
  };

  const playMediaFile = async (mediaFile: TorrentMediaFile) => {
    const mediaElement = videoRef.current;
    if (!mediaElement) {
      throw new Error("Media player is not ready");
    }

    await torrentService.streamToMedia(mediaFile.file, mediaElement);
    setSelectedMediaFile(mediaFile);
    setSelectedMediaIndex(mediaFile.index);
    setSelectedMediaLabel(mediaFile.name);
    setSelectedMediaKind(mediaFile.kind);

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

  const loadTorrentSource = async (loader: () => Promise<unknown>) => {
    if (!videoRef.current) {
      setTorrentError("Media player is not ready");
      return;
    }

    setIsLoadingTorrent(true);
    setTorrentError(null);
    setTorrentProgress(0);
    setDownloadSpeed("0 B/s");
    setMediaFiles([]);
    setSelectedMediaIndex(null);
    setSelectedMediaLabel(null);
    setSelectedMediaKind(null);
    setSelectedMediaFile(null);
    setTorrentPeerCount(0);
    setPlaybackNotice(null);

    try {
      const torrent = (await loader()) as Awaited<ReturnType<typeof torrentService.addMagnet>>;
      const playableMediaFiles = torrentService.getPlayableMediaFiles(torrent);
      setMediaFiles(playableMediaFiles);

      if (playableMediaFiles.length === 0) {
        throw new Error("No supported video or audio file found in torrent");
      }

      const preferredMediaFile = torrentService.getPreferredMediaFile(torrent);
      await playMediaFile(preferredMediaFile);
      setIsLoadingTorrent(false);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unable to load torrent";
      setTorrentError(message);
      setIsLoadingTorrent(false);
      throw error;
    }
  };

  const initializeP2PService = async (role: "host" | "guest") => {
    p2pServiceRef.current?.disconnect();
    disposeSyncService();

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
    });

    p2pService.on("peer_disconnected", (disconnectedPeerId) => {
      setPeers((prev) => prev.filter((peer) => peer.id !== disconnectedPeerId));
    });

    p2pService.on("sync", (message) => {
      syncServiceRef.current?.applyRemoteSync(message);
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
    setRemotePeerId(normalizedId);
    setPeerRole("slave");
    setPeers([
      { id: "self", name: "You", role: "slave", connectionState: "connected" },
      { id: normalizedId, name: "Host", role: "master", connectionState: "connecting" },
    ]);

    try {
      const p2pService = await initializeP2PService("guest");
      await p2pService.connect(`torrsync-${normalizedId}`);
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
    setRemotePeerId("");
    setPeerRole(null);
    setPeers([]);
    setIsConnected(false);
    setIsConnecting(false);
    setConnectionError(null);
    setPlaybackNotice(null);
    resetTorrentState();
  };

  const handleLoadMagnet = async () => {
    if (!magnetLink.trim() || !videoRef.current) {
      return;
    }
    try {
      await loadTorrentSource(() => torrentService.addMagnet(magnetLink.trim()));
    } catch (error) {
      console.error("Torrent load failed:", error);
    }
  };

  const handleLoadTorrentFile = async () => {
    if (!torrentFile || !videoRef.current) {
      return;
    }

    try {
      const torrentBytes = new Uint8Array(await torrentFile.arrayBuffer());
      await loadTorrentSource(() => torrentService.addTorrentFile(torrentBytes));
    } catch (error) {
      console.error("Torrent file load failed:", error);
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

  useEffect(() => {
    const offTorrentError = torrentService.on("error", (error) => {
      console.error("Torrent error", error);
      setTorrentError(error.message);
      setIsLoadingTorrent(false);
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
      sendSync: (action: string, position: number) => {
        if (p2pServiceRef.current?.isConnected()) {
          p2pServiceRef.current.sendSync(action as "play" | "pause" | "seek", position);
        }
      },
    };

    const syncService = new SyncService(p2pSyncTransport, videoRef.current, peerRole);
    syncServiceRef.current = syncService;
    return () => {
      syncService.dispose();
      if (syncServiceRef.current === syncService) {
        syncServiceRef.current = null;
      }
    };
  }, [currentView, peerRole]);

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
          magnetLink={magnetLink}
          torrentFileName={torrentFile?.name ?? null}
          mediaFiles={mediaFiles}
          selectedMediaIndex={selectedMediaIndex}
          selectedMediaLabel={selectedMediaLabel}
          selectedMediaKind={selectedMediaKind}
          torrentPeerCount={torrentPeerCount}
          onMagnetLinkChange={setMagnetLink}
          onTorrentFileChange={setTorrentFile}
          videoRef={videoRef}
          playbackNotice={playbackNotice}
          onPlaybackStarted={() => setPlaybackNotice(null)}
          onLoadMagnet={() => void handleLoadMagnet()}
          onLoadTorrentFile={() => void handleLoadTorrentFile()}
          onSelectMediaFile={async (mediaFile) => {
            if (!videoRef.current) {
              return;
            }
            setIsLoadingTorrent(true);
            setTorrentError(null);
            setPlaybackNotice(null);
            try {
              await playMediaFile(mediaFile);
            } catch (error) {
              const message = error instanceof Error ? error.message : "Unable to play selected media";
              setTorrentError(message);
            } finally {
              setIsLoadingTorrent(false);
            }
          }}
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
