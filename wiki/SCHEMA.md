# Wiki Schema

## Domain

**TorrSyncPlayer** — десктопное приложение для потокового воспроизведения видео через WebTorrent с P2P-синхронизацией между устройствами. Включает frontend (React + TypeScript), desktop runtime (Electron), протокол сигналинга (PeerJS/WebRTC) и сервисы синхронизации.

## Conventions

- File names: lowercase, hyphens, no spaces (e.g., `p2p-service.md`)
- Every wiki page starts with YAML frontmatter (see below)
- Use `[[wikilinks]]` to link between pages (minimum 2 outbound links per page)
- When updating a page, always bump the `updated` date
- Every new page must be added to `index.md` under the correct section
- Every action must be appended to `log.md`

## Frontmatter

```yaml
---
title: Page Title
created: YYYY-MM-DD
updated: YYYY-MM-DD
type: entity | concept | comparison | query | summary
tags: [from taxonomy below]
sources: [raw/articles/source-name.md]
---
```

## Tag Taxonomy

### Architecture
- `frontend` — React, TypeScript, Vite, CSS
- `desktop` — Electron
- `service` — сервисный слой (P2P, Sync, Torrent)
- `component` — React-компоненты
- `utility` — утилиты и хелперы

### P2P / Networking
- `p2p` — peer-to-peer взаимодействие
- `webrtc` — WebRTC data channels
- `signaling` — сигналинг (PeerJS, WebSocket)
- `sync` — синхронизация воспроизведения

### Media
- `torrent` — WebTorrent, magnet-ссылки, торрент-файлы
- `streaming` — потоковое воспроизведение
- `video` — видео воспроизведение и контролы
- `audio` — аудио треки и fallback audio
- `buffer` — буферизация и приоритизация кусков

### Data Types
- `types` — TypeScript типы и интерфейсы
- `protocol` — протоколы обмена сообщениями

### Meta
- `architecture` — архитектурные решения
- `config` — конфигурация и настройки
- `testing` — тесты
- `known-issue` — известные проблемы

Rule: every tag on a page must appear in this taxonomy. If a new tag is needed,
add it here first, then use it.

## Page Thresholds

- **Create a page** when an entity/concept appears in 2+ sources OR is central to one source
- **Add to existing page** when a source mentions something already covered
- **DON'T create a page** for passing mentions, minor details, or things outside the domain
- **Split a page** when it exceeds ~200 lines
- **Archive a page** when its content is fully superseded — move to `_archive/`

## Entity Pages

One page per notable entity (service, component, type). Include:
- Overview / what it is
- Key responsibilities and methods
- Relationships to other entities ([[wikilinks]])
- Source references

## Concept Pages

One page per concept or topic. Include:
- Definition / explanation
- Current state of knowledge
- Open questions or debates
- Related concepts ([[wikilinks]])

## Update Policy

When new information conflicts with existing content:
1. Check the dates — newer sources generally supersede older ones
2. If genuinely contradictory, note both positions with dates and sources
3. Flag for user review in the lint report
