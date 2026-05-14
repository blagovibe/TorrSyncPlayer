---
title: TorrentMediaFile
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [types, torrent, media]
sources: [raw/articles/types-source.md]
---

# TorrentMediaFile

Тип данных для описания воспроизводимого медиафайла внутри торрента.

## Определение

```typescript
interface TorrentMediaFile {
  index: number;           // индекс файла в торренте
  name: string;            // имя файла
  length: number;          // размер в байтах
  kind: "video" | "audio"; // тип медиа
  extension: string;       // расширение файла (например, ".mp4")
  file: TorrentFile;       // ссылка на оригинальный файл WebTorrent
}
```

## Поддерживаемые форматы

**Видео:** mp4, mkv, webm, mov, avi, m4v, ts, ogv

**Аудио:** mp3, m4a, aac, flac, ogg, opus, wav, oga, wma

## Связи

- [[TorrentService]] — создаёт TorrentMediaFile из файлов торрента
- [[VideoPlayer]] — отображает информацию о медиафайле
- [[RoomPage]] — показывает список медиафайлов
