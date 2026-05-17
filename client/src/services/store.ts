/**
 * Zustand-like state management without external dependencies.
 * Minimal implementation with subscribe/setState pattern.
 */

type Listener = () => void;
type Selector<T, R> = (state: T) => R;

export interface Store<T extends object> {
  getState: () => T;
  setState: (partial: Partial<T> | ((state: T) => Partial<T>)) => void;
  subscribe: (listener: Listener) => () => void;
  useStore: <R>(selector: Selector<T, R>) => R;
}

export function createStore<T extends object>(
  initialState: T,
): Store<T> {
  const state: T = { ...initialState };
  const listeners: Listener[] = [];

  function getState(): T {
    return state;
  }

  function setState(partial: Partial<T> | ((state: T) => Partial<T>)): void {
    const next = typeof partial === "function" ? partial(state) : partial;
    const keys = Object.keys(next) as (keyof T)[];
    for (let i = 0; i < keys.length; i++) {
      (state as Record<string, unknown>)[keys[i] as string] = next[keys[i]];
    }
    for (let i = 0; i < listeners.length; i++) {
      listeners[i]();
    }
  }

  function subscribe(listener: Listener): () => void {
    listeners.push(listener);
    return () => {
      const idx = listeners.indexOf(listener);
      if (idx !== -1) listeners.splice(idx, 1);
    };
  }

  function useStore<R>(selector: Selector<T, R>): R {
    return selector(state);
  }

  return { getState, setState, subscribe, useStore };
}

/**
 * Application state interface.
 */
export interface AppState {
  // View
  currentView: "home" | "room";

  // P2P
  peerId: string;
  peerRole: "master" | "slave" | null;
  isConnected: boolean;
  isConnecting: boolean;
  connectionError: string | null;

  // Torrent
  isLoadingTorrent: boolean;
  torrentProgress: number;
  downloadSpeed: string;
  torrentError: string | null;
  torrentPeerCount: number;
  trackerLost: boolean;

  // Media
  magnetLink: string;
  torrentFileName: string | null;
  sharedSourceLabel: string | null;
  mediaFiles: unknown[];
  selectedMediaIndex: number | null;
  selectedMediaLabel: string | null;
  selectedMediaKind: "video" | "audio" | null;
  selectedMediaAudioTracks: unknown[];
  selectedAudioTrackIndex: number | null;

  // Sync
  syncToleranceSeconds: number;
  playbackNotice: string | null;
  isPlayerReady: boolean;

  // Buffer
  bufferWindowMB: number;
  maxBufferMB: number;
}

export const appStore = createStore<AppState>({
  currentView: "home",
  peerId: "",
  peerRole: null,
  isConnected: false,
  isConnecting: false,
  connectionError: null,
  isLoadingTorrent: false,
  torrentProgress: 0,
  downloadSpeed: "0 B/s",
  torrentError: null,
  torrentPeerCount: 0,
  trackerLost: false,
  magnetLink: "",
  torrentFileName: null,
  sharedSourceLabel: null,
  mediaFiles: [],
  selectedMediaIndex: null,
  selectedMediaLabel: null,
  selectedMediaKind: null,
  selectedMediaAudioTracks: [],
  selectedAudioTrackIndex: null,
  syncToleranceSeconds: 0.5,
  playbackNotice: null,
  isPlayerReady: false,
  bufferWindowMB: 50,
  maxBufferMB: 500,
});
