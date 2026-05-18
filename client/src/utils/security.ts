/**
 * Content Security Policy and security headers for Electron.
 *
 * Provides defense-in-depth against XSS, code injection, and other attacks.
 */

export const CSP_POLICY = {
  "default-src": ["'self'"],
  "script-src": ["'self'"],
  "style-src": ["'self'", "'unsafe-inline'"],
  "img-src": ["'self'", "data:", "blob:"],
  "media-src": ["'self'", "blob:", "http://127.0.0.1"],
  "connect-src": [
    "'self'",
    "wss://*.peerjs.com",
    "wss://*.openwebtorrent.com",
    "wss://*.webtorrent.dev",
    "wss://*.btorrent.xyz",
  ],
  "font-src": ["'self'"],
  "object-src": ["'none'"],
  "base-uri": ["'self'"],
  "form-action": ["'self'"],
  "frame-ancestors": ["'none'"],
} as const;

export function buildCSPHeader(): string {
  const directives: string[] = [];
  const keys = Object.keys(CSP_POLICY);
  for (let i = 0; i < keys.length; i++) {
    const key = keys[i];
    const values = (CSP_POLICY as Record<string, readonly string[]>)[key];
    directives.push(key + " " + values.join(" "));
  }
  return directives.join("; ");
}

export const SECURITY_HEADERS = {
  "Content-Security-Policy": buildCSPHeader(),
  "X-Content-Type-Options": "nosniff",
  "X-Frame-Options": "DENY",
  "X-XSS-Protection": "1; mode=block",
  "Referrer-Policy": "no-referrer",
  "Strict-Transport-Security": "max-age=31536000; includeSubDomains",
  "Permissions-Policy": "camera=(), microphone=(), geolocation=()",
} as const;
