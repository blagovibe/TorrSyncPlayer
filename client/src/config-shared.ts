import sharedConfig from "../torrent-shared.json";

export const SHARED_TORRENT_LIMITS = {
  maxTorrentFileBytes: sharedConfig.maxTorrentFileBytes,
  maxMagnetLinkLength: sharedConfig.maxMagnetLinkLength,
  maxTorrentConnections: sharedConfig.maxTorrentConnections,
  maxTorrentFileCount: sharedConfig.maxTorrentFileCount,
  maxTorrentFilenameLength: sharedConfig.maxTorrentFilenameLength,
  videoExtensions: sharedConfig.videoExtensions as readonly string[],
  audioExtensions: sharedConfig.audioExtensions as readonly string[],
  nativeBrowserVideoFormats: sharedConfig.nativeBrowserVideoFormats as readonly string[],
  nativeBrowserAudioFormats: sharedConfig.nativeBrowserAudioFormats as readonly string[],
  blockedTrackerHosts: sharedConfig.blockedTrackerHosts,
  allowedTrackerProtocols: sharedConfig.allowedTrackerProtocols,
  maxTrackerUrlLength: sharedConfig.maxTrackerUrlLength,
  maxMagnetParamCount: sharedConfig.maxMagnetParamCount,
  maxMagnetParamValueLength: sharedConfig.maxMagnetParamValueLength,
} as const;
