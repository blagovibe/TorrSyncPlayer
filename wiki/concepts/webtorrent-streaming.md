---
title: WebTorrent Streaming
created: 2026-05-15
updated: 2026-05-15
type: concept
tags: [torrent, streaming, p2p, architecture]
sources: [raw/articles/torrent-service-source.md, raw/articles/spec-source.md]
---

# WebTorrent Streaming

Потоковое воспроизведение видео через WebTorrent — ключевая функция TorrSyncPlayer.

## Обзор

WebTorrent — это реализация BitTorrent протокола для браузера, использующая WebRTC вместо TCP/UDP. TorrSyncPlayer использует два бэкенда:

1. **WebTorrent (браузер)** — работает в renderer process Electron
2. **Electron native backend** — нативный торрент-клиент в main process Electron, доступный через `window.torrsyncElectronTorrent`

## Процесс стриминга

```
Magnet/Torrent → TorrentService.addMagnet/addTorrentFile
  → Получение метаданных (metadata event)
  → Определение воспроизводимых файлов (getPlayableMediaFiles)
  → Выбор лучшего файла (getPreferredMediaFile)
  → streamToMedia(file, videoElement)
    → streamUrl (Electron) или file.streamTo() (WebTorrent)
    → fallback: blob + object URL
```

## Piece Prioritization

Для плавного воспроизведения загружаются куски вокруг текущей позиции:
- **Buffer window**: ±50MB от позиции воспроизведения
- **Max buffer**: 500MB общий лимит
- **Prioritize interval**: пересчёт каждые 2 секунды

Настройки сохраняются в `localStorage`.

## Поддерживаемые форматы

**Видео:** mp4, mkv, webm, mov, avi, m4v, ts, ogv

**Аудио:** mp3, m4a, aac, flac, ogg, opus, wav, oga, wma

## Связи

- [[TorrentService]] — реализация стриминга
- [[VideoPlayer]] — отображение и контролы
- [[P2PService]] — передача torrent source гостю
