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

2026-10-14: **Посылка изменилась** (idea-0067: Qt снят, фронтенд на Go/Fyne).
Пользователю теперь не надо ставить Qt6, libmpv, CMake и ninja: нужен Go,
заголовки OpenGL/X11 и отдельный `mpv`. В AppImage соответственно уезжает mpv
и cgo-зависимости Fyne, а не Qt6 с libmpv. Суть карточки (нужны AppImage и
Winget, сейчас только сборка из исходников) не изменилась — изменился состав
того, что надо упаковать. Переписать раздел с зависимостями — часть работы
этой карточки, не молчаливая правка.

