---
title: App
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [component, frontend, architecture]
sources: [raw/articles/app-source.md]
---

# App

Корневой React-компонент приложения. Управляет состоянием, маршрутацией между экранами и координирует все сервисы.

## Ответственности

- Управление состоянием приложения (view, peer role, torrent, playback)
- Маршрутация: `home` → `room`
- Создание и координация [[P2PService]], [[SyncService]], [[TorrentService]]
- Обработка входящих P2P-сообщений (sync, torrent_source, room_config)
- Очередь загрузки торрентов (`pendingTorrentLoadRef`)
- Broadcast состояния комнаты новым гостям

## Состояние

Ключевые переменные состояния:
- `currentView`: `"home"` | `"room"`
- `peerRole`: `"master"` | `"slave"` | `null`
- `isConnected`, `isConnecting`, `connectionError`
- `magnetLink`, `torrentFile`
- `mediaFiles`, `selectedMediaIndex`, `selectedMediaLabel`, `selectedMediaKind`
- `syncToleranceSeconds` (по умолчанию 0.5)
- `bufferWindowMB` (по умолчанию 50), `maxBufferMB` (по умолчанию 500)

## Очередь загрузки торрентов

Используется `pendingTorrentLoadRef` + `isProcessingTorrentLoadRef` для последовательной обработки запросов. Новый запрос заменяет предыдущий (если ещё не начал обрабатываться). После завершения — проверяет очередь на наличие следующего.

## Broadcast состояния

При подключении нового гостя (или по таймауту 500мс) host отправляет:
1. Текущий torrent source (`sendTorrentSource`)
2. Конфигурацию комнаты (`sendRoomConfig`)
3. Снимок воспроизведения (`sendSync`)

## Обработка remote sync на slave

Pending remote sync применяется только когда:
- Роль = slave
- Плеер готов (`isPlayerReady`)
- Не идёт загрузка торрента
- Выбран медиафайл
- Совпадает `sourceKey` (или не задан)

## Связи

- [[HomePage]] — экран создания/подключения к комнате
- [[RoomPage]] — экран комнаты с видеоплеером
- [[P2PService]] — P2P-соединение
- [[SyncService]] — синхронизация воспроизведения
- [[TorrentService]] — управление торрентами
- [[VideoPlayer]] — видеоплеер
