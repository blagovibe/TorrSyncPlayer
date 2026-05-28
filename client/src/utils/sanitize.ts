/**
 * HTML entity encoding map for XSS prevention.
 */
const HTML_ENTITIES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#x27;',
  '/': '&#x2F;',
  '`': '&#x60;',
  '=': '&#x3D;',
};

const HTML_ENTITY_PATTERN = /[&<>"'`=/]/g;

/**
 * Escapes HTML entities in a string to prevent XSS attacks.
 * Use this for all user-generated content before rendering.
 *
 * @param input - The string to escape
 * @returns The escaped string safe for HTML insertion
 */
export function escapeHtml(input: string): string {
  if (!input) return '';
  return input.replace(HTML_ENTITY_PATTERN, (char) => HTML_ENTITIES[char] || char);
}

/**
 * Sanitizes a chat message for safe display.
 * - Trims whitespace
 * - Escapes HTML entities
 * - Limits length
 *
 * @param message - Raw chat message content
 * @param maxLength - Maximum allowed length (default: 500)
 * @returns Sanitized message string
 */
export function sanitizeChatMessage(message: string, maxLength = 500): string {
  if (!message || typeof message !== 'string') return '';
  const trimmed = message.trim();
  if (!trimmed) return '';
  const truncated = trimmed.slice(0, maxLength);
  return escapeHtml(truncated);
}

/**
 * Validates that a chat message is safe to process.
 * Returns null if the message is invalid or potentially malicious.
 *
 * @param message - Raw chat message to validate
 * @returns The original message if valid, null otherwise
 */
export function validateChatMessage(message: unknown): string | null {
  if (typeof message !== 'string') return null;
  const trimmed = message.trim();
  if (!trimmed || trimmed.length > 500) return null;
  if (/\p{C}/u.test(trimmed)) return null;
  return trimmed;
}

/**
 * Sanitizes a peer ID for display.
 * Only allows alphanumeric characters, hyphens, and dots.
 *
 * @param peerId - Raw peer ID
 * @returns Sanitized peer ID or empty string if invalid
 */
export function sanitizePeerId(peerId: string): string {
  if (!peerId || typeof peerId !== 'string') return '';
  const trimmed = peerId.trim();
  if (!/^[a-zA-Z0-9._-]+$/.test(trimmed)) return '';
  return escapeHtml(trimmed.slice(0, 100));
}
