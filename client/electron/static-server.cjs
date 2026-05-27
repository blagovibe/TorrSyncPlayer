const http = require("node:http");
const fs = require("node:fs/promises");
const path = require("node:path");
const { electronLogger } = require("./electron-logger.cjs");

const MIME_TYPES = new Map([
  [".html", "text/html; charset=utf-8"],
  [".js", "text/javascript; charset=utf-8"],
  [".css", "text/css; charset=utf-8"],
  [".json", "application/json; charset=utf-8"],
  [".svg", "image/svg+xml"],
  [".png", "image/png"],
  [".jpg", "image/jpeg"],
  [".jpeg", "image/jpeg"],
  [".gif", "image/gif"],
  [".ico", "image/x-icon"],
  [".webp", "image/webp"],
  [".map", "application/json; charset=utf-8"],
  [".txt", "text/plain; charset=utf-8"],
]);

function getContentType(filePath) {
  return MIME_TYPES.get(path.extname(filePath).toLowerCase()) || "application/octet-stream";
}

function isPathInsideDirectory(targetPath, directoryPath) {
  if (targetPath.includes("\0")) return false;
  const resolved = path.resolve(targetPath);
  const resolvedDir = path.resolve(directoryPath);
  const relativePath = path.relative(resolvedDir, resolved);
  return relativePath !== "" && !relativePath.startsWith("..") && !path.isAbsolute(relativePath);
}

async function fileExists(filePath) {
  try {
    const stat = await fs.stat(filePath);
    return stat.isFile();
  } catch {
    return false;
  }
}

async function resolveStaticAsset(requestPath, distDir) {
  const decodedPath = decodeURIComponent(requestPath);
  const sanitizedPath = decodedPath === "/" ? "/index.html" : decodedPath;
  const absolutePath = path.normalize(path.join(distDir, sanitizedPath));

  if (!isPathInsideDirectory(absolutePath, distDir)) {
    return path.join(distDir, "index.html");
  }

  if (await fileExists(absolutePath)) {
    return absolutePath;
  }

  if (!path.extname(sanitizedPath)) {
    return path.join(distDir, "index.html");
  }

  return null;
}

function buildSecurityHeaders(cspHeaderValue) {
  const headers = {
    "X-Content-Type-Options": "nosniff",
    "X-Frame-Options": "DENY",
    "Content-Security-Policy": cspHeaderValue,
  };
  return headers;
}

function setResponseSecurityHeaders(response, cspHeaderValue) {
  const headers = buildSecurityHeaders(cspHeaderValue);
  for (const [key, value] of Object.entries(headers)) {
    response.setHeader(key, value);
  }
  response.setHeader("X-XSS-Protection", "1; mode=block");
  response.setHeader("Referrer-Policy", "no-referrer");
}

function getDefaultPeerConnectSources() {
  return ['wss://0.peerjs.com', 'wss://*.openwebtorrent.com', 'wss://*.webtorrent.dev', 'wss://*.btorrent.xyz'];
}

function buildCspHeader(streamBaseUrl, extraPeerSources) {
  const mediaSource = streamBaseUrl || "'self'";
  const peerSources = (extraPeerSources && extraPeerSources.length > 0)
    ? extraPeerSources
    : getDefaultPeerConnectSources();
  const connectSources = ["'self'", ...peerSources, mediaSource].join(' ');
  return `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob: ${mediaSource}; connect-src ${connectSources}; font-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-src 'none'; frame-ancestors 'none'; worker-src 'self' blob:; upgrade-insecure-requests`;
}

class StaticServer {
  constructor({ distDir, devServerUrl, torrentBridge }) {
    this.distDir = distDir;
    this.devServerUrl = devServerUrl;
    this.torrentBridge = torrentBridge;
    this.serverPromise = null;
    this.instance = null;
  }

  get url() {
    return this.instance ? this.instance.url : null;
  }

  getAllowedOrigins() {
    const origins = new Set();
    if (this.instance) {
      origins.add(this.instance.url);
    }
    try { origins.add(new URL(this.devServerUrl).origin); } catch { /* ignore */ }
    return origins;
  }

  validateOrigin(request) {
    const origin = request.headers.origin ?? request.headers.referer;
    const allowed = this.getAllowedOrigins();
    if (!origin) {
      return allowed.size === 0;
    }
    try {
      const parsed = new URL(origin);
      if (allowed.size === 0) return true;
      for (const a of allowed) {
        if (new URL(a).origin === parsed.origin) return true;
      }
      return false;
    } catch {
      return false;
    }
  }

  async start() {
    if (this.instance) {
      return this.instance;
    }

    if (this.serverPromise) {
      return this.serverPromise;
    }

    this.serverPromise = new Promise((resolve, reject) => {
      let streamBaseUrl = null;
      const server = http.createServer(async (request, response) => {
        try {
          if (!request.url) {
            response.statusCode = 400;
            setResponseSecurityHeaders(response, buildCspHeader(streamBaseUrl));
            response.end();
            return;
          }

          if (!this.validateOrigin(request)) {
            response.statusCode = 403;
            response.setHeader("X-Content-Type-Options", "nosniff");
            response.end("Forbidden");
            return;
          }

          const { pathname } = new URL(request.url, "http://127.0.0.1");

          if (pathname.startsWith("/mux/") || pathname.startsWith("/audio/") || pathname.startsWith("/subtitle/")) {
            this.torrentBridge.handleAudioRequest(request, response).catch((error) => {
              electronLogger.error("Audio/mux request failed:", error);
              if (!response.headersSent) {
                response.statusCode = 500;
                setResponseSecurityHeaders(response, buildCspHeader(streamBaseUrl));
                response.end("Stream request failed");
              } else {
                response.destroy();
              }
            });
            return;
          }

          if (request.method !== "GET" && request.method !== "HEAD") {
            response.statusCode = 405;
            response.setHeader("Allow", "GET, HEAD");
            setResponseSecurityHeaders(response, buildCspHeader(streamBaseUrl));
            response.end();
            return;
          }

          const assetPath = await resolveStaticAsset(pathname, this.distDir);

          if (!assetPath) {
            response.statusCode = 404;
            setResponseSecurityHeaders(response, buildCspHeader(streamBaseUrl));
            response.end();
            return;
          }

          const body = await fs.readFile(assetPath);
          response.statusCode = 200;
          response.setHeader("Content-Type", getContentType(assetPath));
          response.setHeader("Cache-Control", "no-store");
          setResponseSecurityHeaders(response, buildCspHeader(streamBaseUrl));
          response.end(request.method === "HEAD" ? undefined : body);
        } catch (error) {
          if (!response.headersSent) {
            response.statusCode = 500;
            setResponseSecurityHeaders(response, buildCspHeader(streamBaseUrl));
            response.end("Internal Server Error");
          } else if (!response.writableEnded) {
            response.destroy();
          }
        }
      });

      server.listen(0, "127.0.0.1", () => {
        const address = server.address();
        if (!address || typeof address === "string") {
          reject(new Error("Unable to start local static server"));
          return;
        }

        streamBaseUrl = `http://127.0.0.1:${address.port}`;
        this.torrentBridge.setStreamBaseUrl(streamBaseUrl);

        resolve({ server, url: streamBaseUrl });
      });

      server.on("error", reject);
    }).then((result) => {
      this.instance = result;
      this.serverPromise = null;
      result.server.on("error", (err) => {
        electronLogger.error("Static server error:", err);
      });
      return result;
    }).catch((error) => {
      this.serverPromise = null;
      throw error;
    });

    return this.serverPromise;
  }
}

module.exports = { StaticServer, buildCspHeader };
