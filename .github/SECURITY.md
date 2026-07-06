# Security Policy

## Supported Versions

| Version | Supported          | Security Updates Until |
| ------- | ------------------ | ---------------------- |
| 1.x     | :white_check_mark: | December 2026          |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue in TorrSyncPlayer, please report it responsibly.

### How to Report

- **Email**: security@torrsyncplayer.local (or open an issue on GitHub)
- **GitHub Issues**: For non-critical security concerns, you can open a public issue
- **Security Advisory**: For critical vulnerabilities, use GitHub's Security Advisory feature

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

- JWT authentication with token revocation (JTI-based)
- bcrypt password hashing (cost=12)
- CSRF protection with session binding
- Rate limiting (per-IP and per-user)
- Security headers (CSP, HSTS, X-Frame-Options)
- TLS 1.2+ support
- Input validation on all endpoints

## Security Best Practices for Deployment

1. **Always use HTTPS** in production
2. Set a strong `JWT_SECRET` environment variable (minimum 32 bytes)
3. Configure proper `CORS_ORIGINS` for your domain
4. Set up rate limiting appropriate for your use case
5. Keep the application updated to the latest version
6. Use the `--auto-tls` flag or provide your own TLS certificates
7. Configure `TRUSTED_PROXIES` if behind a reverse proxy

## Known Security Limitations

- In-memory user storage by default (no persistence) - use `--data-dir` for file-based persistence
- Development STUN servers hardcoded - configure custom in production
- SSE endpoint doesn't scale to many concurrent connections - use for small groups

Thank you for helping keep TorrSyncPlayer secure!