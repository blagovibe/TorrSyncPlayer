# Security Policy

## Supported Versions

| Version | Supported          | Security Updates Until |
| ------- | ------------------ | ---------------------- |
| 1.x     | :white_check_mark: | December 2026          |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue in TorrSyncPlayer, please report it responsibly.

### How to Report

- **GitHub Security Advisory**: For any security vulnerability, use GitHub's [Security Advisory](https://github.com/blagovibe/TorrSyncPlayer/security/advisories/new) feature (private, goes to maintainers)
- **GitHub Issues**: For non-critical security concerns, you can open a public issue

### What to Include

When reporting a vulnerability, please include:

1. Description of the vulnerability
2. Steps to reproduce
3. Potential impact
4. Suggested fix (if any)
5. Any relevant logs or error messages

### Response Timeline

- **Initial Response**: Within 3 business days
- **Investigation**: Within 7 business days
- **Fix**: Depending on severity (critical issues prioritized)
- **Release**: Coordinated disclosure

## Security Features

- Single per-process access token (random at startup, constant-time compare,
  sent as `X-Access-Token`). No accounts, no JWT, no token revocation.
  `--token` replaces the random value with a fixed one; it then survives a
  restart and is no longer printed to the log, so treat the flag — or the unit
  file carrying it — as the secret it is.
- bcrypt password hashing (cost=12) for room passwords
- Rate limiting (per-IP; a bad token cannot be used to flood)
- Forwarded headers ignored unless the proxy is explicitly trusted
- Security headers (CSP, X-Frame-Options, Permissions-Policy). HSTS is sent
  only over TLS, so it is absent on a plain-HTTP listener
- TLS 1.2+ support
- Input validation on all endpoints

## Security Best Practices for Deployment

1. **Always use HTTPS** in production
2. Share the access token printed at startup only with people you trust
3. Configure proper `CORS_ORIGINS` for your domain
4. Set up rate limiting appropriate for your use case
5. Keep the application updated to the latest version
6. Use the `--auto-tls` flag or provide your own TLS certificates
7. Configure `TRUSTED_PROXIES` if behind a reverse proxy

## Known Security Limitations

- Torrent pieces are held in memory by default (`--memory-capacity`, max 256 GB);
  use `--data-dir` with `--disk-storage` to persist them to disk. There is no
  user storage at all — auth is one token per process.
- SSE endpoint doesn't scale to many concurrent connections - use for small groups
- **`/stream` no longer takes a URL ticket.** The endpoint accepts the ordinary
  `X-Access-Token` header, which `mpv` sends because it can be told to
  (`--http-header-fields`). A room-scoped `CanRead` check precedes serving, so
  holding the process token is necessary but not sufficient — a non-participant
  is rejected. The bearer-in-the-URL deviation (deviation-0029) is closed.
- **`/metrics`, `/health`, `/api/v1/version` and `/swagger/*` are reachable
  without the access token**, each behind its own rate limit. The metrics
  endpoint exposes room counts, peer counts and request rates; it is meant
  for monitoring tools that cannot carry the token. Do not publish it
  without putting an authenticating proxy in front of it.
- **A room id and a magnet hash are not secrets**, and are not treated as such.
  They are identifiers, logged by design.

## What the Server Sees

This section is an inventory of what the server handles, checked against the
code rather than against intent. It lists only what is observed; it makes no
claims about what is absent from the list, because such a claim is only as good
as the last time someone read the code.

**Media data passes through the server.** The backend is the torrent client
(`anacrolix/torrent`). The desktop app has no torrent client of its own: it
sends a magnet URI to `POST /api/v1/torrents` and the player then reads
`GET /api/v1/torrents/{id}/stream`. The chain is swarm peers → backend →
desktop app, so the media bytes are handled by the server. Pieces are held in
memory by default, or on disk under `--data-dir` with `--disk-storage`.

**Playback synchronisation, with positions and times.** Every play, pause, seek
and buffering report is recorded by `internal/sync/service.go`, including the
room id, the acting client and the playback position.

**Room membership.** Room names, the host's user id, the client ids
(`X-Client-ID`), and the peer ids generated per session.

**Room passwords.** Received on the request and stored as a bcrypt hash
(cost 12). Only the hash is kept.

**The access token.** One token per process, printed to stdout at startup unless
`--token` supplies a fixed value.

**HTTP request metadata.** The access log records method, path, status,
duration and remote address. It records `r.URL.Path` and deliberately not the
query string — which is why stream tickets do not reach the log.

### What is written where

| Destination | Contents |
|---|---|
| Log (stdout) | HTTP method/path/status/duration/remote address; sync operations with room id, acting client and position; room joins and leaves with peer and client ids; magnet hashes |
| Disk: `rooms.json` | Room metadata including the password **hash**; file mode 0600 |
| Disk: `sync.json` | Per-room playback positions |
| Disk: `--data-dir` | Torrent pieces, when disk storage is enabled |
| Memory | Buffered media pieces, session state, event buffers |

The torrent library's own peer logging is not connected to the application
logger, so swarm peer addresses are not written by this project. Passwords are
never logged; a wrong room password produces a warning carrying only the room id.

## Threat Model

**The server operator is the main adversary for "what can be learned about my
viewing".** They can see which rooms exist, who is in them, every
synchronisation command with its position, and the media itself. A room is not a
private channel; treat the server as able to reconstruct what each participant
watched and when.

**Possession of the access token means full API access.** There is one token per
process and no per-user identity. Anyone holding it can create rooms, join
rooms and drive playback as the host. The token's protection is the
distribution, not the mechanism: share it only with people who would already be
allowed to control the room.

**`X-Client-ID` is self-declared.** It is not authenticated. A client may claim
any identifier, so `X-Client-ID` distinguishes sessions for bookkeeping, not for
access control.

**Room passwords are the only thing separating a room from an open one, and
they are optional.** A room created without a password is joinable by anyone
who knows its id. Passwords are compared against a bcrypt hash, which is the
right primitive, but there is no attempt limit beyond the per-IP rate limiter.

Thank you for helping keep TorrSyncPlayer secure!