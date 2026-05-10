import { useEffect, useMemo, useRef, useState } from "react";
import HomePage from "./components/HomePage";
import RoomPage from "./components/RoomPage";
import PeerService from "./services/PeerService";
import SignalingService from "./services/SignalingService";
import SyncService from "./services/SyncService";
import TorrentService from "./services/TorrentService";
import { SyncMessage } from "./services/types";
import "./App.css";

export type View = "home" | "room";
export type PeerRole = "master" | "slave";

export interface Peer {
  id: string;
  name: string;
  role: PeerRole;
}

function App() {
  const [currentView, setCurrentView] = useState<View>("home");
  const [roomCode, setRoomCode] = useState("");
  const [peerRole, setPeerRole] = useState<PeerRole | null>(null);
  const [peers, setPeers] = useState<Peer[]>([]);
  const [isConnected, setIsConnected] = useState(false);
  const [magnetLink, setMagnetLink] = useState("");
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const signalingService = useMemo(() => new SignalingService(), []);
  const peerService = useMemo(() => new PeerService(), []);
  const torrentService = useMemo(() => new TorrentService(), []);
  const syncServiceRef = useRef<SyncService | null>(null);

  const createRoomCode = () =>
    Math.random().toString(36).slice(2, 8).toUpperCase();

  const handleCreateRoom = () => {
    const code = createRoomCode();
    setRoomCode(code);
    setPeerRole("master");
    setIsConnected(true);
    setPeers([{ id: "self", name: "You", role: "master" }]);
    setCurrentView("room");
  };

  const handleJoinRoom = (code: string) => {
    const normalized = code.trim().toUpperCase();
    if (!normalized) {
      return;
    }
    setRoomCode(normalized);
    setPeerRole("slave");
    setIsConnected(true);
    setPeers([
      { id: "self", name: "You", role: "slave" },
      { id: "host-1", name: "Host", role: "master" },
    ]);
    setCurrentView("room");
  };

  const handleLeaveRoom = () => {
    signalingService.leaveRoom();
    peerService.closeAll();
    setCurrentView("home");
    setRoomCode("");
    setPeerRole(null);
    setPeers([]);
    setIsConnected(false);
  };

  const handleLoadMagnet = async () => {
    if (!magnetLink.trim() || !videoRef.current) {
      return;
    }
    const torrent = await torrentService.addMagnet(magnetLink.trim());
    const file = torrentService.getVideoFile(torrent);
    await torrentService.streamToVideo(file, videoRef.current);
  };

  useEffect(() => {
    const offConnected = signalingService.on("connected", () => {
      setIsConnected(true);
      if (peerRole === "master") {
        signalingService.createRoom();
      } else if (roomCode) {
        signalingService.joinRoom(roomCode);
      }
    });

    const offDisconnected = signalingService.on("disconnected", () => {
      setIsConnected(false);
    });

    const offRoomCreated = signalingService.on("room_created", ({ code }) => {
      setRoomCode(code);
    });

    const offJoined = signalingService.on("joined", ({ code, peers: remotePeers }) => {
      setRoomCode(code);
      if (remotePeers?.length) {
        setPeers([
          { id: "self", name: "You", role: peerRole ?? "slave" },
          ...remotePeers.map((peerId) => ({
            id: peerId,
            name: peerId,
            role: "slave" as PeerRole,
          })),
        ]);
      }
    });

    const offPeerJoined = signalingService.on("peer_joined", async (peerId) => {
      setPeers((prev) => {
        if (prev.some((peer) => peer.id === peerId)) {
          return prev;
        }
        return [...prev, { id: peerId, name: peerId, role: "slave" }];
      });

      if (peerRole === "master") {
        const offer = await peerService.createConnection(peerId);
        signalingService.sendOffer(peerId, offer);
      }
    });

    const offPeerLeft = signalingService.on("peer_left", (peerId) => {
      setPeers((prev) => prev.filter((peer) => peer.id !== peerId));
      peerService.close(peerId);
    });

    const offOffer = signalingService.on("offer", async ({ from, sdp }) => {
      const answer = await peerService.handleOffer(from, sdp);
      signalingService.sendAnswer(from, answer);
    });

    const offAnswer = signalingService.on("answer", async ({ from, sdp }) => {
      await peerService.handleAnswer(from, sdp);
    });

    const offIce = signalingService.on("ice", async ({ from, candidate }) => {
      await peerService.handleIce(from, candidate);
    });

    const offSync = signalingService.on("sync", (message) => {
      syncServiceRef.current?.applyRemoteSync(message as SyncMessage);
    });

    const offPeerIce = peerService.on("ice", ({ peerId, candidate }) => {
      signalingService.sendIce(peerId, candidate);
    });

    const offPeerData = peerService.on("data", ({ data }) => {
      const message = data as SyncMessage;
      if (message?.action && typeof message.position === "number") {
        syncServiceRef.current?.applyRemoteSync(message);
      }
    });

    const offTorrentError = torrentService.on("error", (error) => {
      console.error("Torrent error", error);
    });

    return () => {
      offConnected();
      offDisconnected();
      offRoomCreated();
      offJoined();
      offPeerJoined();
      offPeerLeft();
      offOffer();
      offAnswer();
      offIce();
      offSync();
      offPeerIce();
      offPeerData();
      offTorrentError();
    };
  }, [peerRole, roomCode, signalingService, peerService, torrentService]);

  useEffect(() => {
    if (currentView !== "room") {
      return;
    }
    signalingService.connect();
  }, [currentView, signalingService]);

  useEffect(() => {
    if (currentView !== "room" || !videoRef.current || !peerRole) {
      syncServiceRef.current = null;
      return;
    }
    syncServiceRef.current = new SyncService(signalingService, videoRef.current, peerRole);
  }, [currentView, peerRole, signalingService]);

  return (
    <main className="app-shell">
      {currentView === "home" ? (
        <HomePage
          roomCode={roomCode}
          magnetLink={magnetLink}
          onMagnetLinkChange={setMagnetLink}
          onCreateRoom={handleCreateRoom}
          onJoinRoom={handleJoinRoom}
        />
      ) : (
        <RoomPage
          roomCode={roomCode}
          peerRole={peerRole}
          peers={peers}
          isConnected={isConnected}
          magnetLink={magnetLink}
          onMagnetLinkChange={setMagnetLink}
          videoRef={videoRef}
          onLoadMagnet={() => void handleLoadMagnet()}
          onLeaveRoom={handleLeaveRoom}
        />
      )}
    </main>
  );
}

export default App;
