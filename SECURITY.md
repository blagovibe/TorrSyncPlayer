# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in TorrSyncPlayer, please report it
responsibly by following these steps:

1. **Do not** open a public GitHub issue for the vulnerability.
2. Send a description of the issue to the project maintainers via GitHub
   private vulnerability reporting: https://github.com/blagovibe/TorrSyncPlayer/security/advisories/new
3. Include steps to reproduce, affected versions, and potential impact.
4. Allow reasonable time for the issue to be addressed before public disclosure.

## Security Considerations

- JWT tokens are signed with HS256 and should use a strong secret (min 32 chars)
- The `JWT_SECRET` environment variable must be set in production (the server refuses to start without one)
- Passwords are hashed with bcrypt (cost=12)
- HTTPS is enforced in production
- Rate limiting is applied per-IP
- CSRF protection is applied for non-JWT requests
- Self-signed certificates are for development only
