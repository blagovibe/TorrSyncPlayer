---
title: TorrentService
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [service, torrent, streaming, buffer]
sources: [raw/articles/torrent-service-source.md]
---

# TorrentService

Сервис управления торрентами. Загрузка через WebTorrent (браузер) или Electron backend, стриминг видео, управление буферизацией.

## Ответственности

- Добавление торрентов по magnet-ссылке или `.torrent` файлу
- Определение воспроизводимых медиафайлов (видео/аудио)
- Стриминг медиафайла в `<video>` элемент
- Управление приоритизацией буфера (piece prioritization)
- Подсчёт пиров и трекинг прогресса загрузки
- Probe аудио-треков и создание fallback audio stream URL

## Два бэкенда

| Бэкенд | Среда | API |
|---|---|---|
| **WebTorrent** | Браузер / Electron renderer | `client.add(torrentSource)` |
| **Electron native** | Electron main process через `window.torrsyncElectronTorrent` | `addMagnet()`, `addTorrentFile()` |

Выбор бэкенда автоматический: если `window.torrsyncElectronTorrent` доступен — используется Electron.

## Ключевые методы

| Метод | Описание |
|---|---|
| `addMagnet(magnetLink)` | Добавляет торрент по magnet-ссылке |
| `addTorrentFile(bytes)` | Добавляет торрент из файла |
| `getPlayableMediaFiles(torrent)` | Возвращает отсортированный список медиафайлов |
| `getPreferredMediaFile(torrent)` | Возвращает лучший медиафайл (по формату и размеру) |
| `streamToMedia(file, element)` | Стримит файл в media-элемент |
| `probeAudioTracks(file)` | Получает список аудио-треков (только Electron) |
| `setBufferSettings(windowMB, maxMB)` | Настройка размера буфера |

## Приоритет медиафайлов

Сортировка: видео > аудио. Видео сортируются по совместимости: mp4/m4v (4) > webm (3) > mov/ogv (2) > ts/mkv/avi (0). При равном приоритете — по размеру (большие первые).

## Буферизация

- **Buffer window**: ±50MB от текущей позиции воспроизведения (настраивается)
- **Max buffer**: 500MB общий лимит (настраивается)
- **Prioritize interval**: каждые 2 секунды пересчитывается приоритет кусков
- Настройки сохраняются в `localStorage`

## Стриминг

1. Если есть `file.streamUrl` (Electron backend) — используется напрямую
2. Иначе — `file.streamTo(mediaElement)` (WebTorrent stream server)
3. Если stream server недоступен — fallback на `file.blob()` + `URL.createObjectURL`

## Связи

- [[VideoPlayer]] — вызывает `streamToMedia` и `probeAudioTracks`
- [[App]] — создаёт TorrentService, управляет загрузкой
- [[TorrentMediaFile]] — тип данных медиафайла
- [[P2PService]] — отправляет torrent source гостю через `sendTorrentSource`
