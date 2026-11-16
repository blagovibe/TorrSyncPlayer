# Ряд измерений: расхождение синхронизации

Здесь живёт ряд замеров расхождения синхронизации: одна строка — один прогон
измерения `TestTheRoomKeepsEveryoneInTheSameSecond`, который гоняет двух
настоящих участников с настоящим файлом и настоящей комнатой поверх
настоящего libp2p-потока. Джоба `sync-measurement` дописывает строку сама и
закладывает её в `main`; руками ничего править не нужно.
**Правило ряда:** не меньше 90% из 20 последних прогонов держат расхождение в
пределе обещания — **одной секунды** (это число цели, «та же секунда»).
Ниже порога — джоба пишет карточку на доске, потому что порог, который никто
не проверяет, мёртв.
Ряд — это не гейт: гейтом является сама проверка внутри измерения, и она
краснеет, когда расхождение выходит за предел. Ряд отвечает на вопрос,
который один прогон не отвечает: **не растёт ли расхождение со временем.**
Пока цифра держится у нуля, всё в порядке; когда она ползёт к пределу, об
этом должен узнать человек, а не тот, кто через полгода откроет лог джобы.
Каждая строка — один прогон: расхождение в установившемся режиме, за сколько
перемотка встала в комнате, и итог относительно обещания.
| Когда | Расхождение (с) | Перемотка встала за (с) | Итог |
|---|---|---|---|
| 2026-10-21T01:48:49Z | 0.20 | 0.61 | within the promise |
| 2026-10-21T02:01:25Z | 0.20 | 0.58 | within the promise |
| 2026-10-21T02:27:53Z | 0.20 | 0.57 | within the promise |
| 2026-10-21T02:38:34Z | 0.00 | 0.56 | within the promise |
| 2026-10-21T02:47:45Z | 0.20 | 0.58 | within the promise |
| 2026-10-21T16:05:08Z | 0.20 | 0.59 | within the promise |
| 2026-10-21T16:26:04Z | 0.20 | 0.56 | within the promise |
| 2026-10-21T16:57:26Z | 0.20 | 0.59 | within the promise |
| 2026-10-21T17:39:48Z | 0.20 | 0.58 | within the promise |
| 2026-10-21T17:54:39Z | 0.20 | 0.59 | within the promise |
| 2026-10-21T18:08:55Z | 0.20 | 0.58 | within the promise |
| 2026-10-21T18:31:01Z | 0.20 | 0.57 | within the promise |
| 2026-10-21T18:45:53Z | 0.00 | 0.58 | within the promise |
| 2026-10-21T20:03:05Z | 0.20 | 0.56 | within the promise |
| 2026-10-21T20:17:04Z | 0.20 | 0.58 | within the promise |
| 2026-11-13T21:48:21Z | 0.20 | 0.57 | within the promise |
| 2026-11-13T23:02:38Z | 0.20 | 0.59 | within the promise |
| 2026-11-13T23:19:51Z | ? | ? | outside the promise |
| 2026-11-13T23:34:03Z | 0.00 | 0.00 | within the promise |
| 2026-11-13T23:49:02Z | 0.20 | 0.55 | within the promise |
| 2026-11-14T00:06:10Z | 0.20 | 0.57 | within the promise |
| 2026-11-14T10:41:21Z | 0.20 | 0.57 | within the promise |
| 2026-11-14T10:51:45Z | 0.20 | 0.56 | within the promise |
| 2026-11-14T13:48:56Z | 0.20 | 0.56 | within the promise |
| 2026-11-14T14:07:42Z | 0.00 | 0.58 | within the promise |
| 2026-11-14T14:22:54Z | 0.20 | 0.57 | within the promise |
| 2026-11-14T14:42:20Z | 0.20 | 0.57 | within the promise |
| 2026-11-14T15:13:27Z | 0.20 | 0.56 | within the promise |
| 2026-11-14T15:31:57Z | 0.20 | 0.57 | within the promise |
| 2026-11-14T15:38:04Z | 0.20 | 0.55 | within the promise |
| 2026-11-14T15:59:16Z | 0.20 | 0.56 | within the promise |
| 2026-11-14T18:03:51Z | 0.20 | 0.58 | within the promise |
| 2026-11-14T18:14:43Z | 0.20 | 0.58 | within the promise |
| 2026-11-14T18:21:30Z | 0.20 | 0.58 | within the promise |
| 2026-11-14T18:34:35Z | 0.20 | 0.58 | within the promise |
| 2026-11-14T18:50:08Z | 0.20 | 0.58 | within the promise |
| 2026-11-14T18:58:59Z | 0.20 | 0.57 | within the promise |
| 2026-11-14T19:11:22Z | 0.00 | 0.56 | within the promise |
| 2026-11-14T19:23:00Z | 0.20 | 0.57 | within the promise |
| 2026-11-14T19:35:50Z | 0.20 | 0.58 | within the promise |
| 2026-11-14T21:32:42Z | 0.00 | 0.56 | within the promise |
| 2026-11-14T21:43:25Z | 0.20 | 0.57 | within the promise |
| 2026-11-14T21:53:12Z | 0.20 | 0.58 | within the promise |
| 2026-11-14T22:08:03Z | 0.20 | 0.57 | within the promise |
| 2026-11-14T22:22:11Z | 0.00 | 0.58 | within the promise |
| 2026-11-15T01:32:30Z | 0.00 | 0.58 | within the promise |
| 2026-11-15T01:42:39Z | 0.00 | 0.58 | within the promise |
| 2026-11-15T01:50:40Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T02:01:01Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T02:41:06Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T10:05:13Z | 0.20 | 0.56 | within the promise |
| 2026-11-15T10:54:42Z | 0.20 | 0.59 | within the promise |
| 2026-11-15T11:01:03Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T11:17:58Z | 0.00 | 0.60 | within the promise |
| 2026-11-15T13:02:28Z | 0.20 | 0.54 | within the promise |
| 2026-11-15T13:11:18Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T13:19:47Z | 0.20 | 0.56 | within the promise |
| 2026-11-15T13:40:52Z | 0.20 | 0.59 | within the promise |
| 2026-11-15T14:22:37Z | 0.20 | 0.56 | within the promise |
| 2026-11-15T14:36:01Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T14:43:58Z | 0.20 | 0.59 | within the promise |
| 2026-11-15T15:29:44Z | 0.20 | 0.57 | within the promise |
| 2026-11-15T16:14:07Z | 0.00 | 0.58 | within the promise |
| 2026-11-15T16:23:26Z | 0.20 | 0.57 | within the promise |
| 2026-11-15T16:32:09Z | 0.20 | 0.56 | within the promise |
| 2026-11-15T16:56:17Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T17:28:28Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T19:30:05Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T19:46:54Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T19:55:09Z | 0.20 | 0.56 | within the promise |
| 2026-11-15T20:03:20Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T20:10:56Z | 0.20 | 0.56 | within the promise |
| 2026-11-15T20:18:46Z | 0.20 | 0.55 | within the promise |
| 2026-11-15T20:26:01Z | 0.20 | 0.55 | within the promise |
| 2026-11-15T20:50:34Z | 0.00 | 0.58 | within the promise |
| 2026-11-15T21:18:15Z | 0.20 | 0.56 | within the promise |
| 2026-11-15T21:28:36Z | 0.20 | 0.58 | within the promise |
| 2026-11-15T21:53:02Z | 0.20 | 0.56 | within the promise |
| 2026-11-15T22:15:50Z | 0.20 | 0.57 | within the promise |
| 2026-11-15T23:03:45Z | 0.00 | 0.59 | within the promise |
| 2026-11-16T06:23:13Z | 0.20 | 0.55 | within the promise |
| 2026-11-16T06:34:17Z | 0.20 | 0.58 | within the promise |
| 2026-11-16T06:43:27Z | 0.20 | 0.56 | within the promise |
| 2026-11-16T06:53:13Z | 0.20 | 0.57 | within the promise |
| 2026-11-16T07:23:11Z | 0.20 | 0.59 | within the promise |
| 2026-11-16T07:35:54Z | 0.20 | 0.58 | within the promise |
| 2026-11-16T07:43:38Z | ? | ? | outside the promise |
| 2026-11-16T07:54:09Z | 0.00 | 0.58 | within the promise |
| 2026-11-16T08:00:37Z | ? | ? | outside the promise |
| 2026-11-16T08:42:51Z | 0.20 | 0.58 | within the promise |
