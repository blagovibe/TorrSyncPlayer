const http = require("node:http");
const { spawn } = require("node:child_process");

const VIDEO_EXTENSIONS = new Set([
  ".mp4", ".mkv", ".webm", ".mov", ".avi", ".m4v", ".ts", ".ogv",
]);

const AUDIO_EXTENSIONS = new Set([
  ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus", ".wav", ".oga", ".wma",
]);
const MAX_TORRENT_CONNECTIONS = 200;
const TORRENT_SERVER_HOST = "127.0.0.1";
const TORRENT_SERVER_PORT = 0;
const AUDIO_SERVER_HOST = "127.0.0.1";
const AUDIO_SERVER_PORT = 0;
const AUDIO_SESSION_TTL_MS = 5 * 60 * 1000;

// Allowed hostnames for stream URL validation — only loopback addresses
const ALLOWED_STREAM_HOSTS = new Set(["127.0.0.1", "::1"]);

// Rate limiting for ffmpeg processes
const MAX_CONCURRENT_FFMPEG = 3;
let activeFfmpegCount = 0;
const ffmpegQueue = [];

function runWithFfmpegLimit(fn) {
  return new Promise((resolve, reject) => {
    const task = async () => {
      activeFfmpegCount++;
      try {
        const result = await fn();
        resolve(result);
      } catch (error) {
        reject(error);
      } finally {
        activeFfmpegCount--;
        if (ffmpegQueue.length > 0) {
          const next = ffmpegQueue.shift();
          next();
        }
      }
    };

    if (activeFfmpegCount < MAX_CONCURRENT_FFMPEG) {
      task();
    } else {
      ffmpegQueue.push(task);
    }
  });
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

function formatTorrentFile(file, index, streamBaseUrl) {
  const extension = getFileExtension(file.name);
  const kind = getMediaKind(extension);
  if (!kind) {
    return null;
  }

  const streamPath = typeof file.streamURL === "string" ? file.streamURL : undefined;

  return {
    index,
    name: file.name,
    length: file.length ?? 0,
    kind,
    extension,
    progress: typeof file.progress === "number" ? file.progress : 0,
    streamUrl: streamBaseUrl && streamPath ? new URL(streamPath, streamBaseUrl).href : streamPath,
  };
}

function formatTorrentSnapshot(torrent, discoveredPeerCount = 0, streamBaseUrl) {
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

class TorrentBridge {
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
    this.audioSessions = new Map();
    this.audioSessionCounter = 0;
  }

  async addMagnet(magnetLink) {
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
      torrent.destroy(() => resolve());
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
      this.audioServer.close();
      this.audioServer = null;
    }
    this.audioServerPromise = null;
    this.audioServerBaseUrl = null;
    this.client = null;
    this.clientPromise = null;

    // Close the torrent stream server if it was created
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
        addedTorrent = client.add(torrentSource, (readyTorrent) => {
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
      console.error("Audio track probe failed:", error);
      return [];
    }
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
    const token = `${Date.now().toString(36)}-${(++this.audioSessionCounter).toString(36)}`;
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

    const audioServerBaseUrl = await this.ensureAudioServer();
    const token = `${Date.now().toString(36)}-${(++this.audioSessionCounter).toString(36)}`;
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
      const server = client.createServer({ origin: "*" });

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

    if (this.audioServerPromise) {
      return this.audioServerPromise;
    }

    this.audioServerPromise = new Promise((resolve, reject) => {
      const server = http.createServer((request, response) => {
        void this.handleAudioRequest(request, response);
      });

      server.listen(AUDIO_SERVER_PORT, AUDIO_SERVER_HOST, () => {
        const address = server.address();
        if (!address || typeof address === "string") {
          reject(new Error("Unable to determine audio server address"));
          return;
        }

        this.audioServer = server;
        this.audioServerBaseUrl = `http://${AUDIO_SERVER_HOST}:${address.port}`;
        resolve(this.audioServerBaseUrl);
      });

      server.on("error", reject);
    });

    return this.audioServerPromise;
  }

  async handleAudioRequest(request, response) {
    try {
      if (!request.url) {
        response.statusCode = 400;
        response.end();
        return;
      }

      if (request.method !== "GET") {
        response.statusCode = 405;
        response.setHeader("Allow", "GET");
        response.end();
        return;
      }

      const { pathname } = new URL(request.url, "http://127.0.0.1");
      if (!pathname.startsWith("/audio/") && !pathname.startsWith("/mux/")) {
        response.statusCode = 404;
        response.end();
        return;
      }

      const isMux = pathname.startsWith("/mux/");
      const token = pathname.slice(isMux ? "/mux/".length : "/audio/".length);
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

      const ffmpegArgs = isMux
        ? [
            // Multiplexed stream: video + audio in one container (WebM for browser compatibility)
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
        : [
            // Audio-only stream (legacy fallback)
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

      const ffmpeg = spawn("ffmpeg", ffmpegArgs);
      session.process = ffmpeg;

      response.statusCode = 200;
      response.setHeader("Content-Type", isMux ? "video/webm" : "audio/mpeg");
      response.setHeader("Cache-Control", "no-store");
      response.setHeader("Pragma", "no-cache");
      response.setHeader("Accept-Ranges", "none");
      response.setHeader("Access-Control-Allow-Origin", "*");
      response.setHeader("Access-Control-Allow-Methods", "GET");
      response.flushHeaders?.();

      ffmpeg.stdout.pipe(response);

      const killProcess = () => {
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
          response.end(error instanceof Error ? error.message : "Audio stream failed");
          return;
        }
        response.destroy(error instanceof Error ? error : new Error("Audio stream failed"));
      });

      ffmpeg.stderr.on("data", (chunk) => {
        const message = String(chunk).trim();
        if (message && process.env.NODE_ENV !== "test") {
          console.warn("[ffmpeg] %s", message);
        }
      });

      response.on("close", killProcess);
      response.on("finish", killProcess);
      ffmpeg.on("close", () => {
        session.process = null;
        session.cleanupTimer = setTimeout(() => {
          this.audioSessions.delete(token);
        }, AUDIO_SESSION_TTL_MS);
        session.cleanupTimer.unref?.();
        response.removeListener("close", killProcess);
        response.removeListener("finish", killProcess);
      });
    } catch (error) {
      // Clean up the session on any error (e.g. spawn throws because ffmpeg is not installed)
      this.audioSessions.delete(token);
      response.statusCode = 500;
      response.end(error instanceof Error ? error.message : "Audio stream failed");
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

      ffprobe.stdout.setEncoding("utf8");
      ffprobe.stdout.on("data", (chunk) => {
        stdout += chunk;
      });
      ffprobe.stderr.setEncoding("utf8");
      ffprobe.stderr.on("data", (chunk) => {
        stderr += chunk;
      });

      ffprobe.on("error", (error) => {
        reject(createErrorFromSpawn(error, "ffprobe failed"));
      });

      ffprobe.on("close", (code) => {
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
        } catch {
          // Process may have already exited
        }
      }
    }
    this.audioSessions.clear();
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

module.exports = { TorrentBridge, formatTorrentSnapshot, checkFfmpegAvailable };
