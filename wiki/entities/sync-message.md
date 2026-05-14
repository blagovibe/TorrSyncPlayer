---
title: SyncMessage
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [types, protocol, sync]
sources: [raw/articles/types-source.md]
---

# SyncMessage

Тип данных для синхронизации воспроизведения между хостом и гостем.

## Определение

```typescript
interface SyncMessage {
  action: "play" | "pause" | "seek" | "state";
  position: number;        // текущая позиция в секундах
  server_ts: number;       // timestamp отправки (Date.now())
  is_playing?: boolean;    // воспроизводится ли в данный момент
  sourceKey?: string;      // ключ источника торрента
}
```

## Поля

| Поле | Тип | Описание |
|---|---|---|
| `action` | `SyncAction` | Тип действия: play, pause, seek, state |
| `position` | `number` | Позиция воспроизведения в секундах |
| `server_ts` | `number` | Unix timestamp миллисекунд момента отправки |
| `is_playing` | `boolean?` | Флаг воспроизведения (обязателен для state) |
| `sourceKey` | `string?` | Идентификатор торрент-источника для валидации |

## Использование

- **play** — отправляется при начале воспроизведения
- **pause** — отправляется при паузе
- **seek** — отправляется при перемотке (без latency compensation на slave)
- **state** — heartbeat, отправляется каждую секунду

## Связи

- [[SyncService]] — создаёт и применяет SyncMessage
- [[P2PService]] — транспортирует SyncMessage через data channel
- [[App]] — обогащает sourceKey перед broadcast
