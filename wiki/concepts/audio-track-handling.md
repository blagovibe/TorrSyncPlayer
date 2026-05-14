---
title: Audio Track Handling
created: 2026-05-15
updated: 2026-05-15
type: concept
tags: [audio, video, frontend]
sources: [raw/articles/videoplayer-source.md]
---

# Audio Track Handling

Поддержка нескольких аудио-треков в видеофайлах с fallback для браузеров без native API.

## Два режима

### 1. Native audioTracks API

Используется если `"audioTracks" in document.createElement("video")`:
- Переключение через `track.enabled = true/false`
- Синхронизация с видео автоматическая
- Нет дополнительных элементов DOM

### 2. Fallback Audio

Для браузеров без native API (например, некоторые WebView):
- Отдельный `<audio>` элемент синхронизируется с `<video>`
- `video.volume = 0` (звук идёт только через audio)
- При смене трека — загрузка нового source URL через `resolveFallbackAudioTrackSource`
- Синхронизация по `currentTime` при загрузке нового source

## Ограничения

- Fallback audio требует поддержки `probeAudioTracks` и `createAudioTrackStreamUrl` от Electron backend
- В браузерном режиме fallback недоступен — только native API
- Переключение трека в fallback режиме вызывает короткую паузу (загрузка нового URL)

## Связи

- [[VideoPlayer]] — UI и логика переключения
- [[TorrentService]] — `probeAudioTracks`, `createAudioTrackStreamUrl`
- [[TorrentMediaFile]] — содержит информацию о файле
