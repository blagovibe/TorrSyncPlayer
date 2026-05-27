const { spawn } = require("node:child_process");
const { randomBytes } = require("node:crypto");
const { electronLogger } = require("./electron-logger.cjs");

const MAX_CONCURRENT_FFMPEG = 3;
const MAX_FFMPEG_QUEUE_SIZE = 20;

let activeFfmpegCount = 0;
const ffmpegQueue = [];

function processQueue() {
  while (ffmpegQueue.length > 0 && activeFfmpegCount < MAX_CONCURRENT_FFMPEG) {
    const next = ffmpegQueue.shift();
    next.task();
  }
}

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

function generateSessionToken() {
  return randomBytes(16).toString("hex");
}

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

function createErrorFromSpawn(error, fallbackMessage) {
  if (error instanceof Error) return error;
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

      return { index, label, language, codecName, channels, sampleRate };
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

      return { index, label, language, codecName, forced, default: defaultTrack };
    })
    .filter((stream) => stream.codecName || stream.label);
}

async function runFfprobe(streamUrl, selectStreams, showEntries) {
  return new Promise((resolve, reject) => {
    const ffprobe = spawn("ffprobe", [
      "-v", "error",
      "-select_streams", selectStreams,
      "-show_entries", showEntries,
      "-of", "json",
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

function getActiveFfmpegCount() {
  return activeFfmpegCount;
}

function getFfmpegQueueLength() {
  return ffmpegQueue.length;
}

function clearFfmpegQueue() {
  const pending = ffmpegQueue.splice(0);
  for (const entry of pending) {
    try {
      entry.reject(new Error("ffmpeg pipeline is shutting down"));
    } catch { /* ignore */ }
  }
}

module.exports = {
  runWithFfmpegLimit,
  generateSessionToken,
  checkFfmpegAvailable,
  parseProbeResult,
  parseSubtitleResult,
  runFfprobe,
  getActiveFfmpegCount,
  getFfmpegQueueLength,
  clearFfmpegQueue,
  MAX_CONCURRENT_FFMPEG,
  MAX_FFMPEG_QUEUE_SIZE,
};
