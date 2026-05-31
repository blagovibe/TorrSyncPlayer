// Автоматически генерируется Wails

export function Play(position: number): Promise<void> {
  return window.go?.main?.SyncService?.Play(position) || Promise.reject('Wails not available')
}

export function Pause(position: number): Promise<void> {
  return window.go?.main?.SyncService?.Pause(position) || Promise.reject('Wails not available')
}

export function Seek(position: number): Promise<void> {
  return window.go?.main?.SyncService?.Seek(position) || Promise.reject('Wails not available')
}

export function UpdateState(state: any): Promise<void> {
  return window.go?.main?.SyncService?.UpdateState(state) || Promise.reject('Wails not available')
}

export function HandleSyncCommand(cmd: any): Promise<void> {
  return window.go?.main?.SyncService?.HandleSyncCommand(cmd) || Promise.reject('Wails not available')
}

export function GetState(): Promise<any> {
  return window.go?.main?.SyncService?.GetState() || Promise.reject('Wails not available')
}

export function GetStats(): Promise<any> {
  return window.go?.main?.SyncService?.GetStats() || Promise.reject('Wails not available')
}

export function SetSyncTolerance(toleranceMs: number): Promise<void> {
  return window.go?.main?.SyncService?.SetSyncTolerance(toleranceMs) || Promise.reject('Wails not available')
}

export function StartHeartbeat(): Promise<void> {
  return window.go?.main?.SyncService?.StartHeartbeat() || Promise.reject('Wails not available')
}

export function StopHeartbeat(): Promise<void> {
  return window.go?.main?.SyncService?.StopHeartbeat() || Promise.reject('Wails not available')
}

export function SetPlaybackRate(rate: number): Promise<void> {
  return window.go?.main?.SyncService?.SetPlaybackRate(rate) || Promise.reject('Wails not available')
}

export function SyncNow(): Promise<void> {
  return window.go?.main?.SyncService?.SyncNow() || Promise.reject('Wails not available')
}

export function GetDrift(): Promise<number> {
  return window.go?.main?.SyncService?.GetDrift() || Promise.reject('Wails not available')
}

export function GetRTT(): Promise<number> {
  return window.go?.main?.SyncService?.GetRTT() || Promise.reject('Wails not available')
}
