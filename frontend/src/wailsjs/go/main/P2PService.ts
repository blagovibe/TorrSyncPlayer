// Автоматически генерируется Wails

export function CreateRoom(): Promise<string> {
  return window.go?.main?.P2PService?.CreateRoom() || Promise.reject('Wails not available')
}

export function JoinRoom(roomID: string, offerSDP: string): Promise<string> {
  return window.go?.main?.P2PService?.JoinRoom(roomID, offerSDP) || Promise.reject('Wails not available')
}

export function CreateOffer(peerID: string): Promise<string> {
  return window.go?.main?.P2PService?.CreateOffer(peerID) || Promise.reject('Wails not available')
}

export function HandleAnswer(peerID: string, answerSDP: string): Promise<void> {
  return window.go?.main?.P2PService?.HandleAnswer(peerID, answerSDP) || Promise.reject('Wails not available')
}

export function SendMessage(msgType: string, data: any): Promise<void> {
  return window.go?.main?.P2PService?.SendMessage(msgType, data) || Promise.reject('Wails not available')
}

export function SendMessageToPeer(peerID: string, msgType: string, data: any): Promise<void> {
  return window.go?.main?.P2PService?.SendMessageToPeer(peerID, msgType, data) || Promise.reject('Wails not available')
}

export function GetPeers(): Promise<any> {
  return window.go?.main?.P2PService?.GetPeers() || Promise.reject('Wails not available')
}

export function IsHost(): Promise<boolean> {
  return window.go?.main?.P2PService?.IsHost() || Promise.reject('Wails not available')
}

export function GetRoomID(): Promise<string> {
  return window.go?.main?.P2PService?.GetRoomID() || Promise.reject('Wails not available')
}

export function GetLocalPeerID(): Promise<string> {
  return window.go?.main?.P2PService?.GetLocalPeerID() || Promise.reject('Wails not available')
}

export function Disconnect(): Promise<void> {
  return window.go?.main?.P2PService?.Disconnect() || Promise.reject('Wails not available')
}

export function Play(timestamp: number): Promise<void> {
  return window.go?.main?.P2PService?.Play(timestamp) || Promise.reject('Wails not available')
}

export function Pause(timestamp: number): Promise<void> {
  return window.go?.main?.P2PService?.Pause(timestamp) || Promise.reject('Wails not available')
}

export function Seek(position: number): Promise<void> {
  return window.go?.main?.P2PService?.Seek(position) || Promise.reject('Wails not available')
}

export function SendState(state: any): Promise<void> {
  return window.go?.main?.P2PService?.SendState(state) || Promise.reject('Wails not available')
}

export function SendChatMessage(message: string): Promise<void> {
  return window.go?.main?.P2PService?.SendChatMessage(message) || Promise.reject('Wails not available')
}

export function SendTorrentInfo(info: any): Promise<void> {
  return window.go?.main?.P2PService?.SendTorrentInfo(info) || Promise.reject('Wails not available')
}

export function AddICECandidate(peerID: string, candidate: string): Promise<void> {
  return window.go?.main?.P2PService?.AddICECandidate(peerID, candidate) || Promise.reject('Wails not available')
}
