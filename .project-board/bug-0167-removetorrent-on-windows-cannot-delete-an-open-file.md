---
id: bug-0167
type: bug
status: open
title: Windows: RemoveTorrent не удаляет файлы с диска — тест краснеет на очистке TempDir
refs:
  - "DEV.md#Соглашения по тестам"
  - "DEV.md#Проверки должны уметь краснеть"
created: 2026-11-14
---

## Суть

`TestAPartialTorrentIsPlayableAndWhatIsPlayingIsFetchedFirst` (backend,
`internal/torrent/real_swarm_test.go`) краснеет на Windows при полном прогоне
пакета и зеленеет при точечном запуске — флейк, зависящий от тайминга:

- `RemoveTorrent` логирует `WARN Torrent: failed to remove disk storage for
  torrent` с `unlinkat ...\seed\film.mkv: The process cannot access the file
  because it is being used by another process` — anacrolix держит файл
  открытым, удаление не происходит;
- сам тест при этом проходит (`--- PASS`), но `t.TempDir()` не может очистить
  каталог (`testing.go:1619: TempDir RemoveAll cleanup`) и делает его
  красным.

Linux/macOS этого ограничения нет (там unlink открытого файла разрешён), CI на
Windows-раннере гоняет тот же путь. Заметно на медленных/загруженных машинах.

Второе, рядом: прогон `internal/cicontract` на этой машине оставил в каталоге
пакета четыре файла-«attempts» с именами вида `C:Users…LocalTemp…` (двоеточия
и обратные слэши имени temp-каталога пережили склейку в имя) — тест уносит
файлы в CWD вместо `t.TempDir()`. Тоже Windows-специфика, тоже чинится в тесте.

## Обсуждение

2026-11-14: Найдено свежей сверкой (граница — e1fa3f9 + незакоммиченные
изменения): полный прогон `go test ./...` в backend дважды из четырёх прогонов
краснел `internal/torrent`; точечный запуск зелёный. Проверено чтением
`real_swarm_test.go:135–145` и лога прогона. Лечение: перед `RemoveTorrent`
дождаться освобождения файлов клиентом (drop + ожидание закрытия стораджа в
anacrolix) либо удалять файлы теста с ретраем/конечной выдержкой; попутно —
сделать файлы «attempts» в `cicontract` локальными для `t.TempDir()`. Тест,
который без правки остаётся красным на Windows, — обязателен.

2026-11-14 (дополнение): на этой машине класс шире — красным при полном
прогоне детерминированно падает ещё `internal/persistence: TestAtomicWrites`
(ожидает 0600, Windows субъектирует права иначе) и три `internal/storage:
TestRemovingATorrent*` (`unlinkat ...\.torrent.bolt.db: The process cannot
access the file because it is being used by another process` — тот же
anacrolix/bolt-хэндл, что и у `internal/torrent`). Проверено на 569282a —
те же падения до pact-go мерджа, то есть это не регрессия, а давно
невидимое: backend-джоба в CI идёт только на ubuntu, поэтому на Linux всё
зелёное. В лечение карточки входит и сведение Windows-личных проверок к
explicit skip-with-reason либо к Windows-совместимым проверкам, и то, что
backend-набор гоняется хотя бы на одном Windows (или дефект задокументирован
как "backend targets Linux").
