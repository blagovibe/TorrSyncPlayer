# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in TorrSyncPlayer, please report it responsibly by opening a [GitHub Issue](https://github.com/Kilo-Org/TorrSyncPlayer/issues) with the "security" label.

Please include:

- A description of the vulnerability
- Steps to reproduce the issue
- Affected versions
- Any suggested mitigation

We will acknowledge your report within 48 hours and work toward a fix as quickly as possible.

## Security Considerations

- TorrSyncPlayer runs torrent code in the Electron main process. All torrent file and magnet link inputs are validated before processing.
- The IPC between renderer and main processes uses origin validation to prevent unauthorized access.
- Audio streaming endpoints validate request origins to prevent unauthorized stream access.
- The `ip` dependency is patched to address known vulnerabilities in upstream versions.
