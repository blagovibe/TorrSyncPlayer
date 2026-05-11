import { Peer } from "peerjs";
import { SyncMessage, SyncAction } from "./types";

type P2PEvents = {
  connected: () => void;
  disconnected: () => void;
  peer_connected: (peerId: string) => void;
  peer_disconnected: (peerId: string) => void;
  sync: (message: SyncMessage) => void;
  error: (error: Error) => void;
};

type EventKey = keyof P2PEvents;

const PEER_ID_LENGTH = 6;
const PEER_ID_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
export const WEBRTC_UNAVAILABLE_MESSAGE =
  "WebRTC data channels are not available in the current desktop runtime. Use the Electron build, which ships Chromium with WebRTC support.";

function generatePeerId(): string {
  let id = "";
  for (let i = 0; i < PEER_ID_LENGTH; i++) {
    id += PEER_ID_CHARS.charAt(Math.floor(Math.random() * PEER_ID_CHARS.length));
  }
  return id;
}

function getErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

export function getWebRTCSupportIssue(): string | null {
  if (typeof RTCPeerConnection === "undefined") {
    return "RTCPeerConnection is not exposed by the current WebView";
  }

  let testConnection: RTCPeerConnection | null = null;
  let testChannel: RTCDataChannel | null = null;
  try {
    testConnection = new RTCPeerConnection({ iceServers: [] });
    if (typeof testConnection.createDataChannel !== "function") {
      return "RTCDataChannel is not exposed by the current WebView";
    }

    testChannel = testConnection.createDataChannel("_torrsync_webrtc_test");
    return null;
  } catch (error) {
    return `RTCPeerConnection failed to initialize: ${getErrorMessage(error)}`;
  } finally {
    testChannel?.close();
    testConnection?.close();
  }
}

function createWebRTCUnavailableError(issue: string): Error {
  return new Error(`${WEBRTC_UNAVAILABLE_MESSAGE} (${issue})`);
}

function normalizePeerError(error: unknown): Error {
  const peerError = error as { type?: string; message?: string };
  const message = peerError.message ?? getErrorMessage(error);

  if (
    peerError.type === "browser-incompatible" ||
    message.includes("The current browser does not support WebRTC")
  ) {
    return createWebRTCUnavailableError("PeerJS could not initialize WebRTC data channels");
  }

  return error instanceof Error ? error : new Error(message);
}

export class P2PService {
  private peerId: string;
  private role: "host" | "guest" = "host";
  private peer: Peer | null = null;
  private dataConnection: import("peerjs").DataConnection | null = null;
  private remotePeerId: string | null = null;
  private listeners: { [K in EventKey]: Set<P2PEvents[K]> } = {
    connected: new Set(),
    disconnected: new Set(),
    peer_connected: new Set(),
    peer_disconnected: new Set(),
    sync: new Set(),
    error: new Set(),
  };
  private isConnecting = false;

  constructor() {
    this.peerId = generatePeerId();
  }

  getPeerId(): string {
    return this.peerId;
  }

  setHost(): void {
    this.role = "host";
  }

  setGuest(): void {
    this.role = "guest";
  }

  isHost(): boolean {
    return this.role === "host";
  }

  on<K extends EventKey>(event: K, callback: P2PEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  async connect(remotePeerId: string): Promise<void> {
    if (this.isConnecting || this.dataConnection?.open) {
      return;
    }

    this.remotePeerId = remotePeerId;
    this.isConnecting = true;

    return new Promise((resolve, reject) => {
      if (!this.peer) {
        this.isConnecting = false;
        reject(new Error("Peer not initialized"));
        return;
      }

      let timeoutId: number | null = null;
      const conn = this.peer.connect(remotePeerId, {
        reliable: true,
        serialization: "json",
      });

      conn.on("open", () => {
        this.dataConnection = conn;
        this.isConnecting = false;
        if (timeoutId !== null) {
          window.clearTimeout(timeoutId);
        }
        this.emit("connected");
        resolve();
      });

      conn.on("data", (data) => {
        const message = data as SyncMessage;
        if (message?.action && typeof message.position === "number") {
          this.emit("sync", message);
        }
      });

      conn.on("close", () => {
        this.dataConnection = null;
        this.emit("peer_disconnected", remotePeerId);
        this.emit("disconnected");
      });

      conn.on("error", (err) => {
        this.isConnecting = false;
        if (timeoutId !== null) {
          window.clearTimeout(timeoutId);
        }
        this.emit("error", new Error(String(err)));
        reject(err);
      });

      timeoutId = window.setTimeout(() => {
        if (this.isConnecting) {
          this.isConnecting = false;
          conn.close();
          reject(new Error("Connection timeout"));
        }
      }, 30000);
    });
  }

  initialize(): Promise<void> {
    return new Promise((resolve, reject) => {
      if (this.peer) {
        resolve();
        return;
      }

      const webRTCSupportIssue = getWebRTCSupportIssue();
      if (webRTCSupportIssue) {
        reject(createWebRTCUnavailableError(webRTCSupportIssue));
        return;
      }

      if (this.role === "host") {
        this.peer = new Peer(`torrsync-${this.peerId}`, {
          debug: 1,
        });
      } else {
        this.peer = new Peer({
          debug: 1,
        });
      }

      this.peer.on("open", (id) => {
        console.log("PeerJS initialized with ID:", id);
        if (id) {
          this.peerId = id.replace("torrsync-", "");
        }
        resolve();
      });

      this.peer.on("connection", (conn) => {
        this.handleIncomingConnection(conn);
      });

      this.peer.on("error", (err) => {
        const error = normalizePeerError(err);
        console.error("PeerJS error:", err);
        this.emit("error", error);
        if (err.type === "peer-unavailable") {
          reject(new Error("Peer not found"));
        } else {
          reject(error);
        }
      });

      this.peer.on("disconnected", () => {
        this.emit("disconnected");
      });

      setTimeout(() => {
        if (!this.peer?.open) {
          reject(new Error("Initialization timeout"));
        }
      }, 15000);
    });
  }

  private handleIncomingConnection(conn: import("peerjs").DataConnection): void {
    if (this.dataConnection && this.dataConnection.open) {
      conn.close();
      return;
    }

    this.dataConnection = conn;
    this.remotePeerId = conn.peer || null;

    conn.on("open", () => {
      this.emit("peer_connected", conn.peer);
      this.emit("connected");
    });

    conn.on("data", (data) => {
      const message = data as SyncMessage;
      if (message?.action && typeof message.position === "number") {
        this.emit("sync", message);
      }
    });

    conn.on("close", () => {
      this.dataConnection = null;
      const remoteId = this.remotePeerId;
      this.remotePeerId = null;
      if (remoteId) {
        this.emit("peer_disconnected", remoteId);
      }
      this.emit("disconnected");
    });

    conn.on("error", (err) => {
      this.emit("error", new Error(String(err)));
    });
  }

  sendSync(action: SyncAction, position: number): void {
    if (!this.dataConnection || this.dataConnection.open !== true) {
      return;
    }

    this.dataConnection.send({
      action,
      position,
      server_ts: Date.now(),
    } as SyncMessage);
  }

  disconnect(): void {
    if (this.dataConnection) {
      this.dataConnection.close();
      this.dataConnection = null;
    }

    if (this.peer) {
      this.peer.destroy();
      this.peer = null;
    }

    this.remotePeerId = null;
    this.isConnecting = false;
    this.emit("disconnected");
  }

  isConnected(): boolean {
    return this.dataConnection?.open === true;
  }

  getRemotePeerId(): string | null {
    return this.remotePeerId;
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<P2PEvents[K]>) {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<P2PEvents[K]>) => void)(...args);
    }
  }
}

export default P2PService;
