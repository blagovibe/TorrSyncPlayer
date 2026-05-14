---
title: VideoPlayer
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [component, frontend, video, audio]
sources: [raw/articles/videoplayer-source.md]
---

# VideoPlayer

React-компонент видеоплеера с кастомными контролами. Поддерживает видео и аудио треки, fallback audio, настройки буферизации.

## Ответственности

- Воспроизведение видео/аудио с кастомными UI-контролами
- Управление аудио-треками (native `audioTracks` API или fallback)
- Настройка масштабирования видео (fit/fill/stretch/original)
- Настройка параметров буферизации
- Авто-скрытие контролов через 3 секунды

## Режимы масштабирования

| Режим | Описание |
|---|---|
| `fit` | Вписать без обрезки (по умолчанию) |
| `fill` | Заполнить контейнер с обрезкой |
| `stretch` | Растянуть до размеров контейнера |
| `original` | Исходный размер, по центру |

Сохраняется в `localStorage` (`torrsyncplayer.videoScale`).

## Аудио треки

### Native audioTracks API
Используется если `"audioTracks" in document.createElement("video")`. Переключение через свойство `track.enabled`.

### Fallback audio
Если native API недоступен и есть `resolveFallbackAudioTrackSource`:
- Создаётся отдельный `<audio>` элемент
- Аудио синхронизируется с видео по `currentTime`
- При смене трека — загружается новый source URL
- Громкость применяется к fallback audio, видео при этом `volume=0`

## Буферизация

Настройки передаются в [[TorrentService]]:
- `bufferWindowMB` — окно приоритета (по умолчанию 50MB)
- `maxBufferMB` — максимальный буфер (по умолчанию 500MB)

## Связи

- [[App]] — передаёт props и callback'и
- [[RoomPage]] — использует VideoPlayer
- [[TorrentService]] — стриминг и буферизация
- [[SyncService]] — play/pause/seek вызывают sync
