# Signaling Server

There is currently no active in-repository signaling server. TorrSyncPlayer uses PeerJS signaling from `client/src/services/P2PService.ts`.

This directory is reserved for a future self-hosted Go signaling server. Do not commit generated dependency folders such as `node_modules/` here. If a custom server is reintroduced, add `go.mod`, server source, run instructions, and the matching client configuration in the same change.
