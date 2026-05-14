# Signaling Server

This directory is reserved for a future self-hosted signaling server.

**Current status**: TorrSyncPlayer uses the PeerJS cloud-hosted signaling broker. No local signaling server is required for development, testing, or production use.

If a self-hosted option is added in the future, it should be implemented as a Go module under this directory. Any changes must include:
- `go.mod` and server source
- Run instructions
- Matching client configuration updates
- Updated tests

Do not commit generated dependency folders such as `node_modules/` here.
