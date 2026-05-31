import { describe, it, expect } from 'vitest'
import { validateMagnetURI, sanitizeInput } from '../sanitize'

describe('validateMagnetURI', () => {
  it('returns true for valid magnet URI', () => {
    expect(validateMagnetURI('magnet:?xt=urn:btih:abc123')).toBe(true)
  })

  it('returns false for invalid magnet URI', () => {
    expect(validateMagnetURI('https://example.com')).toBe(false)
  })

  it('returns false for empty string', () => {
    expect(validateMagnetURI('')).toBe(false)
  })

  it('returns false for partial magnet prefix', () => {
    expect(validateMagnetURI('magnet:?xt=urn:')).toBe(false)
  })

  it('returns true for magnet URI with additional parameters', () => {
    expect(validateMagnetURI('magnet:?xt=urn:btih:abc123&dn=Test')).toBe(true)
  })
})

describe('sanitizeInput', () => {
  it('removes HTML tags', () => {
    expect(sanitizeInput('<script>alert("xss")</script>')).toBe('scriptalert("xss")/script')
  })

  it('trims whitespace', () => {
    expect(sanitizeInput('  test  ')).toBe('test')
  })

  it('limits input length to 1000 characters', () => {
    const longInput = 'a'.repeat(1500)
    expect(sanitizeInput(longInput).length).toBe(1000)
  })

  it('handles empty string', () => {
    expect(sanitizeInput('')).toBe('')
  })

  it('handles string with only HTML tags', () => {
    expect(sanitizeInput('<div></div>')).toBe('div/div')
  })

  it('returns empty string for non-string input', () => {
    expect(sanitizeInput(null as any)).toBe('')
    expect(sanitizeInput(undefined as any)).toBe('')
    expect(sanitizeInput(123 as any)).toBe('')
    expect(sanitizeInput({} as any)).toBe('')
    expect(sanitizeInput([] as any)).toBe('')
  })

  describe('Event Attributes', () => {
    it('removes onclick attribute with double quotes', () => {
      expect(sanitizeInput('text" onclick="alert(1)"')).toBe('text"')
    })

    it('removes onclick attribute with single quotes', () => {
      expect(sanitizeInput("text' onclick='alert(1)'")).toBe("text'")
    })

    it('removes onclick attribute without quotes', () => {
      expect(sanitizeInput('text onclick=alert(1)')).toBe('text')
    })

    it('removes onerror attribute', () => {
      expect(sanitizeInput('text" onerror="alert(1)"')).toBe('text"')
    })

    it('removes onload attribute', () => {
      expect(sanitizeInput('text" onload="alert(1)"')).toBe('text"')
    })

    it('removes onmouseover attribute', () => {
      expect(sanitizeInput('text" onmouseover="alert(1)"')).toBe('text"')
    })

    it('removes onfocus attribute', () => {
      expect(sanitizeInput('text" onfocus="alert(1)"')).toBe('text"')
    })

    it('removes onblur attribute', () => {
      expect(sanitizeInput('text" onblur="alert(1)"')).toBe('text"')
    })

    it('removes onchange attribute', () => {
      expect(sanitizeInput('text" onchange="alert(1)"')).toBe('text"')
    })

    it('removes onsubmit attribute', () => {
      expect(sanitizeInput('text" onsubmit="alert(1)"')).toBe('text"')
    })

    it('removes multiple event attributes', () => {
      const input = 'text" onclick="alert(1)" onerror="alert(2)"'
      expect(sanitizeInput(input)).toBe('text"')
    })

    it('removes event attributes with spaces around equals', () => {
      expect(sanitizeInput('text" onclick = "alert(1)"')).toBe('text"')
    })

    it('removes event attributes case-insensitively', () => {
      expect(sanitizeInput('text" ONCLICK="alert(1)"')).toBe('text"')
      expect(sanitizeInput('text" OnClick="alert(1)"')).toBe('text"')
    })

    it('removes event attributes in HTML context', () => {
      expect(sanitizeInput('<div onclick="alert(1)">text</div>')).toBe('div text/div')
    })
  })

  describe('JavaScript URIs', () => {
    it('removes javascript: protocol', () => {
      expect(sanitizeInput('javascript:alert(1)')).toBe('alert(1)')
    })

    it('removes javascript: protocol with spaces', () => {
      expect(sanitizeInput('javascript : alert(1)')).toBe('alert(1)')
    })

    it('removes javascript: protocol case-insensitively', () => {
      expect(sanitizeInput('JAVASCRIPT:alert(1)')).toBe('alert(1)')
      expect(sanitizeInput('JavaScript:alert(1)')).toBe('alert(1)')
    })

    it('removes javascript: in href context', () => {
      expect(sanitizeInput('href="javascript:alert(1)"')).toBe('href="alert(1)"')
    })

    it('removes data:text/html URI', () => {
      expect(sanitizeInput('data:text/html,<script>alert(1)</script>')).toBe(',scriptalert(1)/script')
    })

    it('removes data:text/html URI case-insensitively', () => {
      expect(sanitizeInput('DATA:text/html,content')).toBe(',content')
      expect(sanitizeInput('Data:Text/Html,content')).toBe(',content')
    })

    it('removes data:text/html with spaces', () => {
      expect(sanitizeInput('data : text/html,content')).toBe(',content')
    })
  })

  describe('HTML Entities', () => {
    it('removes dangerous entities', () => {
      expect(sanitizeInput('&javascript;')).toBe('')
      expect(sanitizeInput('&alert;')).toBe('')
      expect(sanitizeInput('&script;')).toBe('')
    })

    it('removes numeric entities', () => {
      expect(sanitizeInput('&#x00000;')).toBe('')
      expect(sanitizeInput('&#60;')).toBe('')
      expect(sanitizeInput('&#62;')).toBe('')
    })

    it('removes unknown named entities', () => {
      expect(sanitizeInput('&unknown;')).toBe('')
      expect(sanitizeInput('&xyz;')).toBe('')
    })

    it('preserves bare ampersand without semicolon', () => {
      expect(sanitizeInput('&')).toBe('&')
      expect(sanitizeInput('&text')).toBe('&text')
      expect(sanitizeInput('text&')).toBe('text&')
    })

    it('preserves nbsp entity', () => {
      const nbsp = '&' + 'nbsp;'
      expect(sanitizeInput(nbsp)).toBe('&nbsp;')
    })
  })

  describe('Null Bytes and Control Characters', () => {
    it('removes null bytes', () => {
      expect(sanitizeInput('test\x00text')).toBe('testtext')
    })

    it('removes multiple null bytes', () => {
      expect(sanitizeInput('a\x00b\x00c\x00d')).toBe('abcd')
    })

    it('removes control characters', () => {
      expect(sanitizeInput('test\x01\x02\x03text')).toBe('testtext')
    })

    it('removes bell character', () => {
      expect(sanitizeInput('test\x07text')).toBe('testtext')
    })

    it('removes backspace character', () => {
      expect(sanitizeInput('test\x08text')).toBe('testtext')
    })

    it('removes vertical tab', () => {
      expect(sanitizeInput('test\x0Btext')).toBe('testtext')
    })

    it('removes form feed', () => {
      expect(sanitizeInput('test\x0Ctext')).toBe('testtext')
    })

    it('removes escape character', () => {
      expect(sanitizeInput('test\x1Btext')).toBe('testtext')
    })

    it('removes DEL character', () => {
      expect(sanitizeInput('test\x7Ftext')).toBe('testtext')
    })

    it('preserves newline character', () => {
      expect(sanitizeInput('test\ntext')).toBe('test\ntext')
    })

    it('preserves carriage return', () => {
      expect(sanitizeInput('test\rtext')).toBe('test\rtext')
    })

    it('preserves tab character', () => {
      expect(sanitizeInput('test\ttext')).toBe('test\ttext')
    })

    it('removes mixed control characters but preserves whitespace', () => {
      expect(sanitizeInput('line1\n\x01line2\r\x02line3\t\x03end')).toBe('line1\nline2\rline3\tend')
    })
  })

  describe('Combined Attack Vectors', () => {
    it('handles complex XSS attack with multiple vectors', () => {
      const input = '<img src=x onerror="javascript:alert(1)">'
      expect(sanitizeInput(input)).toBe('img src=x')
    })

    it('handles nested attack vectors', () => {
      const input = '<div onclick="alert(1)"><script>alert(2)</script></div>'
      expect(sanitizeInput(input)).toBe('div scriptalert(2)/script/div')
    })

    it('handles attack with null bytes', () => {
      const input = 'test\x00<script>alert(1)</script>'
      expect(sanitizeInput(input)).toBe('testscriptalert(1)/script')
    })

    it('handles attack with control characters', () => {
      const input = 'test\x1B[31m<script>alert(1)</script>'
      expect(sanitizeInput(input)).toBe('test[31mscriptalert(1)/script')
    })
  })

  describe('Edge Cases', () => {
    it('handles string with only special characters', () => {
      expect(sanitizeInput('!@#$%^&*()')).toBe('!@#$%^&*()')
    })

    it('handles unicode characters', () => {
      expect(sanitizeInput('\u041F\u0440\u0438\u0432\u0435\u0442 \u043C\u0438\u0440')).toBe('\u041F\u0440\u0438\u0432\u0435\u0442 \u043C\u0438\u0440')
      expect(sanitizeInput('\u4F60\u597D\u4E16\u754C')).toBe('\u4F60\u597D\u4E16\u754C')
      expect(sanitizeInput('\uD83C\uDF89\uD83C\uDF8A')).toBe('\uD83C\uDF89\uD83C\uDF8A')
    })

    it('handles very long string', () => {
      const longString = 'a'.repeat(10000)
      expect(sanitizeInput(longString).length).toBe(1000)
    })

    it('handles string with only whitespace', () => {
      expect(sanitizeInput('   ')).toBe('')
    })

    it('handles string with newlines and tabs', () => {
      expect(sanitizeInput('line1\nline2\tline3')).toBe('line1\nline2\tline3')
    })

    it('handles string with HTML-like content but no actual tags', () => {
      expect(sanitizeInput('a < b > c')).toBe('a  b  c')
    })
  })
})
