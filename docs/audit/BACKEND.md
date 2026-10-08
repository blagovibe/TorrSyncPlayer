# Аудит Go-бэкенда TorrSyncPlayer

**Область:** `backend/` (Go 1.27.1; 8155 строк production-кода + 5192 строки тестов, 54 `.go`-файла)
**База:** коммит `d6b1a3f`
**Метод:** статический разбор всех 54 файлов + реальный запуск `go build`, `go vet`, `go test`, `go test -race` + исполняемые PoC на каждом P0.

---

## Вердикт

Бэкенд написан аккуратно: типизированные ошибки, `subtle.ConstantTimeCompare`, `http.MaxBytesReader`, per-IP rate limiting, осознанное разделение публичных и защищённых маршрутов, комментарии, объясняющие *почему*, а не *что*. Но аудит вскрыл **три P0-дефекта, каждый из которых ломает одну из двух ключевых возможностей продукта**: (1) подделка stream-ticket'а полностью обходит access token на эндпоинте стриминга; (2) SSE-эндпоинт событий комнаты возвращает 500, потому что middleware-обёртка `responseWriter` не реализует `http.Flusher`; (3) `WriteTimeout = 30s` обрывает **любой** ответ длиннее 30 секунд — то есть видео длиннее полминуты физически невозможно досмотреть. Первый и третий подтверждены исполняемыми PoC. Отдельно стоит подчеркнуть: тест на stream-ticket **дублирует саму сломанную формулу**, а `go test -race ./...` проходит чисто, хотя в `p2p` есть гонка по карте, которую ни один тест не достигает — то есть CI сейчас даёт ложное чувство защищённости. Проект активно поддерживается (fuzz-тесты, pact-контракты, тесты на дедлоки, мутационное покрытие, k6), и именно поэтому набор из трёх P0 — это провал *покрытия*, а не провал процесса. После трёх точечных исправлений в `auth`, `api` и `constants` бэкенд пригоден к релизу.

---

## Build & test reality

### Окружение (важно для повторения)

Кэш сборки Go по умолчанию (`~/.cache/go-build`) в песочнице **read-only** и даёт ошибки вида `read-only file system`, неотличимые от ошибок компиляции. Все запуски выполнены с:

```bash
export GOCACHE=/tmp/tsp-gocache
cd backend
```

**Эти ошибки — артефакт песочницы, а не дефекты кода**, и в находках не фигурируют.

### `go build ./...` — PASS

```
$ go build ./...
# github.com/go-llsqlite/crawshaw/c
sqlite3.c:125517:9: warning: присваивание отменяет квалификатор «const» ... [-Wdiscarded-qualifiers]
sqlite3.c:131536:15: warning: инициализация отменяет квалификатор «const» ... [-Wdiscarded-qualifiers]
BUILD_EXIT=0
```

Единственные предупреждения — из C-кода транзитивной зависимости `go-llsqlite/crawshaw` (cgo). К проекту отношения не имеют.

### `go vet ./...` — PASS

```
$ go vet ./...
VET_EXIT=0
```

### `go test ./...` — PASS

```
$ go test ./...
ok  	.../internal/api           2.530s
ok  	.../internal/auth          0.004s
ok  	.../internal/buffer        0.132s
ok  	.../internal/errors        0.005s
ok  	.../internal/metrics       0.177s
ok  	.../internal/p2p           2.069s
ok  	.../internal/persistence   0.015s
ok  	.../internal/storage       0.006s
ok  	.../internal/sync          0.007s
ok  	.../internal/torrent       0.021s
ok  	.../internal/utils         0.518s
ok  	.../internal/validation    0.005s
ok  	.../internal/version       0.002s
ok  	.../pkg/logger             0.007s
ok  	.../pkg/response           0.003s
?   	.../cmd/server             [no test files]
?   	.../internal/constants     [no test files]
?   	.../internal/models        [no test files]
```

### `go test -race` — PASS

```
$ go test -race -timeout 15m ./internal/p2p/... ./internal/sync/... ./internal/buffer/... \
      ./internal/auth/... ./internal/api/...
ok  	.../internal/p2p           22.470s
ok  	.../internal/sync           1.033s
ok  	.../internal/buffer         1.139s
ok  	.../internal/auth           1.009s
ok  	.../internal/api            6.357s
RACE_EXIT=0
```

**Важная оговорка:** чистый результат `-race` не опровергает находку P1-2. Гонка по карте в `p2p.emitEvent` существует, но ни один тест не вызывает `emitEvent` конкурентно с `JoinRoom`/`LeaveRoom`/`pruneIdlePeers`, поэтому детектор её не видит. Это иллюзия покрытия.

### Вспомогательные проверки

| Проверка | Результат |
|---|---|
| `grep -rn "TODO\|FIXME\|HACK\|XXX" backend/ --include=*.go` | **0 совпадений** |
| `grep -rn "\.Sum(" backend/ --include=*.go` | 3 совпадения, все в `auth` (см. P0-1, P1-1) |
| Поиск всех вычислений HMAC/подписей | `crypto/hmac` встречается **только** в `auth/auth.go` и `auth/auth_test.go` |
| Прочие крипто-вызовы | `torrent/service.go:181` `sha256.Sum256` — не-streaming вариант, используется корректно |
| Мёртвый код после удаления JWT | см. P2-1…P2-5 |

---

## Findings

| ID | Sev | Location | Issue | Fix |
|---|---|---|---|---|
| **P0-1** | P0 | `internal/auth/auth.go:110`, `:139` | MAC не аутентифицирует payload → полный обход access token на `/stream`, TTL не действует | `mac.Write(payload)` + `mac.Sum(nil)` |
| **P0-2** | P0 | `internal/api/middleware.go:517-527` + `handlers.go:147` | `responseWriter` не реализует `http.Flusher` → SSE отдаёт 500 | Добавить `Flush()`/`Unwrap()` |
| **P0-3** | P0 | `cmd/server/main.go:171`, `constants.go:17` | `WriteTimeout=30s` обрывает любой ответ >30s: видео и SSE обрезаются | `WriteTimeout: 0` + `ResponseController.SetWriteDeadline` |
| **P1-1** | P1 | `internal/auth/auth_test.go:157` | Тест копирует ту же сломанную формулу → проверка вакуумна | Независимая реализация в тесте |
| **P1-2** | P1 | `internal/p2p/service.go:708`, `:725`, `:732` | `emitEvent` итерирует `room.Peers` **без блокировки** → гонка / fatal «concurrent map iteration and map write» | Копировать список пиров под `RLock` |
| **P1-3** | P1 | `internal/api/handlers.go:368-372` | Ветка `p2pSvc == nil` — заглушка; p2p не считается упавшим | Заполнить `services` и `allHealthy` |
| **P1-4** | P1 | `internal/api/middleware.go:311-322`, `:377` | Доверие `X-Forwarded-For` от любого приватного IP → обход rate limit + неограниченный рост памяти | Доверять только loopback; ограничить карту |
| **P1-5** | P1 | `internal/api/middleware.go:218-221` | CORS preflight отвечает `200` на любой origin | `403` для неразрешённого origin |
| **P1-6** | P1 | `cmd/server/main.go:246` | `WaitForSSEConnections()` без таймаута → Ctrl-C может висеть до 30 минут | Обернуть в контекст с дедлайном |
| **P1-7** | P1 | `internal/p2p/service.go:652` | `Close()` вне `closeOnce` → второй вызов паникует «close of closed channel» | Перенести всё внутрь `closeOnce.Do` |
| **P1-8** | P1 | `internal/sync/service.go:189`, `:205-210` | Пользовательский seek сглаживается: перемотка на 600s попадает в 180s | Сглаживание только для дрейфа, не для seek |
| **P1-9** | P1 | `internal/p2p/service.go:383`, `:439` | bcrypt cost-12 выполняется под глобальным write-локом p2p (~250ms блокировки всех пиров) | Хешировать вне лока |
| **P2-1** | P2 | `internal/constants/constants.go:128-157` | Мёртвые JWT-константы (0 ссылок) | Удалить |
| **P2-2** | P2 | `internal/models/types.go:147-148` | Мёртвая `models.Claims` | Удалить |
| **P2-3** | P2 | `internal/api/handlers.go.tmp` | Забытый временный файл 429 строк лежит в дереве исходников | Удалить файл |
| **P2-4** | P2 | `internal/api/handlers.go:346`, `handlers_torrent.go:284,286,299` | Устаревшие комментарии про JWT | Переписать |
| **P2-5** | P2 | `internal/validation/validation.go:34-55,101-181` | Целый слой валидации аккаунтов (0 production-ссылок) вопреки AGENTS.md | Удалить |
| **P2-6** | P2 | `internal/torrent/service.go:517,525,538,551` | `http.Error` вместо `WriteError` → не-JSON ответы в API | `WriteError` |
| **P2-7** | P2 | `internal/buffer/service.go:202-207`, `:275-279` | `O(TotalPieces)` приоритетов на каждый апдейт + тикер 1/сек | Не сбрасывать весь массив; инкрементально |
| **P2-8** | P2 | `internal/buffer/service.go:144,246`, `middleware.go:139,267`, `handlers_room.go:247` | Магические числа вне `constants.go` | Вынести в константы (AGENTS.md) |
| **P2-9** | P2 | `internal/p2p/service.go:139,322-343` | `s.sessions` никогда не чистится → неограниченный рост (userID из заголовка) | TTL-очистка сессий |
| **P2-10** | P2 | `internal/p2p/service.go:391-403,511-514` | Комната удаляется только в `LeaveRoom`; создал и не присоединился → утечка слота | Удалять пустые комнаты по TTL |
| **P2-11** | P2 | `internal/p2p/service.go:687-712`, `:634-645` | TOCTOU: `sendToSession` в закрытый канал → паника в горутине отправителя | Не закрывать каналы, либо координировать |
| **P2-12** | P2 | `internal/api/middleware.go:216` | `Allow-Credentials: true` безусловно | Только вместе с `Allow-Origin` |
| **P2-13** | P2 | `cmd/server/main.go:501-533` | pprof без аутентификации; дамп кучи раскрывает токен | Оставить, но задокументировать в разделе Security |
| **P2-14** | P2 | весь `internal/` | `ctx` принимается и игнорируется (`_ = ctx`) во всех сервисах | Убрать параметр или реализовать отмену |
| **P3-1** | P3 | `internal/auth/auth.go:67-69` | Комментарий обещает хеширование, кода нет | Привести в соответствие |
| **P3-2** | P3 | `internal/api/handlers_room.go:60` | «72» зашито в текст ошибки | Форматировать из константы |
| **P3-3** | P3 | `cmd/server/main.go:446-450` | Утечка fd `keyFile` на ветке ошибки `Chmod` | Закрывать оба файла |
| **P3-4** | P3 | `cmd/server/main.go:349` | Неиспользуемый именованный возврат `retErr` | Убрать |
| **P3-5** | P3 | `internal/utils/debouncer.go:56-60` | `fire()` глушит срабатывание при `inFlight != nil` — возможна потеря сохранения | Перевзводить таймер |
| **P3-6** | P3 | `internal/storage/disk.go:42-48` | `RemoveTorrent` доверяет `infoHash` без проверки (валидация только в хендлере) | Валидировать в сервисе |

---

### P0-1 — Подделка stream-ticket'а: полный обход access token

`backend/internal/auth/auth.go:103-112` и `:118-144`. Одна и та же ошибочная строка в обоих:

```go
mac := hmac.New(sha256.New, s.streamTicketKey())
sig := hex.EncodeToString(mac.Sum([]byte(payload)))   // auth.go:110 — ОШИБКА
```

`hash.Hash.Sum(b)` **дописывает** MAC ко всему, что было *записано* в хеш. В `mac` ничего не записывалось — вызова `mac.Write` нет ни разу. Значит вычисляется не MAC от `payload`, а `payload ‖ HMAC(key, "")` — MAC **пустой** строки, константа на всё время жизни процесса. Payload не аутентифицирован, а просто приклеен в начале.

`ValidateStreamTicket` (`:138-139`) повторяет ту же сломанную формулу, поэтому проверка самосогласована. Ключ детерминирован: `streamTicketKey()` — `hkdf.New(sha256, s.signKey, nil, []byte("stream-ticket-v1"))`, без соли, с фиксированной меткой.

**Исполняемое подтверждение** (реальный `crypto/hmac`, воспроизведено дословно):

```
legit ticket: 1791462594.client-A.TORRENT-XYZ.313739313436323539347c636c69656e742d417c544f5252454e542d58595adde28f39c5432809af2f4315d545ea62adba37b20f511dbdf110c581e71227c5
validate legit -> user="client-A" ok=true

trailing const (hex HMAC of empty msg): dde28f39c5432809af2f4315d545ea62 ...

FORGED ticket accepted? ok=true user="attacker"
sig suffix identical across independent tickets: true
```

Последние 64 hex-символа подписи — `dde28f39c543…127c5` — **побайтово одинаковы** у легитимного и поддельного тикета. Атакующий с одним валидным тикетом берёт этот хвост, приписывает `hex_encode(свой_payload)` — и сервер принимает. Ключевые материалы не нужны.

**Влияние.** `/api/v1/torrents/{id}/stream` вынесен **вне** группы с `TokenMiddleware` (`internal/api/router.go:63`), поскольку libmpv не умеет прикладывать заголовки, и защищён исключительно этим тикетом. Один валидный тикет ⇒ доступ к **любому** торренту на сервере. Атакующий полностью контролирует поле `expiry`, поэтому `StreamTicketTTL = 5 * time.Minute` (`constants.go:162`) фактически не действует.

**Исправление:**

```go
mac := hmac.New(sha256.New, s.streamTicketKey())
if _, err := mac.Write([]byte(payload)); err != nil {
    return "", fmt.Errorf("sign stream ticket: %w", err)
}
sig := hex.EncodeToString(mac.Sum(nil))
```

Симметрично в `ValidateStreamTicket`. Сильнее — непрозрачный тикет фиксированной формы (nonce + серверная карта, либо base64-payload + MAC). Регрессионный тест обязан проверять **стойкость к подделке**, а не round-trip (см. P1-1).

---

### P0-2 — SSE-эндпоинт событий комнаты возвращает 500

Цепочка:

1. `router.go:45` — `r.Use(Logger)`; middleware уровня роутера оборачивает **каждый** маршрут, включая `GET /api/v1/rooms/{roomID}/events`.
2. `middleware.go:113` — обработчику передаётся `&responseWriter{ResponseWriter: w}`.
3. `middleware.go:517-527` — `responseWriter` встраивает интерфейс `http.ResponseWriter`, поэтому в его метод-сете только `Header/Write/WriteHeader`. `Flush()` — часть `http.Flusher`, а **не** `http.ResponseWriter`, и не продвигается. `Unwrap()`/`ReadFrom()` тоже отсутствуют.
4. `handlers.go:147` — `flusher, ok := w.(http.Flusher)`; при `!ok` пишется `500 {"error":"Streaming is not supported"}`.

Проверено отдельной программой: `*responseWriter` **не** удовлетворяет `http.Flusher`, тогда как «голый» `httptest.ResponseRecorder` удовлетворяет.

**Влияние.** `RoomEvents` (`handlers_room.go:218-251`) → `SSEEventHandler` → 500, соединение сразу закрывается. Вся сервер-брокерная синхронизация — вторая ключевая возможность из CONCEPT.md («Синхронные комнаты») — не работает.

**Почему CI не поймал:** ни один тест `internal/api` не проходит SSE-маршрут через полный роутер. `leak_test.go` тестирует `sseConnectionManager` изолированно; `e2e_test.go` имеет `TestE2E_RoomFlow`, но на `/events` не подписывается. Прямые вызовы `SSEEventHandler` идут с `httptest.NewRecorder()`, который `http.Flusher` реализует.

**Исправление:**

```go
func (rw *responseWriter) Flush() {
    if f, ok := rw.ResponseWriter.(http.Flusher); ok { f.Flush() }
}
func (rw *responseWriter) Unwrap() http.ResponseWriter { return rw.ResponseWriter }
```

Плюс e2e-тест на `/events` через `api.NewRouter`.

---

### P0-3 — `WriteTimeout = 30s` обрывает любое воспроизведение длиннее 30 секунд

`cmd/server/main.go:167-173` ставит `WriteTimeout: constants.ServerWriteTimeout` = `30 * time.Second` (`constants.go:17`).

`WriteTimeout` в `net/http` — **абсолютный дедлайн записи**, установленный при чтении заголовков запроса и действующий до завершения ответа. Он не сбрасывается между записями. Ни SSE-обработчик, ни `ServeFile` его не продлевают.

**Исполняемое подтверждение** (изолированный сервер с ровно тем же значением `WriteTimeout`, передача 8 МБ):

```
  server write FAILED after 7798784/8388608 bytes: write tcp ...: i/o timeout
client read 7798784 of 8388608 bytes after 30s, err=unexpected EOF
RESULT: response TRUNCATED by the write deadline -> client received a PARTIAL body
```

**Влияние.** Это самое тяжёлое последствие из всех трёх P0 и бьёт ровно в заявленную в CONCEPT.md главную ценность («фильм запускается сразу… остальное догружается»): `http.ServeContent` в `torrent/service.go:579` отдаёт файл по мере готовности, и через 30 секунд соединение рвётся с `unexpected EOF` на стороне libmpv. Фильм длиннее 30 секунд не досмотреть. Побочно — SSE-соединение с `SSETimeout = 30 * time.Minute` (`constants.go:242`) тоже было бы убито на 30-й секунде, даже если бы P0-2 был исправлен.

**Исправление.** Убрать глобальный `WriteTimeout` (`0`) и управлять дедлайном точечно:

```go
// в SSEEventHandler и ServeFile, до первой записи:
rc := http.NewResponseController(w)
_ = rc.SetWriteDeadline(time.Time{})   // снять дедлайн для долгоживущих потоков
```

Либо оставить `WriteTimeout` для коротких JSON-ответов, но отключать его на этих двух маршрутах через `ResponseController`.

---

### P1-1 — Тест stream-ticket'ов дублирует сломанную формулу

`backend/internal/auth/auth_test.go:152-162`:

```go
t.Run("expired", func(t *testing.T) {
    expiry := time.Now().Add(-time.Minute).Unix()
    payload := strconv.FormatInt(expiry, 10) + "|" + clientID + "|" + torrentID
    mac := hmac.New(sha256.New, svc.streamTicketKey())
    sig := hex.EncodeToString(mac.Sum([]byte(payload)))   // auth_test.go:157 — ТА ЖЕ ОШИБКА
```

Тест **переписывает продольную логику подписи вместо её использования**, поэтому проверяет исключительно самосогласовательность двух экземпляров одной ошибки. Именно поэтому P0-1 не виден CI. Ценность теста на криптографию определяется тем, использует ли он **независимую** реализацию.

Соседние под-тесты тоже проверяют не то: `"wrong torrent"` и `"expired"` — парсер полей, а не MAC; `"tampered signature"` подменяет подпись **целиком** на нули (отвергается случайно); `"issued by another process"` проходит лишь потому, что у другого процесса другая константа-хвост — то есть тоже тестирует константу, а не MAC.

**Исправление.** Собирать «враждебные» тикеты независимым кодом (только `crypto/hmac` + `crypto/rand`) и добавить кейсы «мутирован `expiry` → отвергнут», «мутирован `userID` → отвергнут», «мутирован `torrentID` → отвергнут».

---

### P1-2 — `emitEvent` итерирует карту пиров без блокировки

`backend/internal/p2p/service.go:697-732`:

```go
s.mu.RLock()
room, exists := s.rooms[roomID]
s.mu.RUnlock()          // <- RLock отпущен, но указатель room уходит дальше
...
for _, peer := range room.Peers {   // :708 — итерация БЕЗ блокировки
```

Тот же дефект на `:725` (`room.Peers[hostPeer.ID]`) и `:732` (`len(room.Peers)`).

Тем временем `room.Peers` пишется под `s.mu.Lock()`: `JoinRoom` (`:451`), `LeaveRoom` (`:508`), `pruneIdlePeers` (`:230`). Итог — **конкурентная итерация и запись одной карты**. Это не просто «гонка данных»: рантайм Go в этом случае выдаёт фатальное `fatal error: concurrent map iteration and map write`, которое **не перехватывается `recover()`** и не ловится middleware `Recovery` — процесс падает целиком. Один SSE-клиент в момент, когда кто-то входит или выходит из комнаты, способен уронить сервер.

**Почему `go test -race` это не показал:** ни один тест не создаёт конкурентный поток `emitEvent` + мутация комнаты. Результат `-race` здесь не является опровержением.

**Исправление:** снять список пиров под `RLock`, работать с копией:

```go
s.mu.RLock()
room, exists := s.rooms[roomID]
var peers []*Peer
if exists {
    for _, p := range room.Peers { peers = append(peers, p) }
}
s.mu.RUnlock()
```

И добавить в `internal/p2p/deadlock_test.go` конкурентный тест «N горутин Join/Leave + M горутин SendSignal/BroadcastSync» — он упал бы и на детекторе гонок, и без него.

---

### P1-3 — `DetailedHealthCheck` не считает p2p-сервис упавшим

`backend/internal/api/handlers.go:367-372`:

```go
if p2pSvc != nil {
    services["p2p"] = "ok"
} else {
    _, _ = services, allHealthy     // заглушка: ничего не делает
}
```

Соседние ветки для `torrent` (`:360-365`) и `sync` (`:374-380`) корректны. При `p2pSvc == nil` хендлер отвечает `200 {"status":"ok"}`, а ключ `p2p` просто отсутствует. Для диагностического эндпоинта это ложное «всё хорошо» ровно тогда, когда сервис упал.

### P1-4 — Rate limit обходится подменой `X-Forwarded-For`

`backend/internal/api/middleware.go:310-322` + `:357-378`. При незаданном `TRUSTED_PROXIES` (дефолт, `:377`) `isTrustedProxy` возвращает `true` **для любого приватного IP**. Ключ бакета берётся из первого элемента `X-Forwarded-For`, который клиент контролирует полностью:

1. **Обход частоты.** Ротация заголовка даёт свежий `rate.Limiter` на каждый запрос — лимит 60 req/min не действует. Размещение `PerIPRateLimiter` до `TokenMiddleware` (`router.go:69`) тут не помогает: токен всё равно проверяется корректно, но лимит обходится.
2. **Исчерпание памяти.** `getLimiter` (`:414-449`) создаёт запись для каждого нового значения IP; удаление — только в `cleanupLoop` для записей старше 10 минут. Уникальный `X-Forwarded-For` на запрос ⇒ неограниченный рост карты в окне 10 минут. Ёмкость не ограничена.

**Исправление.** Дефолтом доверять только loopback; приватные сети — лишь явным `TRUSTED_PROXIES`. Ограничить ёмкость карты и сократить TTL очистки.

### P1-5 — CORS preflight отвечает 200 на любой origin

`backend/internal/api/middleware.go:212-221`. Ответ `200` отдаётся **до** решения по origin. Сама защита держится лишь на отсутствии `Access-Control-Allow-Origin`, что тонко: при изменении `CORS_ORIGINS` легко превращается в дыру. Расходится и с кодом: `router.go:66-67` и `auth/middleware.go:28-31` утверждают, что сервер префлайт **не одобряет**.

**Исправление.** На `OPTIONS` с неразрешённым origin отвечать `403` и не выставлять `Access-Control-Allow-*`; `Access-Control-Allow-Credentials` — только вместе с `Allow-Origin`. Комментарии привести в соответствие.

### P1-6 — Shutdown может висеть до 30 минут

`cmd/server/main.go:243-246`: `server.Shutdown(ctx)` (30s) по таймауту возвращает ошибку, но выполняется следом `api.WaitForSSEConnections()` — **без таймаута**. Живое SSE-соединение держится до `SSETimeout = 30 * time.Minute`. Ctrl-C после этого может не завершить процесс 30 минут.

**Исправление.** Обернуть в `select` с `ctx.Done()` либо ограничить ожидание несколькими секундами.

### P1-7 — Повторный `p2p.Close()` паникует

`internal/p2p/service.go:618-667`. Внутри `closeOnce.Do` находятся только строки 619-624 (`pruneTicker.Stop`, `close(s.closeChan)`). Всё остальное — `flushRooms()`, `close(s.doneChan)` на `:652`, `close(s.eventChan)` — выполняется при **каждом** вызове. Второй вызов паникует «close of closed channel». Структура содержит `closeOnce`, а `sync.Service.Close` документирован как «Safe for multiple calls (uses sync.Once)» (`sync/service.go:231`), так что намерение есть — реализация ему не соответствует.

**Исправление.** Перенести тело целиком внутрь `closeOnce.Do`.

### P1-8 — Пользовательская перемотка искажается сглаживанием

`internal/sync/service.go:189` + `:205-210`:

```go
status.Position = applyLatencyCompensation(oldPosition, position)

func applyLatencyCompensation(current, requested float64) float64 {
	if math.Abs(requested-current) <= constants.MaxPositionJump { return requested }
	return current + (requested-current)*constants.SmoothAdjustmentRatio   // 0.3
}
```

Запрос «перемотать на 600 секунд» из позиции 0 применяется как **180 секунд** (0 + 600×0.3). Пользователь не может попасть в дальнюю точку одним действием — только серией перемоток, каждая из которых «съедает» 70% дистанции.

Логика правда сглаживает **дрейф между пирами**, но `ARCHITECTURE.md:475` описывает для этого отдельный метод `SyncWithLatency(peerStatus, latencyMs)`; grep по `backend/` показывает, что **такого метода не существует** — логика компенсации была приклеена к пользовательскому `Seek`.

**Исправление.** `Seek` должен применять позицию как есть (это явная команда), а сглаживание перенести в отдельный `SyncWithLatency`, вызываемый при сверке состояния, и описать его в ARCHITECTURE.

### P1-9 — bcrypt под глобальным write-локом p2p

`internal/p2p/service.go:383` (`bcrypt.GenerateFromPassword`, cost 12) и `:439` (`bcrypt.CompareHashAndPassword`) выполняются **под `s.mu.Lock()`**. Cost 12 — это ~200-300 мс CPU. Всё это время заблокированы все операции p2p: вход, выход, доставка событий всем остальным комнатам.

**Исправление.** Хешировать/сравнивать до захвата лока, сохранив атомарность через повторную проверку под локом.

---

### Технический долг рефакторинга JWT → access token

Миграция доведена функционально, но не до конца. Мёртвый код, подтверждённый grep'ом (0 production-ссылок):

| Символ | Где | Статус |
|---|---|---|
| `JWTTokenTTL`, `JWTSecretLength`, `JTIBytes`, `JWTIssuer`, `JWTAudience`, `MinTokenLength` | `constants.go:128-157` | мёртвые |
| Секция `// ── JWT Constants ──` | `constants.go:125` | озаглавливает блок, где остались только `StreamTicketTTL`/`Secret` |
| `StreamTicketSecret` описан как «JWT secret» | `constants.go:164-166` | устаревший комментарий |
| `models.Claims` | `models/types.go:147-148` | мёртвая структура |
| `NormalizeUsername`, `ValidateUsername`, `ValidatePassword`, `commonPasswords`, `usernameRegex`, `Min/MaxUsernameLength`, `MinPasswordLength` | `validation/validation.go:34-55,101-181` | целый слой валидации аккаунтов, 0 production-ссылок |
| `CSRFTokenTTL`, `CSRFTokenStoreMaxSize`, `CSRFCleanupInterval`, `CSRFShutdownTimeout`, `CSRFTokenBytes`, `CSRFRateLimit/Burst`, `RevocationStore*` | `constants.go:24-44,152-158` | мёртвые |
| Комментарии «REQUIRES JWT AUTHENTICATION», «libmpv cannot attach JWT/CSRF headers» | `handlers.go:346`, `handlers_torrent.go:284,286,299` | устаревшие |
| `internal/api/handlers.go.tmp` (429 строк) | в дереве исходников | временный файл; перекрыт `.gitignore:71`, в git **не** попал |

Правило AGENTS.md «нет user store, нет JWT» соблюдено в *логике*, но не в *коде*: мёртвый слой валидации аккаунтов и комментарий про `NormalizeUsername` прямо описывают инвариант «usernames are case-insensitive» для системы, которой больше нет.

---

## Тестовое покрытие: что реально проверяется

`go test ./...` и `go test -race ./...` зелёные, 5192 строки тестов против 8155 строк кода. Но зелёный статус вводит в заблуждение в трёх конкретных местах:

1. **Криптография self-validating.** `auth_test.go:157` переписывает сломанную формулу — тест на подделку фактически отсутствует (P1-1). Единственный настоящий защитный тест, `TestStreamTicketRejects/tampered signature`, проверяет подмену подписи на нули, что отвергается независимо от того, работает ли MAC.
2. **`-race` не покрывает p2p.** Гонка по `room.Peers` в `emitEvent` (P1-2) не обнаружена, потому что ни один тест не гоняет `emitEvent` конкурентно с мутацией комнаты. `internal/p2p/deadlock_test.go` проверяет отсутствие дедлока, но не конкурентный доступ к карте.
3. **HTTP-слой не проверяется сквозным SSE-тестом.** Нет ни одного теста, который подписывается на `/api/v1/rooms/{id}/events` через `api.NewRouter` — именно поэтому P0-2 и не был замечен. `TestE2E_RoomFlow` покрывает create/join/leave, но не подписку.

Конкретные непокрытые пути:

| Путь | Почему важно |
|---|---|
| `GET /rooms/{id}/events` через полный роутер | Покрыл бы P0-2. Сейчас — ни одного теста |
| `ServeFile` с реальным HTTP-клиентом и `Range` | Стриминг — основная функция; нет теста на `206 Partial Content` |
| Поведение при `WriteTimeout` | Покрыл бы P0-3. Нет теста на длинный ответ |
| `p2p.Close()` дважды | Покрыл бы P1-7 |
| Конкурентные `Join/Leave` + `emitEvent` | Покрыл бы P1-2 |
| `Seek` на дальнюю позицию | Покрыл бы P1-8 — текущие тесты, судя по `sync/service_test.go`, проверяют лишь границы валидации |
| `DetailedHealthCheck` при `p2pSvc == nil` | Покрыл бы P1-3 |
| `getClientIP` с подменённым `X-Forwarded-For` | Покрыл бы P1-4 |
| `utils.Debouncer` — гонка `fire()` при занятом `inFlight` | Покрыл бы P3-5 |

`internal/contract/pact_provider_test.go` не появляется в выводе `go test ./...`, потому что закрыт тегом `//go:build contract` (`:3`). Это **не** пропуск покрытия: `.github/workflows/ci.yml:459` запускает его отдельно через `go test -tags contract -run PactProvider ./internal/contract/...`, а `backend/Makefile:120-123` даёт цель `make contract-test`. При локальном прогоне `make test` эти тесты просто не выполняются — стоит держать это в голове при оценке «зелёного» локального прогона.

---

## Что здесь хорошо

* **Разделение публичного и защищённого.** `TokenMiddleware` навешан на `r.Group` (`router.go:68-70`), публичными оставлены ровно осознанные маршруты. Исключение `/metrics` — принятое проектное решение, находкой не считаю. Поток стрима вынесен наружу **с объяснением «почему»** (`router.go:60-63`, `81-83`) — редкая и ценная дисциплина.
* **Проверка токена сделана правильно.** `subtle.ConstantTimeCompare` (`auth.go:92`), ранний выход на пустом заголовке, ответ 401 без подсказки, какой элемент неверен (`auth/middleware.go:34-39`).
* **Ноль хранимого состояния аутентификации.** Ни базы, ни ротации, ни blacklist — и комментарии объясняют *почему* JWT не нужен одно��ользовательскому десктопу. Зрелое решение.
* **Защита тел запросов.** `http.MaxBytesReader` на комнатах и сигналах с отдельным, обоснованно более жёстким `MaxSignalSize`. Пароль комнаты намеренно не логируется (`handlers_room.go:118`).
* **Защита от path traversal в стриминге.** `sanitizeFilename` (`torrent/service.go:656-664`) использует `path.Base` и вычищает `\r`, `\n`, `\x00` — корректно и для отчётов, и для `Content-Disposition`.
* **TOCTOU в `RemoveTorrent`/`ServeFile` решён правильно.** `streamWG.Add(1)` под `RLock` (`:556`) и удаление из карты под `Lock` (`:333`) гарантируют, что активный стрим не останется на дропнутом торренте. Это редко делается аккуратно.
* **Управление жизненным циклом лимитера.** `Stop()`, `sync.Once`, `WaitGroup`, `recover()` в `cleanupLoop` (`middleware.go:275-307`) — внимательнее среднего.
* **Комментарии как объяснение «почему».** Блоки в `p2p/service.go:272-288` и `sync/service.go:66-79` подробно описывают, почему `scheduleSave` **не** берёт `s.mu` (иначе взаимоблокировка). Это ровно тот класс документации, который спасает при следующем рефакторинге.
* **Ноль TODO/FIXME/HACK** по всему `backend/` — подтверждено grep'ом. Функционально JWT-рефакторинг доведён.
* **Инфраструктура выше среднего:** `make test` / `make test-race` / fuzz / мутационное покрытие с порогом `MUTATION_MIN_RATIO=0.49` / pact-контракты / k6 с явными порогами. Наличие трёх P0 при такой инфраструктуре — провал покрытия, а не процесса.

---

## Рекомендуемые работы

Порядок зависимостей: сверху вниз. Размер: S ≈ <0.5 дня, M ≈ 1-3 дня, L ≈ >3 дней.

1. **Починить MAC стрим-тикета.** `internal/auth/auth.go:109-110,138-139`. `mac.Write(payload)` + `mac.Sum(nil)`. **Почему:** полный обход access token. **S.** Файлы: `auth/auth.go`.
2. **Добавить регрессионный тест на стойкость к подделке** и убрать дублирование формулы из теста. `internal/auth/auth_test.go:152-170`. **Почему:** без него пункт 1 снова сломается тем же способом. **S.** Файлы: `auth/auth_test.go`.
3. **Убрать `WriteTimeout` для долгоживущих потоков.** `cmd/server/main.go:171`, `internal/constants/constants.go:17`; снятие дедлайна в `api/handlers.go` (`SSEEventHandler`) и `torrent/service.go:579`. **Почему:** видео >30s не воспроизводится — ядро продукта. **S.** Файлы: `cmd/server/main.go`, `internal/constants/constants.go`, `internal/api/handlers.go`, `internal/torrent/service.go`.
4. **Реализовать `Flush()`/`Unwrap()` в `responseWriter`.** `internal/api/middleware.go:517-527`. **Почему:** вся синхронизация комнат отдаёт 500. **S.** Файлы: `internal/api/middleware.go`. *(Зависит от 3: без снятия дедлайна SSE всё равно умрёт на 30-й секунде.)*
5. **Закрыть гонку по `room.Peers` в `emitEvent`.** `internal/p2p/service.go:697-732`. **Почему:** фатальное `concurrent map iteration and map write` роняет процесс. **S.** Файлы: `internal/p2p/service.go`.
6. **Добавить сквозной SSE-тест через `api.NewRouter`** и конкурентный p2p-тест (Join/Leave + emitEvent). **Почему:** закрывает слепое пятно, из-за которого 4 и 5 не были замечены; `-race` начнёт работать как надо. **M.** Файлы: `internal/api/e2e_test.go`, `internal/p2p/deadlock_test.go`.
7. **Починить CORS-preflight и доверие к прокси.** `internal/api/middleware.go:212-221`, `:311-322`, `:357-378`. **Почему:** обход rate limit + рост памяти + расхождение комментариев с поведением. **M.** Файлы: `internal/api/middleware.go`, `internal/constants/constants.go`.
8. **Ограничить шум от CORS-токенов:** `find` по всем `bcrypt.GenerateFromPassword` / `CompareHashAndPassword` под локами; `internal/p2p/service.go:383,439`. **Почему:** 250ms блокировки всех комнат на создание пароля. **S.** Файлы: `internal/p2p/service.go`.
9. **Разделить `Seek` и сглаживание дрейфа.** `internal/sync/service.go:189,205-210`; ввести `SyncWithLatency`, как описано в `ARCHITECTURE.md:475`. **Почему:** перемотка на 600s попадает в 180s — пользовательская функция не работает. **M.** Файлы: `internal/sync/service.go`, `internal/interfaces.go`, `docs/ARCHITECTURE.md`.
10. **Закрыть пути shutdown.** `cmd/server/main.go:246` (таймаут на `WaitForSSEConnections`), `internal/p2p/service.go:652` (весь `Close` внутрь `closeOnce`), `:634-645` (не закрывать каналы, или координировать с отправителями). **Почему:** зависание на 30 минут и паника при повторном закрытии. **M.** Файлы: `cmd/server/main.go`, `internal/p2p/service.go`.
11. **Довести чистку JWT-remnant'ов до конца.** Удалить мёртвые константы (`constants.go:24-44,125-167`), `models.Claims`, слой валидации аккаунтов (`validation.go:34-55,101-181`), комментарии (`handlers.go:346`, `handlers_torrent.go:284,286,299`), файл `internal/api/handlers.go.tmp` из git. **Почему:** правило AGENTS.md и здоровье кода; мёртвый код провоцирует повторное использование. **S.** Файлы: перечисленные выше.
12. **Починить `DetailedHealthCheck`** (`handlers.go:368-372`) и единообразить ответы `ServeFile` на `WriteError` (`torrent/service.go:517,525,538,551`). **Почему:** ложное «всё хорошо» + неконсистентный формат ошибок API. **S.** Файлы: `internal/api/handlers.go`, `internal/torrent/service.go`.
13. **Оптимизировать приоритеты писей.** `internal/buffer/service.go:202-207` + тикер `:275-279`. **Почему:** `O(TotalPieces)` `SetPriority` каждую секунду на торрент; на 4K-ремуксе это тысячи вызовов/с. **M.** Файлы: `internal/buffer/service.go`.
14. **Ограничить рост состояния p2p.** TTL-очистка `s.sessions` (`p2p/service.go:139,322-343`) и пустых комнат (`:391-403,511-514`); потолок карты rate limiter'а. **Почему:** неограниченная память по данным атакующего/пользователя. **M.** Файлы: `internal/p2p/service.go`, `internal/api/middleware.go`.
15. **Вынести магические числа в `constants.go`** (`buffer/service.go:144,246`, `api/middleware.go:139,267`, `api/handlers_room.go:247`, `validation.go:345`, `handlers_room.go:60`) и убрать забытый `MaxStringLength`. **Почему:** прямое требование AGENTS.md. **S.** Файлы: перечисленные выше.
16. **Устранить мелочи P3** (комментарий `auth.go:67-69`, утечка fd `main.go:446-450`, `retErr` `main.go:349`, `Debouncer.fire` `utils/debouncer.go:56-60`, `RemoveTorrent` в `storage/disk.go:42-48`). **Почему:** полировка; каждый пункт < 30 минут. **S.** Файлы: перечисленные выше.
17. **Решить судьбу `ctx` в сервисах** (везде `_ = ctx`) и pprof без аутентификации. **Почему:** либо реализовать отмену, либо убрать вводящий в заблуждение параметр; задокументировать риск дампа кучи. **M.** Файлы: `internal/*/service.go`, `cmd/server/main.go`.