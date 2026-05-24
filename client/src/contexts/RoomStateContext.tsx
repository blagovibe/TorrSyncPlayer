import { createContext, useReducer, useCallback, type ReactNode } from "react";
import type { AudioTrackInfo, SharedTorrentSource, SubtitleTrackInfo, SyncMessage } from "../services/types";
import type { TorrentMediaFile } from "../services/TorrentService";

interface RoomState {
  currentTorrentSource: SharedTorrentSource | null;
  selectedMediaIndex: number | null;
  selectedMediaFile: TorrentMediaFile | null;
  selectedMediaLabel: string | null;
  selectedMediaKind: TorrentMediaFile["kind"] | null;
  selectedMediaAudioTracks: AudioTrackInfo[];
  selectedSubtitles: SubtitleTrackInfo[];
  selectedAudioTrackIndex: number | null;
  selectedSubtitleIndex: number | null;
  pendingRemoteSync: SyncMessage | null;
  peerRole: "master" | "slave" | null;
}

type RoomAction =
  | { type: "SET_TORRENT_SOURCE"; source: SharedTorrentSource | null }
  | { type: "SET_MEDIA_INDEX"; index: number | null }
  | { type: "SET_MEDIA_FILE"; file: TorrentMediaFile | null }
  | { type: "SET_MEDIA_LABEL"; label: string | null }
  | { type: "SET_MEDIA_KIND"; kind: TorrentMediaFile["kind"] | null }
  | { type: "SET_MEDIA_AUDIO_TRACKS"; tracks: AudioTrackInfo[] }
  | { type: "SET_SUBTITLES"; subtitles: SubtitleTrackInfo[] }
  | { type: "SET_AUDIO_TRACK_INDEX"; index: number | null }
  | { type: "SET_SUBTITLE_INDEX"; index: number | null }
  | { type: "SET_PENDING_SYNC"; sync: SyncMessage | null }
  | { type: "SET_PEER_ROLE"; role: "master" | "slave" | null }
  | { type: "RESET" };

const initialState: RoomState = {
  currentTorrentSource: null,
  selectedMediaIndex: null,
  selectedMediaFile: null,
  selectedMediaLabel: null,
  selectedMediaKind: null,
  selectedMediaAudioTracks: [],
  selectedSubtitles: [],
  selectedAudioTrackIndex: null,
  selectedSubtitleIndex: null,
  pendingRemoteSync: null,
  peerRole: null,
};

function roomReducer(state: RoomState, action: RoomAction): RoomState {
  switch (action.type) {
    case "SET_TORRENT_SOURCE":
      return { ...state, currentTorrentSource: action.source };
    case "SET_MEDIA_INDEX":
      return { ...state, selectedMediaIndex: action.index };
    case "SET_MEDIA_FILE":
      return { ...state, selectedMediaFile: action.file };
    case "SET_MEDIA_LABEL":
      return { ...state, selectedMediaLabel: action.label };
    case "SET_MEDIA_KIND":
      return { ...state, selectedMediaKind: action.kind };
    case "SET_MEDIA_AUDIO_TRACKS":
      return { ...state, selectedMediaAudioTracks: action.tracks };
    case "SET_SUBTITLES":
      return { ...state, selectedSubtitles: action.subtitles };
    case "SET_AUDIO_TRACK_INDEX":
      return { ...state, selectedAudioTrackIndex: action.index };
    case "SET_SUBTITLE_INDEX":
      return { ...state, selectedSubtitleIndex: action.index };
    case "SET_PENDING_SYNC":
      return { ...state, pendingRemoteSync: action.sync };
    case "SET_PEER_ROLE":
      return { ...state, peerRole: action.role };
    case "RESET":
      return initialState;
    default:
      return state;
  }
}

interface RoomStateContextValue {
  state: RoomState;
  setTorrentSource: (source: SharedTorrentSource | null) => void;
  setMediaIndex: (index: number | null) => void;
  setMediaFile: (file: TorrentMediaFile | null) => void;
  setMediaLabel: (label: string | null) => void;
  setMediaKind: (kind: TorrentMediaFile["kind"] | null) => void;
  setMediaAudioTracks: (tracks: AudioTrackInfo[]) => void;
  setSubtitles: (subtitles: SubtitleTrackInfo[]) => void;
  setAudioTrackIndex: (index: number | null) => void;
  setSubtitleIndex: (index: number | null) => void;
  setPendingSync: (sync: SyncMessage | null) => void;
  setPeerRole: (role: "master" | "slave" | null) => void;
  reset: () => void;
  getCurrentSourceKey: () => string | null;
}

const RoomStateContext = createContext<RoomStateContextValue | null>(null);

export function RoomStateProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(roomReducer, initialState);

  const setTorrentSource = useCallback(
    (source: SharedTorrentSource | null) => dispatch({ type: "SET_TORRENT_SOURCE", source }),
    [],
  );
  const setMediaIndex = useCallback(
    (index: number | null) => dispatch({ type: "SET_MEDIA_INDEX", index }),
    [],
  );
  const setMediaFile = useCallback(
    (file: TorrentMediaFile | null) => dispatch({ type: "SET_MEDIA_FILE", file }),
    [],
  );
  const setMediaLabel = useCallback(
    (label: string | null) => dispatch({ type: "SET_MEDIA_LABEL", label }),
    [],
  );
  const setMediaKind = useCallback(
    (kind: TorrentMediaFile["kind"] | null) => dispatch({ type: "SET_MEDIA_KIND", kind }),
    [],
  );
  const setMediaAudioTracks = useCallback(
    (tracks: AudioTrackInfo[]) => dispatch({ type: "SET_MEDIA_AUDIO_TRACKS", tracks }),
    [],
  );
  const setSubtitles = useCallback(
    (subtitles: SubtitleTrackInfo[]) => dispatch({ type: "SET_SUBTITLES", subtitles }),
    [],
  );
  const setAudioTrackIndex = useCallback(
    (index: number | null) => dispatch({ type: "SET_AUDIO_TRACK_INDEX", index }),
    [],
  );
  const setSubtitleIndex = useCallback(
    (index: number | null) => dispatch({ type: "SET_SUBTITLE_INDEX", index }),
    [],
  );
  const setPendingSync = useCallback(
    (sync: SyncMessage | null) => dispatch({ type: "SET_PENDING_SYNC", sync }),
    [],
  );
  const setPeerRole = useCallback(
    (role: "master" | "slave" | null) => dispatch({ type: "SET_PEER_ROLE", role }),
    [],
  );
  const reset = useCallback(() => dispatch({ type: "RESET" }), []);
  const getCurrentSourceKey = useCallback(
    () => state.currentTorrentSource?.sourceKey ?? null,
    [state.currentTorrentSource],
  );

  return (
    <RoomStateContext.Provider
      value={{
        state,
        setTorrentSource,
        setMediaIndex,
        setMediaFile,
        setMediaLabel,
        setMediaKind,
        setMediaAudioTracks,
        setSubtitles,
        setAudioTrackIndex,
        setSubtitleIndex,
        setPendingSync,
        setPeerRole,
        reset,
        getCurrentSourceKey,
      }}
    >
      {children}
    </RoomStateContext.Provider>
  );
}

export { RoomStateContext };
export type { RoomState, RoomStateContextValue };
