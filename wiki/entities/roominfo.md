---
title: RoomInfo
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [component, frontend]
sources: [raw/articles/roominfo-source.md]
---

# RoomInfo

Компонент информации о комнате. Отображает peer ID, роль, список пиров и кнопку выхода.

## Ответственности

- Отображение peer ID с кнопкой копирования
- Отображение роли (Host/Guest)
- Список подключённых пиров
- Кнопка "Leave Room"

## Связи

- [[RoomPage]] — родительский компонент
- [[App]] — передаёт данные о пирах
