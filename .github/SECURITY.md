# Security Policy

## Supported Versions

| Version | Supported          | Security Updates Until |
| ------- | ------------------ | ---------------------- |
| 0.3.x   | :white_check_mark: | December 2026          |

The supported line is the one the project is on: `0.3.x` is where releases
are published, and a table listing a version that does not exist protects
nothing while reading as though something does.

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
- **`/stream` takes no URL ticket.** The endpoint accepts the ordinary
  `X-Access-Token` header, which `mpv` sends because it can be told to
  (`--http-header-fields`). A `CanRead` ownership check precedes serving: the
  caller must own the torrent, and nobody else's backend is reachable at all —
  guests pull from their own. The bearer-in-the-URL deviation (deviation-0029)
  is closed, and so is the room-scoped variant of it (deviation-0039): there is
  no guest access path left to scope.
- **`/metrics`, `/health`, `/api/v1/version` and `/swagger/*` are reachable
  without the access token**, each behind its own rate limit. The metrics
  endpoint exposes torrent and request rates; it is meant
  for monitoring tools that cannot carry the token. Do not publish it
  without putting an authenticating proxy in front of it.
- **A magnet hash is not a secret**, and is not treated as such. It is an
  identifier, logged by design. Room ids no longer exist anywhere to leak:
  rooms are named by invite code, which never touches this server.

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

**Playback synchronisation does not.** Play, pause, seek and buffering reports
travel encrypted libp2p streams between the participants' own applications;
the server never sees them, has no endpoint for them, and keeps no state about
them (idea-0086). What the server operator can reconstruct about viewing ends
at the media their own backend fetched.

**Room membership does not either.** There are no rooms, names, hosts or
passwords on this server and nothing persists them. What the DHT rendezvous
sees instead is below, under Threat Model — it is a different machine with a
different view.

**The access token.** One token per process, printed to stdout at startup unless
`--token` supplies a fixed value. It guards this backend only: nobody else's
application talks to it, and the invite code does not carry it.

**HTTP request metadata.** The access log records method, path, status,
duration and remote address. It records `r.URL.Path` and deliberately not the
query string.

### What is written where

| Destination | Contents |
|---|---|
| Log (stdout) | HTTP method/path/status/duration/remote address; torrent adds and magnet hashes |
| Disk: `--data-dir` | Torrent pieces, when disk storage is enabled; the torrent registry |
| Memory | Buffered media pieces |

The torrent library's own peer logging is not connected to the application
logger, so swarm peer addresses are not written by this project.

## Threat Model

**The server operator learns almost nothing about viewing.** The backend they
run fetches media for its owner and answers that owner's application. Rooms,
commands and positions never reach it: a room is not a server resource anymore,
so "which rooms exist" is not a question this machine can answer. What it can
reconstruct is which torrents its owner fetched — the media itself, as before.

**The DHT rendezvous is the party that sees something.** Whoever runs the
bootstrap nodes — by default someone else's public nodes — sees peer IP
addresses and provider records: who looked for which rendezvous key, and who
announced. It does not see commands or positions: streams run under Noise/TLS,
and admission needs the password on top. A relay, when one carries a harsh-NAT
pair, sees encrypted traffic between two peer IDs and nothing above that.
Treat the default bootstrap as an introducer that knows the guest list, not as
a participant. `--bootstrap` points at other nodes; running your own removes
even that view, as long as every participant points at it.

**Possession of the access token means full API access — to one backend.**
There is one token per process and no per-user identity. Anyone holding it
controls that backend's torrents. What it no longer means is anything about
rooms: there are no rooms to create, join or drive from this API, and the
invite code does not carry the token. The token's protection is the
distribution, not the mechanism: share it only with people who would already be
allowed to touch the machine it guards — which, since guests never need it, is
nobody outside it.

**`X-Client-ID` is self-declared.** It is not authenticated. A client may claim
any identifier, so `X-Client-ID` distinguishes sessions for bookkeeping, not for
access control.

**The invite code is the room's only secret, and it is shared by copying.**
Community, password and key travel one line from host to guest, by hand or by
messenger. Anyone holding it can watch — and, holding the key, can also open
streams, which is why strangers' commands are ignored and only the host's are
obeyed. There is no attempt limit and no revocation: a leaked code cannot be
unshared short of opening a new room. The community alone finds the room's
provider records — they are public by DHT construction — but finding is not
joining: without the key there is no connection, without the password no
stream. A room created without care around the code is joinable by anyone who
is told it, which, for a random nine-character community, means exactly that.

Thank you for helping keep TorrSyncPlayer secure!