---
title: P2PService
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [service, p2p, webrtc, signaling]
sources: [raw/articles/p2p-service-source.md]
---

# P2PService

Сервис пирингового соединения через PeerJS/WebRTC. Управляет жизненным циклом P2P-соединения между хостом и гостем.

## Ответственности

- Инициализация PeerJS peer с ролью `host` или `guest`
- Управление исходящими и входящими `DataConnection`
- Отправка и приём сообщений трёх типов: `sync`, `torrent_source`, `room_config`
- Обработка ошибок WebRTC и сигналинга
- Генерация случального peer ID (6 символов, буквы A-Z и цифры)

## Ключевые методы

| Метод | Описание |
|---|---|
| `initialize()` | Создаёт PeerJS peer, проверяет WebRTC availability, подписывается на события |
| `connect(remotePeerId)` | Устанавливает исходящее соединение с таймаутом 30с |
| `sendSync(message)` | Отправляет sync-сообщение через data channel |
| `sendTorrentSource(payload)` | Отправляет информацию о торрент-источнике |
| `sendRoomConfig(payload)` | Отправляет конфигурацию комнаты (sync tolerance) |
| `disconnect()` | Закрывает все соединения, уничтожает peer, очищает слушателей |
| `on(event, callback)` | Подписка на события с возвратом функции отписки |

## Типы сообщений

- **sync** — состояние воспроизведения (play/pause/seek/state)
- **torrent_source** — источник торрента (magnet или файл) + выбранный медиафайл
- **room_config** — настройки синхронизации (sync tolerance)

## Роли

- **Host** — peer ID генерируется с префиксом `torrsync-`, принимает входящие соединения
- **Guest** — подключается к host по room code (peer ID хоста)

## Конфигурация сервера сигналинга

Настраивается через env-переменные Vite:
- `VITE_PEERJS_HOST` — хост (по умолчанию `0.peerjs.com`)
- `VITE_PEERJS_PORT` — порт (по умолчанию 443)
- `VITE_PEERJS_PATH` — путь (по умолчанию `/`)
- `VITE_PEERJS_SECURE` — использовать WSS

## Связи

- [[SyncService]] — получает sync-сообщения от P2PService через событие `sync`
- [[App]] — создаёт и управляет P2PService, подписывается на все события
- [[SyncMessage]] — тип данных для sync-сообщений
- [[SharedTorrentSource]] — тип данных для торрент-источника
- [[RoomConfigMessage]] — тип данных для конфигурации комнаты

## Известные ограничения

- При потере соединения с сигналинг-сервером peer не переподключается автоматически
- Максимум одно активное соединение (один host ↔ один guest)
