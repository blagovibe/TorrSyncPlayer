import { SyncAction, SyncMessage } from "./types";

type SignalingEvents = {
  connected: () => void;
  disconnected: () => void;
  peer_joined: (peerId: string) => void;
  peer_left: (peerId: string) => void;
  offer: (payload: { from: string; sdp: RTCSessionDescriptionInit }) => void;
  answer: (payload: { from: string; sdp: RTCSessionDescriptionInit }) => void;
  ice: (payload: { from: string; candidate: RTCIceCandidateInit }) => void;
  sync: (payload: SyncMessage & { from?: string }) => void;
  room_created: (payload: { code: string }) => void;
  joined: (payload: { code: string; peers?: string[] }) => void;
};

type EventKey = keyof SignalingEvents;

type OutboundMessage =
  | { type: "create_room" }
  | { type: "join_room"; roomCode: string }
  | { type: "leave_room" }
  | { type: "offer"; target: string; sdp: RTCSessionDescriptionInit }
  | { type: "answer"; target: string; sdp: RTCSessionDescriptionInit }
  | { type: "ice"; target: string; candidate: RTCIceCandidateInit }
  | { type: "sync"; action: SyncAction; position: number; server_ts: number };

export class SignalingService {
  private readonly url: string;
  private ws: WebSocket | null = null;
  private reconnectAttempt = 0;
  private reconnectTimer: number | null = null;
  private manuallyClosed = false;
  private listeners: { [K in EventKey]: Set<SignalingEvents[K]> } = {
    connected: new Set(),
    disconnected: new Set(),
    peer_joined: new Set(),
    peer_left: new Set(),
    offer: new Set(),
    answer: new Set(),
    ice: new Set(),
    sync: new Set(),
    room_created: new Set(),
    joined: new Set(),
  };

  constructor(url = "ws://localhost:8080") {
    this.url = url;
  }

  on<K extends EventKey>(event: K, callback: SignalingEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  connect(): void {
    this.manuallyClosed = false;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      return;
    }

    this.ws = new WebSocket(this.url);

    this.ws.onopen = () => {
      this.reconnectAttempt = 0;
      this.emit("connected");
    };

    this.ws.onclose = () => {
      this.emit("disconnected");
      if (!this.manuallyClosed) {
        this.scheduleReconnect();
      }
    };

    this.ws.onerror = () => {
      this.ws?.close();
    };

    this.ws.onmessage = (event) => {
      this.handleMessage(event.data);
    };
  }

  createRoom(): void {
    this.send({ type: "create_room" });
  }

  joinRoom(roomCode: string): void {
    this.send({ type: "join_room", roomCode });
  }

  leaveRoom(): void {
    this.manuallyClosed = true;
    this.send({ type: "leave_room" });
    this.ws?.close();
    this.clearReconnectTimer();
  }

  sendOffer(target: string, sdp: RTCSessionDescriptionInit): void {
    this.send({ type: "offer", target, sdp });
  }

  sendAnswer(target: string, sdp: RTCSessionDescriptionInit): void {
    this.send({ type: "answer", target, sdp });
  }

  sendIce(target: string, candidate: RTCIceCandidateInit): void {
    this.send({ type: "ice", target, candidate });
  }

  sendSync(action: SyncAction, position: number): void {
    this.send({ type: "sync", action, position, server_ts: Date.now() });
  }

  private send(payload: OutboundMessage): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return;
    }
    this.ws.send(JSON.stringify(payload));
  }

  private scheduleReconnect(): void {
    this.clearReconnectTimer();
    const delayMs = Math.min(1000 * 2 ** this.reconnectAttempt, 15000);
    this.reconnectAttempt += 1;
    this.reconnectTimer = window.setTimeout(() => this.connect(), delayMs);
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private handleMessage(rawData: string): void {
    let message: any;
    try {
      message = JSON.parse(rawData);
    } catch {
      return;
    }

    switch (message.type) {
      case "peer_joined":
        this.emit("peer_joined", message.peerId);
        break;
      case "peer_left":
        this.emit("peer_left", message.peerId);
        break;
      case "offer":
        this.emit("offer", { from: message.from, sdp: message.sdp });
        break;
      case "answer":
        this.emit("answer", { from: message.from, sdp: message.sdp });
        break;
      case "ice":
        this.emit("ice", { from: message.from, candidate: message.candidate });
        break;
      case "sync":
        this.emit("sync", {
          action: message.action,
          position: message.position,
          server_ts: message.server_ts,
          from: message.from,
        });
        break;
      case "room_created":
        this.emit("room_created", { code: message.code });
        break;
      case "joined":
        this.emit("joined", { code: message.code, peers: message.peers });
        break;
      default:
        break;
    }
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<SignalingEvents[K]>) {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<SignalingEvents[K]>) => void)(...args);
    }
  }
}

export default SignalingService;
