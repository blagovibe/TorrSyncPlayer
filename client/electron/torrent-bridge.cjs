const { spawn } = require("node:child_process");
const { randomBytes } = require("node:crypto");
const path = require("node:path");
const { electronLogger } = require("./electron-logger.cjs");
const MemoryChunkStore = require("memory-chunk-store");
const { BoundedChunkStore } = require("./bounded-chunk-store.cjs");

const {
  MAX_TORRENT_CONNECTIONS,
  AUDIO_SESSION_TTL_MS,
  SUBTITLE_SESSION_TTL_MS,
  VIDEO_EXTENSIONS,
  AUDIO_EXTENSIONS,
  MAX_TORRENT_FILE_COUNT,
  MAX_TORRENT_FILENAME_LENGTH,
} = require("./torrent-constants.cjs");

const DEFAULT_MAX_BUFFER_MB = 500;

function generateSessionToken() {
  return randomBytes(16).toString("hex");
}

const TORRENT_SERVER_HOST = "127.0.0.1";
const TORRENT_SERVER_PORT = 0;

// Allowed hostnames for stream URL validation — only loopback addresses
const ALLOWED_STREAM_HOSTS = new Set(["127.0.0.1", "::1"]);

// Rate limiting for ffmpeg processes
const MAX_CONCURRENT_FFMPEG = 3;
const MAX_FFMPEG_QUEUE_SIZE = 20;
let activeFfmpegCount = 0;
const ffmpegQueue = [];

function runWithFfmpegLimit(fn) {
  return new Promise((resolve, reject) => {
    let settled = false;
    const settleResolve = (value) => {
      if (settled) return;
      settled = true;
      resolve(value);
    };
    const settleReject = (error) => {
      if (settled) return;
      settled = true;
      reject(error);
    };
    const task = async () => {
      activeFfmpegCount++;
      try {
        const result = await fn();
        settleResolve(result);
      } catch (error) {
        settleReject(error);
      } finally {
        activeFfmpegCount--;
        processQueue();
      }
    };

    const entry = { task, reject: settleReject };

    if (activeFfmpegCount < MAX_CONCURRENT_FFMPEG) {
      task();
    } else if (ffmpegQueue.length >= MAX_FFMPEG_QUEUE_SIZE) {
      settleReject(new Error("ffmpeg queue is full — too many concurrent stream requests"));
    } else {
      ffmpegQueue.push(entry);
    }
  });
}

function processQueue() {
  while (ffmpegQueue.length > 0 && activeFfmpegCount < MAX_CONCURRENT_FFMPEG) {
    const next = ffmpegQueue.shift();
    next.task();
  }
}

// Check ffmpeg availability on startup
let ffmpegAvailable = false;
let ffmpegChecked = false;

async function checkFfmpegAvailable() {
  if (ffmpegChecked) return ffmpegAvailable;
  ffmpegChecked = true;
  return new Promise((resolve) => {
    const proc = spawn("ffmpeg", ["-version"], { timeout: 5000 });
    proc.on("error", () => { ffmpegAvailable = false; resolve(false); });
    proc.on("close", (code) => { ffmpegAvailable = code === 0; resolve(ffmpegAvailable); });
    proc.on("timeout", () => { proc.kill(); ffmpegAvailable = false; resolve(false); });
  });
}

function normalizePeerId(peerId) {
  if (peerId == null) {
    return null;
  }

  const normalized = String(peerId).trim();
  return normalized.length > 0 ? normalized : null;
}

function getFileExtension(name) {
  const normalized = String(name || "").trim().toLowerCase();
  const lastDot = normalized.lastIndexOf(".");
  if (lastDot === -1) {
    return "";
  }
  return normalized.slice(lastDot);
}

function getMediaKind(extension) {
  if (VIDEO_EXTENSIONS.has(extension)) {
    return "video";
  }
  if (AUDIO_EXTENSIONS.has(extension)) {
    return "audio";
  }
  return null;
}

function validateTorrentFiles(files) {
  if (!Array.isArray(files)) return;
  if (files.length > MAX_TORRENT_FILE_COUNT) {
    throw new Error(`Torrent contains too many files (${files.length}). Maximum allowed is ${MAX_TORRENT_FILE_COUNT}.`);
  }
  for (const file of files) {
    if (typeof file?.name === "string" && file.name.length > MAX_TORRENT_FILENAME_LENGTH) {
      throw new Error(`Torrent file name exceeds maximum length (${file.name.length} > ${MAX_TORRENT_FILENAME_LENGTH}): ${file.name.slice(0, 64)}…`);
    }
  }
}

function formatTorrentFile(file, index, streamBaseUrl) {
  const extension = getFileExtension(file.name);
  const kind = getMediaKind(extension);
  if (!kind) {
    return null;
  }

  const streamPath = typeof file.streamURL === "string" ? file.streamURL : undefined;

  let streamUrl;
  if (streamBaseUrl && streamPath) {
    try {
      streamUrl = new URL(streamPath, streamBaseUrl).href;
    } catch {
      streamUrl = streamPath;
    }
  } else {
    streamUrl = streamPath;
  }

  return {
    index,
    name: file.name,
    length: file.length ?? 0,
    kind,
    extension,
    progress: typeof file.progress === "number" ? file.progress : 0,
    streamUrl: streamUrl,
  };
}

function formatTorrentSnapshot(torrent, discoveredPeerCount = 0, streamBaseUrl) {
  validateTorrentFiles(torrent.files);
  const files = torrent.files
    .map((file, index) => formatTorrentFile(file, index, streamBaseUrl))
    .filter(Boolean)
    .sort((left, right) => {
      if (left.kind !== right.kind) {
        return left.kind === "video" ? -1 : 1;
      }
      if (right.length !== left.length) {
        return right.length - left.length;
      }
      return left.name.localeCompare(right.name);
    });

  return {
    files,
    progress: torrent.progress ?? 0,
    downloadSpeed: torrent.downloadSpeed ?? 0,
    numPeers: torrent.numPeers ?? 0,
    discoveredPeerCount,
  };
}

function normalizeTorrentSource(torrentSource) {
  if (torrentSource instanceof Uint8Array) {
    return torrentSource;
  }
  if (Array.isArray(torrentSource)) {
    return Uint8Array.from(torrentSource);
  }
  throw new Error("Invalid torrent source type: expected Uint8Array or Array");
}

function createErrorFromSpawn(error, fallbackMessage) {
  if (error instanceof Error) {
    return error;
  }
  return new Error(fallbackMessage);
}

function parseProbeResult(stdout) {
  let parsed;
  try {
    parsed = JSON.parse(stdout);
  } catch {
    return [];
  }

  const streams = Array.isArray(parsed?.streams) ? parsed.streams : [];
  return streams
    .map((stream, index) => {
      const tags = stream?.tags ?? {};
      const codecName = typeof stream?.codec_name === "string" ? stream.codec_name : "";
      const codecLongName = typeof stream?.codec_long_name === "string" ? stream.codec_long_name : "";
      const title = typeof tags.title === "string" ? tags.title.trim() : "";
      const language = typeof tags.language === "string" ? tags.language.trim() : "";
      const channels = Number.isInteger(stream?.channels) ? stream.channels : null;
      const sampleRate = Number.isInteger(Number(stream?.sample_rate)) ? Number(stream.sample_rate) : null;
      const label = title || codecLongName || codecName || `Audio ${index + 1}`;

      return {
        index,
        label,
        language,
        codecName,
        channels,
        sampleRate,
      };
    })
    .filter((stream) => stream.codecName || stream.label);
}

function parseSubtitleResult(stdout) {
  let parsed;
  try {
    parsed = JSON.parse(stdout);
  } catch {
    return [];
  }

  const streams = Array.isArray(parsed?.streams) ? parsed.streams : [];
  return streams
    .map((stream, index) => {
      const tags = stream?.tags ?? {};
      const codecName = typeof stream?.codec_name === "string" ? stream.codec_name : "";
      const title = typeof tags.title === "string" ? tags.title.trim() : "";
      const language = typeof tags.language === "string" ? tags.language.trim() : "";
      const forced = tags.forced === "1" || tags.forced === 1;
      const defaultTrack = tags.default === "1" || tags.default === 1;
      const label = title || language || codecName || `Subtitle ${index + 1}`;

      return {
        index,
        label,
        language,
        codecName,
        forced,
        default: defaultTrack,
      };
    })
    .filter((stream) => stream.codecName || stream.label);
}

class TorrentBridge {
  static async checkFfmpegAvailable() {
    return checkFfmpegAvailable();
  }

  constructor() {
    this.clientPromise = null;
    this.client = null;
    this.activeTorrent = null;
    this.discoveredPeerIds = new Set();
    this.streamBaseUrl = null;
    this.serverPromise = null;
    this.audioServer = null;
    this.audioServerPromise = null;
    this.audioServerBaseUrl = null;
    this._ensureAudioServerTimeout = null;
    this.audioSessions = new Map();

    this.maxBufferBytes = DEFAULT_MAX_BUFFER_MB * 1024 * 1024;
  }

  setMaxBufferMB(mb) {
    this.maxBufferBytes = Math.max(1, mb) * 1024 * 1024;
  }

  setStreamBaseUrl(url) {
    this.audioServerBaseUrl = url;
    if (this._resolveAudioServerWaiter) {
      this._resolveAudioServerWaiter(url);
      this._resolveAudioServerWaiter = null;
      this._ensureAudioServerWaiter = null;
    }
  }

  async addMagnet(magnetLink) {
    const { isValidMagnetLink, MAX_MAGNET_LINK_LENGTH: MAX_MAGNET_LEN } = require("./torrent-constants.cjs");
    if (typeof magnetLink !== "string" || magnetLink.length > MAX_MAGNET_LEN) {
      throw new Error("Invalid magnet link: too long or not a string");
    }
    if (!isValidMagnetLink(magnetLink)) {
      throw new Error("Invalid magnet link format");
    }
    const { validateMagnetTrackerUrls } = require("./torrent-constants.cjs");
    validateMagnetTrackerUrls(magnetLink);
    return this.addSource(magnetLink);
  }

  async addTorrentFile(torrentFile) {
    return this.addSource(normalizeTorrentSource(torrentFile));
  }

  async getStats() {
    if (!this.activeTorrent) {
      return null;
    }

    return formatTorrentSnapshot(this.activeTorrent, this.discoveredPeerIds.size, this.streamBaseUrl);
  }

  async clear() {
    const torrent = this.activeTorrent;
    this.activeTorrent = null;
    this.discoveredPeerIds.clear();
    this.clearAudioSessions();

    if (!torrent || typeof torrent.destroy !== "function") {
      return;
    }

    await new Promise((resolve) => {
      const timeout = setTimeout(() => {
        resolve();
      }, 10_000);
      timeout.unref?.();

      try {
        torrent.destroy(() => {
          clearTimeout(timeout);
          resolve();
        });
      } catch {
        clearTimeout(timeout);
        resolve();
      }
    });
  }

  async destroy() {
    const client = this.client;

    this.activeTorrent = null;
    this.discoveredPeerIds.clear();
    this.streamBaseUrl = null;
    this.serverPromise = null;
    this.clearAudioSessions();
    if (this.audioServer) {
      try {
        this.audioServer.close();
      } catch {
        // Server may have already been closed
      }
      this.audioServer = null;
    }
    this.audioServerPromise = null;
    this.audioServerBaseUrl = null;
    if (this._ensureAudioServerTimeout) {
      clearTimeout(this._ensureAudioServerTimeout);
      this._ensureAudioServerTimeout = null;
    }
    if (this._resolveAudioServerWaiter) {
      this._resolveAudioServerWaiter(null);
      this._resolveAudioServerWaiter = null;
    }
    this._ensureAudioServerWaiter = null;
    this.client = null;
    this.clientPromise = null;

    if (client && !client.destroyed && typeof client._server === "object" && client._server !== null) {
      const server = client._server;
      if (typeof server.close === "function") {
        try {
          server.close();
        } catch {
          // Server may have already been closed
        }
      }
    }

    if (!client || client.destroyed || typeof client.destroy !== "function") {
      return;
    }

    await new Promise((resolve) => {
      client.destroy(() => resolve());
    });
  }

  async addSource(torrentSource) {
    const client = await this.getClient();
    await this.ensureTorrentServer(client);
    await this.clear();

    const MAX_PEER_IDS = 50_000;

    const torrent = await new Promise((resolve, reject) => {
      let settled = false;
      const settleResolve = (value) => {
        if (settled) return;
        settled = true;
        clearTimeout(addTimeout);
        resolve(value);
      };
      const settleReject = (error) => {
        if (settled) return;
        settled = true;
        clearTimeout(addTimeout);
        reject(error);
      };

      let addedTorrent;
      try {
        const storeOpts = {
          maxBytes: this.maxBufferBytes,
        };
        addedTorrent = client.add(torrentSource, { store: BoundedChunkStore, storeOpts }, (readyTorrent) => {
          settleResolve(readyTorrent);
        });
      } catch (error) {
        settleReject(error instanceof Error ? error : new Error(String(error)));
        return;
      }

      if (!addedTorrent) {
        settleReject(new Error("Failed to add torrent: client.add returned null"));
        return;
      }

      const addTimeout = setTimeout(() => {
        settleReject(new Error("Torrent addition timed out after 60 seconds"));
      }, 60_000);

      addedTorrent.on("peer", (peerId) => {
        const normalizedPeerId = normalizePeerId(peerId);
        if (normalizedPeerId && this.discoveredPeerIds.size < MAX_PEER_IDS) {
          this.discoveredPeerIds.add(normalizedPeerId);
        }
      });

      addedTorrent.on("error", (error) => {
        settleReject(error instanceof Error ? error : new Error(String(error)));
      });
    });
    this.activeTorrent = torrent;
    return formatTorrentSnapshot(torrent, this.discoveredPeerIds.size, this.streamBaseUrl);
  }

  async probeAudioTracks(streamUrl) {
    if (!streamUrl) {
      return [];
    }

    this.validateLocalStreamUrl(streamUrl);

    try {
      const result = await this.runFfprobe(streamUrl);
      return parseProbeResult(result.stdout);
    } catch (error) {
      electronLogger.error("Audio track probe failed:", error);
      return [];
    }
  }

  async probeSubtitles(streamUrl) {
    if (!streamUrl) {
      return [];
    }

    this.validateLocalStreamUrl(streamUrl);

    try {
      const result = await this.runFfprobeSubtitles(streamUrl);
      return parseSubtitleResult(result.stdout);
    } catch (error) {
      electronLogger.error("Subtitle probe failed:", error);
      return [];
    }
  }

  async createSubtitleStreamUrl({ streamUrl, trackIndex, startSeconds }) {
    if (!streamUrl) {
      throw new Error("Subtitle stream URL is unavailable");
    }

    this.validateLocalStreamUrl(streamUrl);

    if (typeof trackIndex !== "number" || !Number.isInteger(trackIndex) || trackIndex < 0) {
      throw new Error("Invalid track index");
    }

    if (typeof startSeconds !== "number" || !Number.isFinite(startSeconds) || startSeconds < 0 || startSeconds > 86400) {
      throw new Error("Invalid start seconds");
    }

    const audioServerBaseUrl = await this.ensureAudioServer();
    const token = generateSessionToken();
    const session = {
      streamUrl,
      trackIndex,
      kind: "subtitle",
      startSeconds: Math.max(0, startSeconds),
      process: null,
      cleanupTimer: setTimeout(() => {
        this.audioSessions.delete(token);
      }, SUBTITLE_SESSION_TTL_MS),
    };
    session.cleanupTimer.unref?.();
    this.audioSessions.set(token, session);
    return `${audioServerBaseUrl}/subtitle/${token}`;
  }

  async createAudioTrackStreamUrl({ streamUrl, trackIndex, startSeconds }) {
    if (!streamUrl) {
      throw new Error("Audio stream URL is unavailable");
    }

    this.validateLocalStreamUrl(streamUrl);

    if (typeof trackIndex !== "number" || !Number.isInteger(trackIndex) || trackIndex < 0) {
      throw new Error("Invalid track index");
    }

    if (typeof startSeconds !== "number" || !Number.isFinite(startSeconds) || startSeconds < 0 || startSeconds > 86400) {
      throw new Error("Invalid start seconds");
    }

    const audioServerBaseUrl = await this.ensureAudioServer();
    const token = generateSessionToken();
    const session = {
      streamUrl,
      trackIndex,
      startSeconds: Math.max(0, startSeconds),
      process: null,
      cleanupTimer: setTimeout(() => {
        this.audioSessions.delete(token);
      }, AUDIO_SESSION_TTL_MS),
    };
    session.cleanupTimer.unref?.();
    this.audioSessions.set(token, session);
    return `${audioServerBaseUrl}/audio/${token}`;
  }

  // Create a multiplexed audio+video stream for a single <video> element.
  // This ensures perfect audio/video sync since both are in the same container.
  async createMultiplexedStreamUrl({ streamUrl, audioTrackIndex, startSeconds }) {
    if (!streamUrl) {
      throw new Error("Stream URL is unavailable");
    }

    this.validateLocalStreamUrl(streamUrl);

    if (typeof startSeconds !== "number" || !Number.isFinite(startSeconds) || startSeconds < 0 || startSeconds > 86400) {
      throw new Error("Invalid start seconds");
    }

    if (audioTrackIndex !== undefined && audioTrackIndex !== null && (typeof audioTrackIndex !== "number" || !Number.isInteger(audioTrackIndex) || audioTrackIndex < 0)) {
      throw new Error("Invalid audio track index");
    }

    const audioServerBaseUrl = await this.ensureAudioServer();
    const token = generateSessionToken();
    const session = {
      streamUrl,
      audioTrackIndex: audioTrackIndex ?? 0,
      startSeconds: Math.max(0, startSeconds),
      process: null,
      cleanupTimer: setTimeout(() => {
        this.audioSessions.delete(token);
      }, AUDIO_SESSION_TTL_MS),
    };
    session.cleanupTimer.unref?.();
    this.audioSessions.set(token, session);
    return `${audioServerBaseUrl}/mux/${token}`;
  }

  async getClient() {
    if (this.client) {
      return this.client;
    }

    if (!this.clientPromise) {
    this.clientPromise = import("webtorrent").then(({ default: WebTorrent }) => {
      this.client = new WebTorrent({
        maxConns: MAX_TORRENT_CONNECTIONS,
        sequential: true,
      });
      return this.client;
    });
    }

    return this.clientPromise;
  }

  async ensureTorrentServer(client) {
    if (this.streamBaseUrl) {
      return this.streamBaseUrl;
    }

    if (this.serverPromise) {
      return this.serverPromise;
    }

    const existingServer = client?._server;
    if (existingServer && typeof existingServer.address === "function") {
      const address = existingServer.address();
      if (address && typeof address !== "string") {
        this.streamBaseUrl = `http://${TORRENT_SERVER_HOST}:${address.port}`;
        return this.streamBaseUrl;
      }
    }

    if (!client || typeof client.createServer !== "function") {
      throw new Error("WebTorrent client server API is unavailable");
    }

    this.serverPromise = (async () => {
      const server = client.createServer();

      await new Promise((resolve, reject) => {
        const underlyingServer = server?.server;

        const handleError = (error) => {
          if (underlyingServer && typeof underlyingServer.removeListener === "function") {
            underlyingServer.removeListener("error", handleError);
          }
          reject(error instanceof Error ? error : new Error(String(error)));
        };

        if (underlyingServer && typeof underlyingServer.once === "function") {
          underlyingServer.once("error", handleError);
        }

        try {
          server.listen(TORRENT_SERVER_PORT, TORRENT_SERVER_HOST, () => {
            if (underlyingServer && typeof underlyingServer.removeListener === "function") {
              underlyingServer.removeListener("error", handleError);
            }
            resolve(undefined);
          });
        } catch (error) {
          if (underlyingServer && typeof underlyingServer.removeListener === "function") {
            underlyingServer.removeListener("error", handleError);
          }
          handleError(error);
        }
      });

      const address = server.address();
      if (!address || typeof address === "string") {
        throw new Error("Unable to determine torrent server address");
      }

      this.streamBaseUrl = `http://${TORRENT_SERVER_HOST}:${address.port}`;
      return this.streamBaseUrl;
    })();

    return this.serverPromise;
  }

  async ensureAudioServer() {
    if (this.audioServerBaseUrl) {
      return this.audioServerBaseUrl;
    }

    if (!this._ensureAudioServerWaiter) {
      this._ensureAudioServerWaiter = new Promise((resolve) => {
        this._resolveAudioServerWaiter = resolve;
        this._ensureAudioServerTimeout = setTimeout(() => {
          if (this._resolveAudioServerWaiter) {
            this._resolveAudioServerWaiter(null);
            this._resolveAudioServerWaiter = null;
            this._ensureAudioServerWaiter = null;
          }
        }, 10_000);
      });
    }
    const result = await this._ensureAudioServerWaiter;
    if (this._ensureAudioServerTimeout) {
      clearTimeout(this._ensureAudioServerTimeout);
      this._ensureAudioServerTimeout = null;
    }
    if (!result) {
      this._ensureAudioServerWaiter = null;
      throw new Error("Audio server is unavailable — timed out waiting for stream base URL");
    }
    return result;
  }

  async handleAudioRequest(request, response) {
    if (!request.url) {
      response.statusCode = 400;
      response.end();
      return;
    }

    // Handle CORS preflight (OPTIONS) for cross-origin media requests
    if (request.method === "OPTIONS") {
      const origin = request.headers.origin;
      const allowedOrigin = (origin && origin.startsWith("http://127.0.0.1")) ? origin : (this.streamBaseUrl || "http://127.0.0.1");
      response.statusCode = 204;
      response.setHeader("Access-Control-Allow-Origin", allowedOrigin);
      response.setHeader("Access-Control-Allow-Methods", "GET, OPTIONS");
      response.setHeader("Access-Control-Allow-Headers", "Range, Origin");
      response.setHeader("Access-Control-Max-Age", "86400");
      response.setHeader("Access-Control-Allow-Private-Network", "true");
      response.end();
      return;
    }

    if (request.method !== "GET") {
      response.statusCode = 405;
      response.setHeader("Allow", "GET, OPTIONS");
      response.end();
      return;
    }

    const { pathname } = new URL(request.url, "http://127.0.0.1");
    const isSubtitle = pathname.startsWith("/subtitle/");
    if (!pathname.startsWith("/audio/") && !pathname.startsWith("/mux/") && !isSubtitle) {
      response.statusCode = 404;
      response.end();
      return;
    }

    const isMux = pathname.startsWith("/mux/");
    const token = pathname.slice(isSubtitle ? "/subtitle/".length : isMux ? "/mux/".length : "/audio/".length);
    const session = this.audioSessions.get(token);

    if (!session) {
      response.statusCode = 404;
      response.end();
      return;
    }

    if (session.cleanupTimer) {
      clearTimeout(session.cleanupTimer);
      session.cleanupTimer = null;
    }

    try {
      this.validateLocalStreamUrl(session.streamUrl);
      const ffmpegArgs = isMux
        ? [
            "-hide_banner",
            "-loglevel",
            "error",
            "-nostdin",
            "-ss",
            String(session.startSeconds ?? 0),
            "-i",
            session.streamUrl,
            "-map",
            "0:v:0",
            "-map",
            `0:a:${session.audioTrackIndex ?? 0}`,
            "-c:v",
            "libvpx-vp9",
            "-deadline",
            "realtime",
            "-cpu-used",
            "5",
            "-crf",
            "30",
            "-b:v",
            "0",
            "-c:a",
            "libopus",
            "-b:a",
            "128k",
            "-f",
            "webm",
            "pipe:1",
          ]
        : isSubtitle
          ? [
              "-hide_banner",
              "-loglevel",
              "error",
              "-nostdin",
              "-ss",
              String(session.startSeconds ?? 0),
              "-i",
              session.streamUrl,
              "-map",
              `0:s:${session.trackIndex}`,
              "-f",
              "webvtt",
              "pipe:1",
            ]
          : [
              "-hide_banner",
              "-loglevel",
              "error",
              "-nostdin",
              "-ss",
              String(session.startSeconds ?? 0),
              "-i",
              session.streamUrl,
              "-map",
              `0:a:${session.trackIndex}`,
              "-vn",
              "-sn",
              "-dn",
              "-c:a",
              "libmp3lame",
              "-q:a",
              "4",
              "-f",
              "mp3",
              "pipe:1",
            ];

      // Rate limit ffmpeg processes to prevent resource exhaustion
      const spawnFfmpeg = () => spawn("ffmpeg", ffmpegArgs);

      if (activeFfmpegCount < MAX_CONCURRENT_FFMPEG) {
        activeFfmpegCount++;
        session.process = spawnFfmpeg();
      } else {
        await new Promise((resolve, reject) => {
          ffmpegQueue.push({
            task: () => {
              if (!this.audioSessions.has(token)) {
                activeFfmpegCount--;
                reject(new Error("Audio session was cleaned up before ffmpeg could start"));
                processQueue();
                return;
              }
              activeFfmpegCount++;
              try {
                session.process = spawnFfmpeg();
              } catch (spawnError) {
                activeFfmpegCount--;
                session.process = null;
                processQueue();
                reject(spawnError);
                return;
              }
              resolve();
            },
            reject,
          });
        });
      }
      const ffmpeg = session.process;

      response.statusCode = 200;
      response.setHeader("Content-Type", isSubtitle ? "text/vtt" : isMux ? "video/webm" : "audio/mpeg");
      response.setHeader("Cache-Control", "no-store");
      response.setHeader("Pragma", "no-cache");
      response.setHeader("Accept-Ranges", "none");
      response.setHeader("Access-Control-Allow-Origin", this.streamBaseUrl || "http://127.0.0.1");
      response.setHeader("Access-Control-Allow-Methods", "GET, OPTIONS");
      response.setHeader("Access-Control-Allow-Private-Network", "true");
      response.flushHeaders?.();

      ffmpeg.stdout.pipe(response);

      let processKilled = false;
      const killProcess = () => {
        if (processKilled) return;
        processKilled = true;
        try {
          if (!ffmpeg.killed) {
            ffmpeg.kill("SIGKILL");
          }
        } catch {
          // Process may have already exited
        }
      };

      ffmpeg.on("error", (error) => {
        this.audioSessions.delete(token);
        killProcess();
        if (!response.headersSent) {
          response.statusCode = 500;
          response.end(error instanceof Error ? error.message : "Stream failed");
        } else {
          response.destroy(error instanceof Error ? error : new Error("Stream failed"));
        }
      });

      ffmpeg.stderr.on("data", (chunk) => {
        const message = String(chunk).trim();
        if (message && !process.env.VITEST && process.env.NODE_ENV !== "test") {
          electronLogger.warn(`ffmpeg: ${message}`);
        }
      });

      response.on("close", killProcess);
      response.on("finish", killProcess);
      ffmpeg.on("close", () => {
        if (session.process !== ffmpeg) return;
        response.removeListener("close", killProcess);
        response.removeListener("finish", killProcess);
        this.audioSessions.delete(token);
        activeFfmpegCount = Math.max(0, activeFfmpegCount - 1);
        processQueue();
        session.process = null;
        session.cleanupTimer = setTimeout(() => {
          this.audioSessions.delete(token);
        }, AUDIO_SESSION_TTL_MS);
        session.cleanupTimer.unref?.();
      });
    } catch (error) {
      this.audioSessions.delete(token);
      if (!response.headersSent) {
        response.statusCode = 500;
        response.end(error instanceof Error ? error.message : "Stream failed");
      } else {
        response.destroy();
      }
    }
  }

  async runFfprobe(streamUrl) {
    return new Promise((resolve, reject) => {
      const ffprobe = spawn("ffprobe", [
        "-v",
        "error",
        "-select_streams",
        "a",
        "-show_entries",
        "stream=index,codec_name,codec_long_name,channels,sample_rate:stream_tags=language,title",
        "-of",
        "json",
        streamUrl,
      ]);

      let stdout = "";
      let stderr = "";
      let timedOut = false;

      const timeout = setTimeout(() => {
        timedOut = true;
        ffprobe.kill("SIGKILL");
        reject(new Error("ffprobe timed out after 15s"));
      }, 15_000);
      timeout.unref?.();

      ffprobe.stdout.setEncoding("utf8");
      ffprobe.stdout.on("data", (chunk) => { stdout += chunk; });
      ffprobe.stderr.setEncoding("utf8");
      ffprobe.stderr.on("data", (chunk) => { stderr += chunk; });

      ffprobe.on("error", (error) => {
        clearTimeout(timeout);
        reject(createErrorFromSpawn(error, "ffprobe failed"));
      });

      ffprobe.on("close", (code) => {
        clearTimeout(timeout);
        if (timedOut) return;
        if (code !== 0) {
          reject(new Error(stderr.trim() || `ffprobe exited with code ${code}`));
          return;
        }
        resolve({ stdout, stderr });
      });
    });
  }

  async runFfprobeSubtitles(streamUrl) {
    return new Promise((resolve, reject) => {
      const ffprobe = spawn("ffprobe", [
        "-v",
        "error",
        "-select_streams",
        "s",
        "-show_entries",
        "stream=index,codec_name:stream_tags=language,title,forced,default",
        "-of",
        "json",
        streamUrl,
      ]);

      let stdout = "";
      let stderr = "";
      let timedOut = false;

      const timeout = setTimeout(() => {
        timedOut = true;
        ffprobe.kill("SIGKILL");
        reject(new Error("ffprobe timed out after 15s"));
      }, 15_000);
      timeout.unref?.();

      ffprobe.stdout.setEncoding("utf8");
      ffprobe.stdout.on("data", (chunk) => { stdout += chunk; });
      ffprobe.stderr.setEncoding("utf8");
      ffprobe.stderr.on("data", (chunk) => { stderr += chunk; });

      ffprobe.on("error", (error) => {
        clearTimeout(timeout);
        reject(createErrorFromSpawn(error, "ffprobe failed"));
      });

      ffprobe.on("close", (code) => {
        clearTimeout(timeout);
        if (timedOut) return;
        if (code !== 0) {
          reject(new Error(stderr.trim() || `ffprobe exited with code ${code}`));
          return;
        }
        resolve({ stdout, stderr });
      });
    });
  }

  clearAudioSessions() {
    for (const session of this.audioSessions.values()) {
      if (session?.cleanupTimer) {
        clearTimeout(session.cleanupTimer);
      }
      if (session?.process && !session.process.killed) {
        try {
          session.process.kill("SIGKILL");
          activeFfmpegCount = Math.max(0, activeFfmpegCount - 1);
        } catch {
          // Process may have already exited
        }
      }
    }
    this.audioSessions.clear();
    // Reject all pending ffmpeg queue tasks — the service is shutting down
    const pendingTasks = ffmpegQueue.splice(0);
    for (const pending of pendingTasks) {
      try {
        pending.reject(new Error("Torrent bridge is shutting down"));
      } catch {
        // Ignore
      }
    }
  }

  validateLocalStreamUrl(streamUrl) {
    let parsed;
    try {
      parsed = new URL(streamUrl);
    } catch {
      throw new Error(`Invalid stream URL: ${streamUrl}`);
    }

    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      throw new Error(`Stream URL must use http or https protocol, got: ${parsed.protocol}`);
    }

    const allowedHosts = ALLOWED_STREAM_HOSTS;
    if (!ALLOWED_STREAM_HOSTS.has(parsed.hostname)) {
      throw new Error(`Stream URL must point to a local address, got: ${parsed.hostname}`);
    }
  }
}

module.exports = { TorrentBridge, formatTorrentSnapshot };
