const { spawn } = require("node:child_process");
const { electronLogger } = require("./electron-logger.cjs");
const {
  generateSessionToken,
  runWithFfmpegLimit,
  parseProbeResult,
  parseSubtitleResult,
  runFfprobe,
  getActiveFfmpegCount,
  clearFfmpegQueue,
} = require("./ffmpeg-pipeline.cjs");

const ALLOWED_STREAM_HOSTS = new Set(["127.0.0.1", "::1"]);

function validateLocalStreamUrl(streamUrl) {
  let parsed;
  try {
    parsed = new URL(streamUrl);
  } catch {
    throw new Error(`Invalid stream URL: ${streamUrl}`);
  }

  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new Error(`Stream URL must use http or https protocol, got: ${parsed.protocol}`);
  }

  if (!ALLOWED_STREAM_HOSTS.has(parsed.hostname)) {
    throw new Error(`Stream URL must point to a local address, got: ${parsed.hostname}`);
  }
}

class AudioSessionManager {
  constructor() {
    this.audioSessions = new Map();
    this.audioServerBaseUrl = null;
    this._ensureAudioServerWaiter = null;
    this._resolveAudioServerWaiter = null;
    this._ensureAudioServerTimeout = null;
  }

  get baseUrl() {
    return this.audioServerBaseUrl;
  }

  setBaseUrl(url) {
    this.audioServerBaseUrl = url;
    if (this._resolveAudioServerWaiter) {
      this._resolveAudioServerWaiter(url);
      this._resolveAudioServerWaiter = null;
      this._ensureAudioServerWaiter = null;
    }
  }

  async ensureAudioServer() {
    if (this.audioServerBaseUrl) return this.audioServerBaseUrl;

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

  async probeAudioTracks(streamUrl) {
    if (!streamUrl) return [];
    validateLocalStreamUrl(streamUrl);
    try {
      const result = await runFfprobe(
        streamUrl, "a",
        "stream=index,codec_name,codec_long_name,channels,sample_rate:stream_tags=language,title"
      );
      return parseProbeResult(result.stdout);
    } catch (error) {
      electronLogger.error("Audio track probe failed:", error);
      return [];
    }
  }

  async probeSubtitles(streamUrl) {
    if (!streamUrl) return [];
    validateLocalStreamUrl(streamUrl);
    try {
      const result = await runFfprobe(
        streamUrl, "s",
        "stream=index,codec_name:stream_tags=language,title,forced,default"
      );
      return parseSubtitleResult(result.stdout);
    } catch (error) {
      electronLogger.error("Subtitle probe failed:", error);
      return [];
    }
  }

  async createSubtitleStreamUrl({ streamUrl, trackIndex, startSeconds, ttlMs }) {
    if (!streamUrl) throw new Error("Subtitle stream URL is unavailable");
    validateLocalStreamUrl(streamUrl);

    if (typeof trackIndex !== "number" || !Number.isInteger(trackIndex) || trackIndex < 0) {
      throw new Error("Invalid track index");
    }
    if (typeof startSeconds !== "number" || !Number.isFinite(startSeconds) || startSeconds < 0 || startSeconds > 86400) {
      throw new Error("Invalid start seconds");
    }

    const audioServerBaseUrl = await this.ensureAudioServer();
    const token = generateSessionToken();
    const session = {
      streamUrl, trackIndex, kind: "subtitle",
      startSeconds: Math.max(0, startSeconds),
      process: null,
      cleanupTimer: setTimeout(() => { this.audioSessions.delete(token); }, ttlMs),
    };
    session.cleanupTimer.unref?.();
    this.audioSessions.set(token, session);
    return `${audioServerBaseUrl}/subtitle/${token}`;
  }

  async createAudioTrackStreamUrl({ streamUrl, trackIndex, startSeconds, ttlMs }) {
    if (!streamUrl) throw new Error("Audio stream URL is unavailable");
    validateLocalStreamUrl(streamUrl);

    if (typeof trackIndex !== "number" || !Number.isInteger(trackIndex) || trackIndex < 0) {
      throw new Error("Invalid track index");
    }
    if (typeof startSeconds !== "number" || !Number.isFinite(startSeconds) || startSeconds < 0 || startSeconds > 86400) {
      throw new Error("Invalid start seconds");
    }

    const audioServerBaseUrl = await this.ensureAudioServer();
    const token = generateSessionToken();
    const session = {
      streamUrl, trackIndex,
      startSeconds: Math.max(0, startSeconds),
      process: null,
      cleanupTimer: setTimeout(() => { this.audioSessions.delete(token); }, ttlMs),
    };
    session.cleanupTimer.unref?.();
    this.audioSessions.set(token, session);
    return `${audioServerBaseUrl}/audio/${token}`;
  }

  async createMultiplexedStreamUrl({ streamUrl, audioTrackIndex, startSeconds, ttlMs }) {
    if (!streamUrl) throw new Error("Stream URL is unavailable");
    validateLocalStreamUrl(streamUrl);

    if (typeof startSeconds !== "number" || !Number.isFinite(startSeconds) || startSeconds < 0 || startSeconds > 86400) {
      throw new Error("Invalid start seconds");
    }
    if (audioTrackIndex !== undefined && audioTrackIndex !== null &&
        (typeof audioTrackIndex !== "number" || !Number.isInteger(audioTrackIndex) || audioTrackIndex < 0)) {
      throw new Error("Invalid audio track index");
    }

    const audioServerBaseUrl = await this.ensureAudioServer();
    const token = generateSessionToken();
    const session = {
      streamUrl,
      audioTrackIndex: audioTrackIndex ?? 0,
      startSeconds: Math.max(0, startSeconds),
      process: null,
      cleanupTimer: setTimeout(() => { this.audioSessions.delete(token); }, ttlMs),
    };
    session.cleanupTimer.unref?.();
    this.audioSessions.set(token, session);
    return `${audioServerBaseUrl}/mux/${token}`;
  }

  async handleAudioRequest(request, response, streamBaseUrl) {
    if (!request.url) {
      response.statusCode = 400;
      response.end();
      return;
    }

    if (request.method === "OPTIONS") {
      const origin = request.headers.origin;
      const allowedOrigin = (origin && origin.startsWith("http://127.0.0.1")) ? origin : (streamBaseUrl || "http://127.0.0.1");
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
      validateLocalStreamUrl(session.streamUrl);

      const ffmpegArgs = isMux
        ? [
            "-hide_banner", "-loglevel", "error", "-nostdin",
            "-ss", String(session.startSeconds ?? 0),
            "-i", session.streamUrl,
            "-map", "0:v:0",
            "-map", `0:a:${session.audioTrackIndex ?? 0}`,
            "-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "5", "-crf", "30", "-b:v", "0",
            "-c:a", "libopus", "-b:a", "128k",
            "-f", "webm", "pipe:1",
          ]
        : isSubtitle
          ? [
              "-hide_banner", "-loglevel", "error", "-nostdin",
              "-ss", String(session.startSeconds ?? 0),
              "-i", session.streamUrl,
              "-map", `0:s:${session.trackIndex}`,
              "-f", "webvtt", "pipe:1",
            ]
          : [
              "-hide_banner", "-loglevel", "error", "-nostdin",
              "-ss", String(session.startSeconds ?? 0),
              "-i", session.streamUrl,
              "-map", `0:a:${session.trackIndex}`,
              "-vn", "-sn", "-dn",
              "-c:a", "libmp3lame", "-q:a", "4",
              "-f", "mp3", "pipe:1",
            ];

      const spawnFfmpeg = () => spawn("ffmpeg", ffmpegArgs);

      if (getActiveFfmpegCount() < 3) {
        session.process = spawnFfmpeg();
      } else {
        await new Promise((resolve, reject) => {
          const queueEntry = {
            task: () => {
              if (!this.audioSessions.has(token)) {
                reject(new Error("Audio session was cleaned up before ffmpeg could start"));
                return;
              }
              try {
                session.process = spawnFfmpeg();
              } catch (spawnError) {
                session.process = null;
                reject(spawnError);
                return;
              }
              resolve();
            },
            reject,
          };
          const { runWithFfmpegLimit: limit } = require("./ffmpeg-pipeline.cjs");
          limit(() => queueEntry.task()).then(resolve).catch(reject);
        });
      }

      const ffmpeg = session.process;

      response.statusCode = 200;
      response.setHeader("Content-Type", isSubtitle ? "text/vtt" : isMux ? "video/webm" : "audio/mpeg");
      response.setHeader("Cache-Control", "no-store");
      response.setHeader("Pragma", "no-cache");
      response.setHeader("Accept-Ranges", "none");
      response.setHeader("Access-Control-Allow-Origin", streamBaseUrl || "http://127.0.0.1");
      response.setHeader("Access-Control-Allow-Methods", "GET, OPTIONS");
      response.setHeader("Access-Control-Allow-Private-Network", "true");
      response.flushHeaders?.();

      ffmpeg.stdout.pipe(response);

      let processKilled = false;
      const killProcess = () => {
        if (processKilled) return;
        processKilled = true;
        try {
          if (!ffmpeg.killed) ffmpeg.kill("SIGKILL");
        } catch { /* already exited */ }
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
        session.process = null;
        session.cleanupTimer = setTimeout(() => { this.audioSessions.delete(token); }, 300_000);
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

  clearAll() {
    for (const session of this.audioSessions.values()) {
      if (session?.cleanupTimer) clearTimeout(session.cleanupTimer);
      if (session?.process && !session.process.killed) {
        try { session.process.kill("SIGKILL"); } catch { /* already exited */ }
      }
    }
    this.audioSessions.clear();
    clearFfmpegQueue();
  }

  destroy() {
    this.clearAll();
    if (this._ensureAudioServerTimeout) {
      clearTimeout(this._ensureAudioServerTimeout);
      this._ensureAudioServerTimeout = null;
    }
    if (this._resolveAudioServerWaiter) {
      this._resolveAudioServerWaiter(null);
      this._resolveAudioServerWaiter = null;
    }
    this._ensureAudioServerWaiter = null;
    this.audioServerBaseUrl = null;
  }
}

module.exports = { AudioSessionManager, validateLocalStreamUrl };
