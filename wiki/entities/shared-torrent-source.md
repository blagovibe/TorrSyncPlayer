---
title: SharedTorrentSource
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [types, protocol, torrent]
sources: [raw/articles/types-source.md]
---

# SharedTorrentSource

Тип данных для описания торрент-источника, передаваемого между хостом и гостем.

## Определение

```typescript
type SharedTorrentSource =
  | {
      kind: "magnet";
      magnetLink: string;
      sourceKey: string;
    }
  | {
      kind: "file";
      fileName: string;
      bytes: number[];       // Uint8Array сериализован в массив
      sourceKey: string;
    };
```

## Варианты

| Kind | Описание | Поля |
|---|---|---|
| `magnet` | Magnet-ссылка | `magnetLink` — полная magnet URI |
| `file` | Загруженный `.torrent` файл | `fileName` — имя файла, `bytes` — содержимое |

## sourceKey

Уникальный идентификатор источника:
- Для magnet: `"magnet:" + magnetLink`
- Для file: `"file:" + fileName + ":" + bytes.length + ":" + hash(bytes)`

Используется для дедупликации и валидации (slave проверяет совпадение sourceKey перед применением sync).

## Связи

- [[TorrentService]] — создаёт и обрабатывает SharedTorrentSource
- [[P2PService]] — передаёт через `sendTorrentSource`
- [[App]] — создаёт source из magnet/torrent file
