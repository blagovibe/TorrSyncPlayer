---
title: RoomConfigMessage
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [types, protocol, sync]
sources: [raw/articles/types-source.md]
---

# RoomConfigMessage

Тип данных для конфигурации комнаты.

## Определение

```typescript
interface RoomConfigMessage {
  syncToleranceSeconds: number;
}
```

## Поля

| Поле | Тип | Описание |
|---|---|---|
| `syncToleranceSeconds` | `number` | Допустимое расхождение времени между хостом и гостем (по умолчанию 0.5с) |

## Использование

Отправляется хостом при подкении нового гостя и при изменении настройки в UI. Гость применяет значение через `SyncService.setSyncToleranceSeconds()`.

## Связи

- [[P2PService]] — транспортирует через `sendRoomConfig`
- [[SyncService]] — использует значение для принятия решения о коррекции
- [[RoomPage]] — UI для изменения значения
