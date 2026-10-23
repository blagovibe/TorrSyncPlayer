---
id: bug-0155
type: bug
status: open
title: Mesh.WaitFor выбрасывает несовпавшие сообщения и не знает типа peers
refs:
  - "DEV.md#Сообщения комнаты"
created: 2026-10-23
---

## Суть

Комментарий `Mesh.WaitFor` обещает: «Messages of other types stay queued for
later WaitFor calls» (`frontend/internal/rtc/mesh.go:507–509`). Код читает из
общего `m.incoming` и всё, что не совпало по типу, молча теряет
(`mesh.go:510–526`): сообщение, вынутое из канала, никуда не возвращается.

Рядом — вторая половина: `messageType` (`frontend/internal/rtc/protocol.go`)
знает `magnet`, `pause`, `position`, `buffering`, `hello`, `host`, `bye`, но не
`peers`, поэтому `WaitFor(TypePeers…)` не сработает никогда, а пришедшая
адресная книга будет выброшена.

Продовый драйвер ходит через `Next`, поэтому дефект пока не проявляется в
приложении; но тесты и будущий код опираются на `WaitFor` и на его обещание.

## Обсуждение

2026-10-23: Найдено аудитом. Проверено чтением `mesh.go:494–526` и
`protocol.go` (`messageType`). Лечение: очередь «отложенных» сообщений вместо
чтения общего канала, `case Peers` в `messageType`, тест «сообщение другого типа
не теряется».
