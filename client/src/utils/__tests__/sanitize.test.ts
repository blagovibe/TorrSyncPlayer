import { describe, expect, it } from 'vitest';
import { escapeHtml, sanitizeChatMessage, validateChatMessage, sanitizePeerId } from '../sanitize';

describe('sanitize', () => {
  describe('escapeHtml', () => {
    it('escapes ampersands', () => {
      expect(escapeHtml('foo & bar')).toBe('foo &amp; bar');
    });

    it('escapes less-than signs', () => {
      expect(escapeHtml('a < b')).toBe('a &lt; b');
    });

    it('escapes greater-than signs', () => {
      expect(escapeHtml('a > b')).toBe('a &gt; b');
    });

    it('escapes double quotes', () => {
      expect(escapeHtml('say "hello"')).toBe('say &quot;hello&quot;');
    });

    it('escapes single quotes', () => {
      expect(escapeHtml("it's")).toBe('it&#x27;s');
    });

    it('escapes forward slashes', () => {
      expect(escapeHtml('a/b')).toBe('a&#x2F;b');
    });

    it('escapes backticks', () => {
      expect(escapeHtml('`code`')).toBe('&#x60;code&#x60;');
    });

    it('escapes equals sign', () => {
      expect(escapeHtml('a=b')).toBe('a&#x3D;b');
    });

    it('escapes all dangerous characters in XSS payload', () => {
      const xss = '<script>alert("xss")</script>';
      const escaped = escapeHtml(xss);
      expect(escaped).not.toContain('<script>');
      expect(escaped).toBe('&lt;script&gt;alert(&quot;xss&quot;)&lt;&#x2F;script&gt;');
    });

    it('returns empty string for empty input', () => {
      expect(escapeHtml('')).toBe('');
    });
  });

  describe('validateChatMessage', () => {
    it('returns trimmed message for valid input', () => {
      expect(validateChatMessage('hello world')).toBe('hello world');
    });

    it('trims whitespace', () => {
      expect(validateChatMessage('  hello  ')).toBe('hello');
    });

    it('returns null for empty string', () => {
      expect(validateChatMessage('')).toBeNull();
    });

    it('returns null for whitespace-only string', () => {
      expect(validateChatMessage('   ')).toBeNull();
    });

    it('returns null for messages exceeding max length', () => {
      expect(validateChatMessage('a'.repeat(501))).toBeNull();
    });

    it('accepts messages at max length', () => {
      expect(validateChatMessage('a'.repeat(500))).toBe('a'.repeat(500));
    });

    it('returns null for non-string input', () => {
      expect(validateChatMessage(123)).toBeNull();
      expect(validateChatMessage(null)).toBeNull();
      expect(validateChatMessage(undefined)).toBeNull();
      expect(validateChatMessage({})).toBeNull();
    });

    it('returns null for strings with control characters', () => {
      expect(validateChatMessage('hello\x00world')).toBeNull();
      expect(validateChatMessage('hello\x1Fworld')).toBeNull();
    });
  });

  describe('sanitizeChatMessage', () => {
    it('sanitizes and escapes a valid message', () => {
      expect(sanitizeChatMessage('Hello <world>')).toBe('Hello &lt;world&gt;');
    });

    it('truncates long messages', () => {
      const long = 'a'.repeat(600);
      expect(sanitizeChatMessage(long).length).toBe(500);
    });

    it('uses custom max length', () => {
      expect(sanitizeChatMessage('hello world', 5)).toBe('hello');
    });

    it('returns empty string for invalid input', () => {
      expect(sanitizeChatMessage('')).toBe('');
      expect(sanitizeChatMessage('   ')).toBe('');
    });

    it('escapes XSS payloads', () => {
      const xss = '<img src=x onerror=alert(1)>';
      const sanitized = sanitizeChatMessage(xss);
      expect(sanitized).not.toContain('<img');
      expect(sanitized).not.toContain('onerror=');
      expect(sanitized).toContain('&lt;');
      expect(sanitized).toContain('onerror&#x3D;');
    });
  });

  describe('sanitizePeerId', () => {
    it('returns sanitized alphanumeric peer ID', () => {
      expect(sanitizePeerId('ABC123')).toBe('ABC123');
    });

    it('trims whitespace', () => {
      expect(sanitizePeerId('  ABC123  ')).toBe('ABC123');
    });

    it('returns empty string for invalid characters', () => {
      expect(sanitizePeerId('<script>')).toBe('');
      expect(sanitizePeerId('foo bar')).toBe('');
      expect(sanitizePeerId('a@b')).toBe('');
    });

    it('allows hyphens and dots', () => {
      expect(sanitizePeerId('peer-id.test')).toBe('peer-id.test');
    });

    it('truncates long peer IDs', () => {
      const long = 'a'.repeat(200);
      expect(sanitizePeerId(long).length).toBe(100);
    });

    it('returns empty string for empty input', () => {
      expect(sanitizePeerId('')).toBe('');
      expect(sanitizePeerId('   ')).toBe('');
    });
  });
});
