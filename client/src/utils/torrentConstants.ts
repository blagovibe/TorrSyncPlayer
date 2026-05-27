/**
 * Shared magnet link constants.
 * See BEP-0009 and BEP-0005 for the BitTorrent info hash specification.
 * btih = BitTorrent Info Hash (SHA-1, 40 hex chars)
 * btmh = BitTorrent Merkle Hash (SHA-1, 40 hex chars)
 * ed2k = eDonkey2000 hash (MD4, 32 hex chars)
 */
import { SHARED_TORRENT_LIMITS } from "../config-shared";

const INFO_HASH_PATTERN = /^(?:urn:)?(?:btih|btmh|sha1):[a-fA-F0-9]{40}$|^(?:urn:)?ed2k:[a-fA-F0-9]{32}$/;
const MAX_MAGNET_LINK_LENGTH = SHARED_TORRENT_LIMITS.maxMagnetLinkLength;

function isValidMagnetLinkFormat(magnetLink: string): boolean {
  if (!magnetLink.startsWith("magnet:?")) return false;
  const queryStart = magnetLink.indexOf("?");
  if (queryStart === -1) return false;
  const params = new URLSearchParams(magnetLink.slice(queryStart + 1));
  const xt = params.get("xt");
  if (!xt) return false;
  return INFO_HASH_PATTERN.test(xt);
}

export { isValidMagnetLinkFormat, MAX_MAGNET_LINK_LENGTH };
