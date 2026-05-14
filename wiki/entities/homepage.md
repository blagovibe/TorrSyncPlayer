---
title: HomePage
created: 2026-05-15
updated: 2026-05-15
type: entity
tags: [component, frontend]
sources: [raw/articles/homepage-source.md]
---

# HomePage

Экран создания и подключения к комнате. Стартовый экран приложения.

## Ответственности

- Отображение peer ID хоста с возможностью копирования
- Форма ввода room code для подключения гостем
- Кнопка "Create Room (Host)"

## UI элементы

- Логотип "TorrSyncPlayer"
- Peer ID display + кнопка "Copy"
- Поле ввода room code (6 символов, авто-uppercase)
- Кнопка "Connect to Friend"
- Отображение ошибок подключения

## Связи

- [[App]] — родительский компонент, передаёт `peerId`, `onCreateRoom`, `onJoinRoom`
