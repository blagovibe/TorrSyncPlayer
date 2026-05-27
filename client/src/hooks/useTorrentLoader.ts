import { useCallback, useRef, useState } from "react";
import TorrentService, { type TorrentMediaFile } from "../services/TorrentService";
import { type SharedTorrentSource } from "../services/types";
import { createTorrentFileSource } from "../utils/torrent";
import { uiLogger } from "../utils/logger";
import { MAX_TORRENT_FILE_BYTES } from "../config";
import { useRoomStateContext } from "./useRoomStateContext";
import { isPlaybackBlockedError } from "../utils/syncUtils";

const LOAD_COOLDOWN_MS = 5000;
const MAX_RETRIES = 3;

interface LoadRequest {
  source: SharedTorrentSource;
  selectedMediaIndex: number | null;
  selectedAudioTrackIndex: number | null;
  selectedSubtitleIndex: number | null;
  autoplay: boolean;
  broadcast: boolean;
}

export interface TorrentLoader {
  isLoadingTorrent: boolean;
  torrentProgress: number;
  downloadSpeed: string;
  torrentError: string | null;
  torrentPeerCount: number;
  trackerLost: boolean;
  playbackNotice: string | null;
  mediaFiles: TorrentMediaFile[];
  getTorrentService: () => TorrentService;
  loadTorrent: (req: LoadRequest) => void;
  loadTorrentFile: (file: File) => void;
  resetTorrent: () => Promise<void>;
}

export function useTorrentLoader(
  videoRef: React.RefObject<HTMLVideoElement | null>,
  currentView: string,
  scheduleBroadcast: (targetPeerId?: string) => void,
): TorrentLoader {
  const rs = useRoomStateContext();
  const {
    state: { currentTorrentSource, selectedMediaIndex, selectedMediaFile, mediaFiles, peerRole },
    setTorrentSource, setMediaIndex, setMediaFile, setMediaLabel, setMediaKind,
    setMediaAudioTracks, setSubtitles: setCtxSubtitles,
    setAudioTrackIndex, setSubtitleIndex, setPendingSync,
  } = rs;

  const [isLoading, setIsLoading] = useState(false);
  const [progress, setProgress] = useState(0);
  const [speed, setSpeed] = useState("0 B/s");
  const [error, setError] = useState<string | null>(null);
  const [peerCount, setPeerCount] = useState(0);
  const [lost, setLost] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [files, setFiles] = useState<TorrentMediaFile[]>([]);

  const svcRef = useRef<TorrentService | null>(null);
  const pendingRef = useRef<LoadRequest | null>(null);
  const processingRef = useRef(false);
  const loadingRef = useRef(false);
  const readyRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);
  const verRef = useRef(0);
  const retriesRef = useRef(0);
  const lastLoadRef = useRef(0);


  const srcRef = useRef(currentTorrentSource); srcRef.current = currentTorrentSource;
  const idxRef = useRef(selectedMediaIndex); idxRef.current = selectedMediaIndex;
  const audioRef = useRef(rs.state.selectedAudioTrackIndex); audioRef.current = rs.state.selectedAudioTrackIndex;
  const subRef = useRef(rs.state.selectedSubtitleIndex); subRef.current = rs.state.selectedSubtitleIndex;
  const fileRef = useRef(selectedMediaFile); fileRef.current = selectedMediaFile;
  const filesRef = useRef(mediaFiles); filesRef.current = mediaFiles;
  const roleRef = useRef(peerRole); roleRef.current = peerRole;

  const svc = useCallback(() => {
    if (!svcRef.current || svcRef.current.isDestroyed() || svcRef.current.isDestroying()) {
      svcRef.current = new TorrentService();
    }
    return svcRef.current;
  }, []);

  const playMedia = useCallback(async (mf: TorrentMediaFile, autoplay: boolean) => {
    const el = videoRef.current;
    if (!el) throw new Error("Media player is not ready");
    await svc().streamToMedia(mf.file, el);
    setMediaFile(mf);
    setMediaAudioTracks([]);
    setCtxSubtitles([]);
    const probeIndex = mf.index;
    void svc().probeAudioTracks(mf.file).then((t) => {
      if (fileRef.current?.index === probeIndex) setMediaAudioTracks(t);
    }).catch(() => undefined);
    void svc().probeSubtitles(mf.file).then((s) => {
      if (fileRef.current?.index === probeIndex) setCtxSubtitles(s);
    }).catch(() => undefined);
    el.defaultMuted = false; el.muted = false;
    if (el.volume <= 0) el.volume = 1;
    setMediaIndex(mf.index); setMediaLabel(mf.name); setMediaKind(mf.kind);
    if (!autoplay) return;
    try { await el.play(); setNotice(null); } catch (e) {
      if (isPlaybackBlockedError(e)) setNotice("Autoplay was blocked. Press Play to start.");
      else throw e;
    }
  }, [videoRef, svc, setMediaFile, setMediaIndex, setMediaLabel, setMediaKind, setMediaAudioTracks, setCtxSubtitles]);

  const loadReq = useCallback(async (req: LoadRequest) => {
    if (!videoRef.current) throw new Error("Media player is not ready");
    if (abortRef.current?.signal.aborted) throw new Error("Cancelled");
    const ver = verRef.current;
    const curSrc = srcRef.current;

    if (curSrc?.sourceKey === req.source.sourceKey) {
      const desIdx = req.selectedMediaIndex ?? idxRef.current;
      const desAudio = req.selectedAudioTrackIndex ?? audioRef.current;
      const desSub = req.selectedSubtitleIndex ?? subRef.current;
      const curFile = fileRef.current ?? (idxRef.current !== null ? filesRef.current.find((f) => f.index === idxRef.current) ?? null : null);
      if (desAudio !== audioRef.current) setAudioTrackIndex(desAudio);
      if (desSub !== subRef.current) setSubtitleIndex(desSub);
      if (desIdx !== null && desIdx !== idxRef.current) {
        const next = filesRef.current.find((f) => f.index === desIdx);
        if (!next) throw new Error("Requested media file not available");
        await playMedia(next, req.autoplay);
      } else if (req.autoplay && curFile && videoRef.current?.paused) {
        await playMedia(curFile, true);
      }
      if (req.broadcast && roleRef.current === "master") scheduleBroadcast();
      return;
    }

    setFiles([]); setMediaIndex(null); setMediaLabel(null);
    setMediaKind(null); setMediaFile(null); setMediaAudioTracks([]);
    setAudioTrackIndex(req.selectedAudioTrackIndex);
    setSubtitleIndex(req.selectedSubtitleIndex);

    const torrent = req.source.kind === "magnet"
      ? await svc().addMagnet(req.source.magnetLink)
      : await svc().addTorrentFile(new Uint8Array(req.source.bytes));

    if (abortRef.current?.signal.aborted || verRef.current !== ver) return;
    const playable = svc().getPlayableMediaFiles(torrent);
    setFiles(playable);
    if (!playable.length) throw new Error("No supported video or audio file found");
    const preferred = req.selectedMediaIndex !== null
      ? playable.find((f) => f.index === req.selectedMediaIndex) ?? svc().getPreferredMediaFile(torrent)
      : svc().getPreferredMediaFile(torrent);
    await playMedia(preferred, req.autoplay);
    setTorrentSource(req.source);
    if (req.broadcast && roleRef.current === "master") scheduleBroadcast();
  }, [videoRef, svc, playMedia, scheduleBroadcast, setAudioTrackIndex, setSubtitleIndex, setMediaIndex, setMediaFile, setTorrentSource, setMediaAudioTracks, setMediaKind, setMediaLabel]);

  // H2 fix: synchronous queue continuation instead of setTimeout(0) race
  const processQueue = useCallback(async () => {
    if (processingRef.current) return;
    const next = pendingRef.current;
    if (!next || !readyRef.current || !videoRef.current) return;
    if (roleRef.current === "master") {
      const now = Date.now();
      if (now - lastLoadRef.current < LOAD_COOLDOWN_MS) return;
      lastLoadRef.current = now;
    }

    pendingRef.current = null;
    processingRef.current = true;
    loadingRef.current = true;
    setIsLoading(true);
    setError(null); setProgress(0); setSpeed("0 B/s");
    setPeerCount(0); setNotice(null);

    verRef.current++;
    abortRef.current?.abort();
    const ac = new AbortController();
    abortRef.current = ac;
    const loadVer = verRef.current;

    try {
      await loadReq(next);
    } catch (err) {
      if (ac.signal.aborted) return;
      const msg = err instanceof Error ? err.message : "Unable to load";
      if (roleRef.current === "slave" && retriesRef.current < MAX_RETRIES) {
        retriesRef.current++;
        const delay = 1000 * Math.pow(2, retriesRef.current - 1);
        setError(`${msg} — retrying (${retriesRef.current}/${MAX_RETRIES})...`);
        await new Promise<void>((r) => setTimeout(r, delay));
        if (!ac.signal.aborted && abortRef.current === ac) {
          try { await loadReq(next); retriesRef.current = 0; }
          catch (re) { setError(`${re instanceof Error ? re.message : msg}. Try requesting resend.`); }
        }
      } else {
        setError(roleRef.current === "slave" ? `${msg}. Try requesting resend.` : msg);
      }
    } finally {
      loadingRef.current = false;
      abortRef.current = null;
      setIsLoading(false);
      processingRef.current = false;
      // H2 fix: check for pending work synchronously in finally block
      if (pendingRef.current && !ac.signal.aborted && verRef.current === loadVer) {
        void processQueue();
      }
    }
  }, [loadReq, videoRef]);

  const loadTorrent = useCallback((req: LoadRequest) => {
    pendingRef.current = req;
    if (currentView !== "room" || !readyRef.current || !videoRef.current) return;
    if (!processingRef.current) void processQueue();
  }, [currentView, processQueue, videoRef]);

  const loadTorrentFile = useCallback((file: File) => {
    if (roleRef.current !== "master") return;
    if (file.size > MAX_TORRENT_FILE_BYTES) {
      setError(`File too large (${(file.size / 1024 / 1024).toFixed(1)} MB)`);
      return;
    }
    void (async () => {
      try {
        const bytes = new Uint8Array(await file.arrayBuffer());
        loadTorrent({
          source: createTorrentFileSource(file.name, bytes),
          selectedMediaIndex: null, selectedAudioTrackIndex: null, selectedSubtitleIndex: null,
          autoplay: true, broadcast: true,
        });
      } catch (e) { uiLogger.error("File load:", e); }
    })();
  }, [loadTorrent]);

  const resetTorrent = useCallback(async () => {
    if (roleRef.current !== "master") return;
    abortRef.current?.abort(); abortRef.current = null;
    const old = svcRef.current;
    svcRef.current = null;
    setMediaFile(null); setTorrentSource(null);
    setIsLoading(false); setProgress(0); setSpeed("0 B/s"); setError(null);
    setFiles([]); setMediaIndex(null); setMediaLabel(null); setMediaKind(null);
    setMediaAudioTracks([]); setAudioTrackIndex(null); setSubtitleIndex(null);
    setCtxSubtitles([]); setPeerCount(0); setLost(false); setNotice(null);
    setTorrentSource(null); setPendingSync(null);
    pendingRef.current = null; processingRef.current = false; loadingRef.current = false;
    if (old) await old.destroy().catch((e) => uiLogger.warn("Cleanup:", e));
  }, [setMediaFile, setTorrentSource, setMediaIndex, setMediaLabel, setMediaKind, setMediaAudioTracks, setAudioTrackIndex, setSubtitleIndex, setCtxSubtitles, setPendingSync]);

  return {
    isLoadingTorrent: isLoading, torrentProgress: progress, downloadSpeed: speed,
    torrentError: error, torrentPeerCount: peerCount, trackerLost: lost,
    playbackNotice: notice, mediaFiles: files,
    getTorrentService: svc, loadTorrent, loadTorrentFile, resetTorrent,
  };
}
