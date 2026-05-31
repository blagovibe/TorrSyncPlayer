export function sanitizeInput(input: string): string {
  if (typeof input !== 'string') return ''
  
  return input
    // Удаляем HTML теги
    .replace(/[<>]/g, '')
    // Удаляем атрибуты событий (onclick, onerror, onload и т.д.)
    .replace(/\bon\w+\s*=\s*["'][^"']*["']/gi, '')
    .replace(/\bon\w+\s*=\s*[^\s>]*/gi, '')
    // Удаляем javascript: URI
    .replace(/javascript\s*:/gi, '')
    .replace(/data\s*:\s*text\/html/gi, '')
    // Декодируем HTML-сущности и удаляем потенциально опасные
    .replace(/&[#\w]+;/g, (match) => {
      // Разрешаем только безопасные сущности
      const safeEntities = [
        '\x26amp;',
        '\x26lt;',
        '\x26gt;',
        '\x26quot;',
        '\x26#39;',
        '\x26nbsp;'
      ]
      return safeEntities.indexOf(match) !== -1 ? match : ''
    })
    // Удаляем нулевые байты
    .replace(/\0/g, '')
    // Удаляем управляющие символы (кроме \n, \r, \t)
    .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]/g, '')
    .trim()
    .slice(0, 1000)
}

export function validateMagnetURI(uri: string): boolean {
  return uri.startsWith('magnet:?xt=urn:btih:')
}
