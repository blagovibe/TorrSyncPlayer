// Browser build shim: disables DHT path in torrent-discovery.
// torrent-discovery checks `typeof DHT !== "function"` and skips DHT when false.
export const Client = {};
