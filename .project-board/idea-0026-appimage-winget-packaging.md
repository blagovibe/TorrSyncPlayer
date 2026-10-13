---
id: idea-0026
type: idea
status: raw
title: Упаковка: AppImage (Linux) + Winget (Windows)
refs:
  - "CONCEPT.md#Границы проекта"
  - "DEV.md#Системные требования"
  - "DEV.md#Сборка"
created: 2026-10-09
---

## Суть

Сейчас пользователь должен: установить Qt6, libmpv, CMake, ninja, собрать фронтенд, собрать бэкенд. Multiplex распространяется через Flathub (авто-апдейты, sandbox).

Предлагаю:
- **Linux**: AppImage (включает Qt6, libmpv, бэкенд) — работает на любом дистре без зависимостей. GitHub Actions → AppImage → GitHub Releases.
- **Windows**: Winget манифест + GitHub Releases (MSIX/EXE инсталлер).
- **macOS**: .dmg (позже, если будет востребовано).

Flatpak — позже (требует манифест, портирование под portal API).

## Обсуждение

2026-10-09: Пользователь выбрал AppImage + Winget. AppImage проще Flatpak (не нужен портал, работает везде). Winget — нативный для Windows.