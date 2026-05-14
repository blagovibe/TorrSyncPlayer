---
title: format utility
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [utility, frontend]
sources: [raw/articles/format-source.md]
---

# format utility

Утилита форматирования данных.

## Функции

| Функция | Описание |
|---|---|
| `formatBytes(bytes)` | Форматирует размер в байтах в читаемый вид (KB, MB, GB) |
| `formatSpeed(bytesPerSecond)` | Форматирует скорость загрузки (B/s, KB/s, MB/s) |

## Связи

- [[TorrentService]] — использует `formatBytes` для отображения размера файла
- [[StatusBar]] — использует `formatSpeed` для отображения скорости
