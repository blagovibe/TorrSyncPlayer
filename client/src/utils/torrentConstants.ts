const MAGNET_LINK_PATTERN = /^magnet:\?xt=urn:(?:btih:[a-fA-F0-9]{40}|btmh:[a-fA-F0-9]{40}|sha1:[a-fA-F0-9]{40}|ed2k:[a-fA-F0-9]{32})(?:&.+)?$/;
const MAX_MAGNET_LINK_LENGTH = 8000;

export { MAGNET_LINK_PATTERN, MAX_MAGNET_LINK_LENGTH };
