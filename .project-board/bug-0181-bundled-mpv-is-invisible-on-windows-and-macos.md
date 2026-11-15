---
id: bug-0181
type: bug
status: open
title: Windows/macOS: в релизном комплекте лежит mpv, а приложение его не видит
refs:
  - "DEV.md#mpv как внешний процесс"
created: 2026-11-15
---

## Суть

Все три релизных артефакта несут mpv внутри: AppImage — `usr/bin/mpv`
(Linux; дальше работает, потому что `AppRun` дописывает `$APPDIR/usr/bin`
в PATH перед запуском), Windows — `mpv.exe` рядом с `TorrSyncPlayer.exe`,
macOS — `Contents/MacOS/mpv` рядом с бандл-бинарем. В то же время
`mpvExecutable` (`frontend/internal/ui/window.go`, `mpvCandidates`) смотрит
только: настроенное значение → PATH → «общеизвестные места» (Program Files,
chocolatey, LocalAppData; Homebrew, /usr/local, /Applications/mpv.app) —
и никогда **бандл-относительное** расположение. Итог: пользователь Windows и
macOS, скачавший официальный релиз, получает подсказку «установите mpv» —
хотя mpv лежит прямо рядом с приложением, а README утверждает «`mpv` уже
внутри, отдельно ставить ничего не нужно». На Linux обещание держится
только благодаря `AppRun`.

## Обсуждение

2026-11-15: Поиск задачи пользователем: «не нравится, что нужно внешне
устанавливать mpv». Проверено фактами: README line «mpv уже внутри…» против
кода `mpvExecutableWithCandidates` (ни одной бандл-относительной кандидатуры)
и против `release.yml` (все три платформы паковали mpv внутри задолго до
этого бага). Отличать от idea-0182 (libmpv в процессе) — это про поиск
внешнего бинарника, не про архитектуру воспроизведения.
