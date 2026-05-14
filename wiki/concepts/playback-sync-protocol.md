---
title: Playback Synchronization Protocol
created: 2026-05-15
updated: 2026-05-15
type: concept
tags: [sync, protocol, p2p, architecture]
sources: [raw/articles/sync-service-source.md, raw/articles/p2p-service-source.md]
---

# Playback Synchronization Protocol

Протокол синхронизации воспроизведения между хостом (master) и гостем (slave).

## Архитектура

```
[Master] → SyncMessage → [P2PService data channel] → [Slave]
                                                          ↓
                                                   SyncService.applyRemoteSync()
                                                          ↓
                                                   video.currentTime = compensated
```

## Типы сообщений

| Action | Когда отправляется | Latency compensation |
|---|---|---|
| `play` | Начало воспроизведения | Да |
| `pause` | Пауза | Да |
| `seek` | Перемотка | **Нет** — мгновенный переход |
| `state` | Heartbeat (каждую секунду) | Да |

## Latency Compensation

```
latencySeconds = (Date.now() - message.server_ts) / 1000
compensatedPosition = message.position + min(max(latencySeconds, 0), 5)
```

Ограничение: 0–5 секунд. Для `seek` компенсация не применяется — хост уже вычислил целевую позицию.

## Sync Tolerance

Если `|localTime - compensatedPosition| < syncToleranceSeconds` — коррекция не применяется. По умолчанию 0.5с. Настраивается хостом в UI.

## Подавление эхо-событий

При программном вызове `play()`/`pause()`/`seek()` устанавливается флаг `suppressNextEventSync`, чтобы событие от `<video>` элемента не вызвало повторную отправку. Heartbeat сбрасывает флаги, если событие не произошло.

## Безопасное воспроизведение

`safePlay()` проверяет `video.readyState >= 2` (HAVE_CURRENT_DATA). Если данных недостаточно — браузер автоматически воспроизведёт при появлении данных.

## Source Key Validation

Slave применяет sync только если `message.sourceKey` совпадает с текущим источником. Это предотвращает применение sync от предыдущего торрента.

## Связи

- [[SyncService]] — реализация протокола
- [[P2PService]] — транспорт
- [[SyncMessage]] — тип данных
- [[App]] — координация
