type PeerEvents = {
  ice: (payload: { peerId: string; candidate: RTCIceCandidateInit }) => void;
  data: (payload: { peerId: string; data: unknown }) => void;
  open: (peerId: string) => void;
  close: (peerId: string) => void;
  error: (payload: { peerId: string; error: Error }) => void;
};

type EventKey = keyof PeerEvents;

export class PeerService {
  private readonly rtcConfig: RTCConfiguration = {
    iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
  };
  private connections = new Map<string, RTCPeerConnection>();
  private channels = new Map<string, RTCDataChannel>();
  private listeners: { [K in EventKey]: Set<PeerEvents[K]> } = {
    ice: new Set(),
    data: new Set(),
    open: new Set(),
    close: new Set(),
    error: new Set(),
  };

  on<K extends EventKey>(event: K, callback: PeerEvents[K]): () => void {
    this.listeners[event].add(callback);
    return () => this.listeners[event].delete(callback);
  }

  async createConnection(peerId: string): Promise<RTCSessionDescriptionInit> {
    const pc = this.getOrCreateConnection(peerId);
    const channel = pc.createDataChannel("sync");
    this.bindDataChannel(peerId, channel);

    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    return offer;
  }

  async handleOffer(
    peerId: string,
    sdp: RTCSessionDescriptionInit,
  ): Promise<RTCSessionDescriptionInit> {
    const pc = this.getOrCreateConnection(peerId);
    await pc.setRemoteDescription(new RTCSessionDescription(sdp));
    const answer = await pc.createAnswer();
    await pc.setLocalDescription(answer);
    return answer;
  }

  async handleAnswer(peerId: string, sdp: RTCSessionDescriptionInit): Promise<void> {
    const pc = this.getOrCreateConnection(peerId);
    await pc.setRemoteDescription(new RTCSessionDescription(sdp));
  }

  async handleIce(peerId: string, candidate: RTCIceCandidateInit): Promise<void> {
    const pc = this.getOrCreateConnection(peerId);
    await pc.addIceCandidate(new RTCIceCandidate(candidate));
  }

  sendData(peerId: string, data: unknown): void {
    const channel = this.channels.get(peerId);
    if (!channel || channel.readyState !== "open") {
      return;
    }
    channel.send(JSON.stringify(data));
  }

  close(peerId: string): void {
    this.channels.get(peerId)?.close();
    this.connections.get(peerId)?.close();
    this.channels.delete(peerId);
    this.connections.delete(peerId);
  }

  closeAll(): void {
    for (const peerId of this.connections.keys()) {
      this.close(peerId);
    }
  }

  private getOrCreateConnection(peerId: string): RTCPeerConnection {
    const existing = this.connections.get(peerId);
    if (existing) {
      return existing;
    }

    const pc = new RTCPeerConnection(this.rtcConfig);
    this.connections.set(peerId, pc);

    pc.onicecandidate = (event) => {
      if (event.candidate) {
        this.emit("ice", { peerId, candidate: event.candidate.toJSON() });
      }
    };

    pc.ondatachannel = (event) => {
      this.bindDataChannel(peerId, event.channel);
    };

    pc.onconnectionstatechange = () => {
      if (pc.connectionState === "disconnected" || pc.connectionState === "closed") {
        this.emit("close", peerId);
      }
    };

    return pc;
  }

  private bindDataChannel(peerId: string, channel: RTCDataChannel): void {
    this.channels.set(peerId, channel);

    channel.onopen = () => {
      this.emit("open", peerId);
    };

    channel.onclose = () => {
      this.emit("close", peerId);
    };

    channel.onerror = () => {
      this.emit("error", { peerId, error: new Error("Data channel error") });
    };

    channel.onmessage = (event) => {
      try {
        this.emit("data", { peerId, data: JSON.parse(event.data) });
      } catch {
        this.emit("data", { peerId, data: event.data });
      }
    };
  }

  private emit<K extends EventKey>(event: K, ...args: Parameters<PeerEvents[K]>) {
    for (const callback of this.listeners[event]) {
      (callback as (...eventArgs: Parameters<PeerEvents[K]>) => void)(...args);
    }
  }
}

export default PeerService;
