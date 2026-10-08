# Аудит frontend (Qt6/C++) и контракта API

**Область:** `frontend/` (28 файлов, ~8 600 LOC), плюс сквозная сверка call sites фронтенда
с фактической поверхностью API (`backend/internal/api/router.go`, `paths.go`, handlers,
`internal/p2p`, `internal/models`, `pacts/frontend-backend.json`).

**Метод:** статический анализ всех исходников фронтенда + чтение роутера/хендлеров/pact-контракта
на стороне бэкенда (только чтение, ничего не менялось).

---

## Вердикт

Фронтенд **не работает как продукт**. Ключевая фича проекта — синхронные комнаты
(`CONCEPT.md`, раздел «Синхронные комнаты») — нефункциональна **по трём независимым причинам**,
каждая из которых по отдельности убивает фичу: `RoomManager` никогда не эмитит свои сигналы
(`isInRoom()` навсегда `false`), ответ `POST /api/v1/rooms/join` не содержит `id`
(в `docs/API.md` он тоже не документирован), а события синхронизации приходят с
`type:"sync"`, тогда как фронтенд маршрутизирует только `type=="signal"`. Вторая по значимости
поломка: `mpv_observe_property()` **никогда не вызывается**, поэтому `time-pos`/`duration`
не обновляются никогда — длительность всегда 0, а `MainWindow::onSeek()` выходит по
`if (m_duration <= 0) return;`, то есть перемотка и всё, что на ней построено, мертвы.
Третья: список торрентов и список файлов разбираются как «голый JSON-массив», а бэкенд
возвращает конверт `{torrents|files, totalCount, …}` — UI останется пустым всегда.
Отдельно: тестовая обвязка, которая должна была это поймать, физически не собирается
(`test_asan`/`test_tsan` объявляют два `main()` в одном таргете), а существующие тесты
в значительной части проверяют `QVERIFY(m_manager != nullptr)`. При этом качество
инфраструктурных решений — санитизация URL, TLS-политика, обработка кодов возврата mpv,
`QPointer` в асинхронных колбэках, разумные лимиты на размер ответа — сделано хорошо;
проблема не в небрежности, а в том, что рефакторинг (выделение `TorrentManager`/`RoomManager`)
оборвал сигнальные соединения, и никто это не проверил.

---

## Build & test reality

**Тулчейн отсутствует.** На машине нет `cmake`, нет Qt6 dev-пакетов; `libmpv` при этом есть
(через `pkg-config`). Собрать фронтенд и выполнить `ctest` физически невозможно.
Я **не** пытался ставить тулчейн — это была бы потеря времени без результата.

Что вместо этого сделано (статическая верификация):

1. **Полное чтение всех 28 файлов** фронтенда (`src/*.cpp`, `src/*.h`, `src/interfaces/`,
   `src/mocks/`, `test_*.cpp`), включая `CMakeLists.txt` (553 строки) — проверена каждая
   `add_executable`/`add_test`/`target_link_libraries` на предмет связности.
2. **Сверка каждого call site** с `backend/internal/api/router.go` (реальные регистрации маршрутов),
   `paths.go` (константы путей), хендлерами (`handlers_torrent.go`, `handlers_room.go`,
   `handlers_sync.go`), моделями (`internal/models`) и `pacts/frontend-backend.json`.
3. **Проверка мок-сервера в тестах против роутера**: `startRoomMockServer()` в
   `test_networkmanager.cpp` отвечает на `POST /api/v1/csrf-token`, которого в роутере нет,
   и отдаёт `POST /api/v1/rooms/join` ответ `{"id":…}`, которого реальный хендлер не отдаёт.
   То есть тест закрепляет **неверный** контракт и «зелёный» именно потому, что мок совпадает
   с багом, а не с сервером.
4. **Ручной разбор границ владения и блокировок** в `MpvWidget` и `NetworkManager`
   (пути `emit` под `QMutexLocker`, порядок разрушения mpv-ресурсов, lifetime лямбд).
5. **Анализ связности тестовой обвязки**: `make test-frontend-asan`, `test-frontend-tsan`,
   `test-frontend-gmock`, `contract-test-frontend`, `e2e-test` ссылаются на таргеты,
   которых нет в `frontend/CMakeLists.txt`; часть из них дополнительно не линкуется.

Лёгкий синтаксический вариант, который *бы* частично проверил: `g++ -fsyntax-only` невозможен
без заголовков Qt. Единственное, что даёт хоть какой-то сигнал, — `clang-tidy`/grep-инварианты;
они и использованы (в частности, проверено, что `mpv_observe_property` не встречается в проекте
ни разу — `grep` по всему `frontend/`).

**Итог: все утверждения ниже основаны на чтении кода, а не на прогоне.** Пункты P0, помеченные
«подтверждено роутером/хендлером», — это дедукция из двух независимых источников (код фронтенда
+ код бэкенда/контракт), а не наблюдение за рантаймом. Их стоит перепроверить первым делом,
как только тулчейн появится.

---

## Таблица дрейфа контракта API

Источник истины — `backend/internal/api/router.go` + хендлеры + `pacts/frontend-backend.json`.
Колонка «Статус» — итог для фронтенда.

| # | Call site (файл:строка) | Метод + путь | Что реально отдаёт бэкенд | Статус |
|---|---|---|---|---|
| 1 | `networkmanager.cpp:140` | `POST /api/v1/torrents` `{magnetUri}` | `201` + `TorrentInfo{id,name,progress,status,size}` | **OK** |
| 2 | `networkmanager.cpp:162` | `POST /api/v1/torrents` `{torrentFile}` (base64) | тот же хендлер, ветка `hasTorrentFile` | **OK** |
| 3 | `networkmanager.cpp:178` | `DELETE /api/v1/torrents/{id}` | `200 {"message":"Torrent removed"}` | **OK** (ID = 40 hex, санитизация совместима) |
| 4 | `networkmanager.cpp:183` → `:478` | `GET /api/v1/torrents` | `200 {torrents:[…], totalCount, limit, offset, hasMore}` | **СЛОМАНО**: фронтенд требует `doc.isArray()` → `torrentListReceived` не эмитится никогда |
| 5 | `networkmanager.cpp:194` → `:494` | `GET /api/v1/torrents/{id}/files` | `200 {files:[…], totalCount, limit, offset, hasMore}` | **СЛОМАНО**: то же, `doc.isArray()` |
| 6 | `networkmanager.cpp:217` → `:500-513` | `POST /api/v1/torrents/{id}/select` `{fileIndex}` | `200 {"message":"File selected"}` | **OK** (torrentId берётся из regex по URL, не из тела) |
| 7 | `networkmanager.cpp:411` | `POST /api/v1/torrents/{id}/stream-ticket` | `200 {ticket, torrentId}` (`StreamTicketResponse`) | **OK**, `X-Client-ID` реально шлётся (`applyAuthHeader`), хендлер его требует — попадание |
| 8 | `mainwindow.cpp:470` (libmpv) | `GET /api/v1/torrents/{id}/stream?ticket=…` | публичный маршрут вне token-группы, тикет HMAC, TTL 5 мин | **OK по форме**, но: тикет не URL-экранируется, не перевыпускается на 401/истечении, логируется (см. F-13/F-14) |
| 9 | `networkmanager.cpp:231` | `POST /api/v1/torrents/{id}/buffer/position` `{position}` | `200 {"message":"Position updated"}` | **OK** |
| 10 | `networkmanager.cpp:240` → `mainwindow.cpp:632` | `GET /api/v1/torrents/{id}/buffer/info` | `{torrent_id, file_index, current_position, buffer_start, buffer_end, buffer_size, buffered_bytes, buffered_percent, download_speed, is_buffering}` | **OK**: фронтенд читает `buffered_percent` — единственное точное совпадение по полю. `docs/API.md:380` описывает `{position, buffered, bufferSize}` — **устарело** (зона docs-агента) |
| 11 | `networkmanager.cpp:277` → `:519-529` | `POST /api/v1/rooms` `{name,password?}` | `201 {id, name, hostId, peerCount}` | **OK по форме**, но SSE не открывается (F-06) |
| 12 | `networkmanager.cpp:312` → `:530-547` | `POST /api/v1/rooms/join` `{roomId,password?}` | `200 {"message":"Joined the room"}` — **поля `id` нет** | **СЛОМАНО**: `obj["id"].toString()` → `""` → `m_currentRoomId=""`, SSE-путь `/api/v1/rooms//events` → 404 |
| 13 | `networkmanager.cpp:325` | `POST /api/v1/rooms/leave` | `200 {"message":"Left the room"}` | **OK** по форме (но недостижимо, см. F-02) |
| 14 | `networkmanager.cpp:346` | `POST /api/v1/rooms/signal` | ожидает `{roomId, signal: <base64 []byte>}` | **СЛОМАНО + мёртвый код**: фронтенд шлёт `{action,position}` без обёртки; `[]byte` ⇒ base64-строка; вызов `sendSignal` **нигде не делается** |
| 15 | `networkmanager.cpp:545`, `:861` | `GET /api/v1/rooms/{roomID}/events` (SSE) | маршрут есть | **СЛОМАНО**: `roomID` пуст из-за #12; открывается только для join, не для create |
| 16 | SSE `type:"peer_joined"` / `"peer_left"` | `{"type":…,"peerId":…}` (service.go:463, :524) | совпадает | **OK** по форме, но недостижимо (см. #12/F-06) |
| 17 | SSE `type:"sync"` (`BroadcastSync`, service.go:737) | `{"type":"sync","data":{action,status:{…},initiator}}` | совпадает | **СЛОМАНО**: `onSsEReadyRead` ретранслирует в `signalReceived` только `type=="signal"`; `"sync"` уходит в безвственный `roomEvent` |
| 18 | SSE `type:"signal"` (`SendSignal`, service.go:562) | `{"type":"signal","data":<base64 []byte>}` | `data` — **строка** base64, не объект | **СЛОМАНО**: фронтенд делает `event["data"].toObject()` → пустой объект → `action` пуст → отбрасывается |
| 19 | Поле позиции в sync-событии | фронт читает `signal["position"]` | позиция лежит в `status.position` | **СЛОМАНО** даже после починки #17 |
| 20 | `networkmanager.cpp:186,355,359,364` | `POST /api/v1/sync/{play,pause,seek}` | маршруты есть, `seek` ждёт `{position}` | **OK по форме**, но **недостижимо**: `RoomManager::isInRoom()` всегда `false` (F-02) |
| 21 | `torrentmodel.h:46-48` | поля `downloaded`, `uploadSpeed`, `downloadSpeed` | бэкенд `TorrentInfo` их **не отдаёт** | **Дрейф**: всегда 0; роли `DownloadedRole/UploadSpeedRole/DownloadSpeedRole` мертвы |
| 22 | `networkmanager.h:52-53` + `networkmanager.cpp:971-980` | `RequestType::Login` / `Register` | `paths.go` содержит `/api/v1/auth/{login,register,logout}`, но **маршруты не зарегистрированы** | **Мёртвый код**: фронтенд их не вызывает, ветка обработки `Register` недостижима |
| 23 | комментарий `networkmanager.cpp:87`, мок `test_networkmanager.cpp:199` | `POST /api/v1/csrf-token` | маршрута **нет** в роутере | фронтенд больше не вызывает (только комментарий), но **тест закрепляет несуществующий эндпоинт** |
| 24 | — | `GET /health`, `GET /api/v1/version`, `GET /api/v1/health/detailed`, `GET /metrics`, `GET /api/v1/sync/status` | все существуют | **Существуют, но фронтенд их не вызывает**: нет определения «сервер жив»/версии, нет показа версии в UI, health-check перед стартом нет |
| 25 | `main.cpp:249-255` | `--jwt-secret <hardcoded dev default>` | бэкенд использует per-process access token, JWT в коде отсутствует | **Дрейф**: устаревший флаг + захардкоженный дефолтный секрет в бинарнике |
| 26 | `utils.h:21` | `MaxTorrentIdLength = 40` | `ValidateTorrentID` = 40 hex (v1 infohash) | **OK** (v2/64-char infohash не поддержан — заметка на будущее) |
| 27 | `utils.h:22` / `networkmanager.cpp:534-536` | room ID: длина 32, санитизация до `[a-fA-F0-9]` | pact + хендлер: ровно 32 hex-символа | **OK**: санитизация идемпотентна, ID не портится |
| 28 | `networkmanager.cpp:37-42` (`buildApiPath`) | санитизация всех ID в URL до hex | ID всегда hex | **OK** (AGENTS.md соблюдён; см. «What's good») |

---

## Findings

| ID | Severity | Location | Issue | Fix |
|---|---|---|---|---|
| F-01 | **P0** | `frontend/src/mpvwidget.cpp:500-541` vs `:186-238` | `mpv_observe_property()` **не вызывается нигде**. Обработчик `MPV_EVENT_PROPERTY_CHANGE` для `time-pos`/`duration`/`pause` существует, но события никогда не приходят | После `mpv_initialize()` вызвать `mpv_observe_property(m_mpv, 0, "time-pos", MPV_FORMAT_DOUBLE)`, же `"duration"` (DOUBLE), `"pause"` (FLAG), `"eof-reached"` (FLAG), с проверкой кода возврата |
| F-02 | **P0** | `roommanager.cpp` (весь файл), `mainwindow.cpp:295-297,308-311` | `RoomManager` **не эмитит** `roomCreated`/`roomJoined`/`roomLeft` и **никогда не присваивает** `m_currentRoomId`; `MainWindow` не соединяет `NetworkManager::roomCreated/roomJoined/roomLeft` → `RoomManager`. `isInRoom()` всегда `false` | В конструкторе `RoomManager` соединить `NetworkManager::roomCreated/Joined/Left` со слотами, которые пишут `m_currentRoomId`/`m_isHost` и ре-эмитят сигналы |
| F-03 | **P0** | `networkmanager.cpp:530-547` | `POST /api/v1/rooms/join` не возвращает `id` (только `{"message":…}`). Фронтенд читает `obj["id"]` → `""` → SSE-путь `/api/v1/rooms//events` | Использовать `roomId` из собственного запроса (сохранять в `m_pendingJoinRoomId` до отправки), а не из ответа; параллельно предложить бэкенду добавить `id` в ответ (в `docs/API.md` его тоже нет) |
| F-04 | **P0** | `networkmanager.cpp:478-481`, `:494-497` | `ListTorrents` и `GetFiles` ждут «голый массив», бэкенд отдаёт конверт `{torrents:…}` / `{files:…}` → список торрентов и список файлов всегда пусты | `emit torrentListReceived(doc.object()["torrents"].toArray())` и `filesReceived(id, doc.object()["files"].toArray())`; добавить тест на оба конверта |
| F-05 | **P0** | `networkmanager.cpp:637-640`, `roommanager.cpp:118-139` | События синхронизации приходят с `type:"sync"` (`BroadcastSync`), фронтенд ретранслирует в `signalReceived` только `type=="signal"`. Плюс позиция лежит в `status.position`, а читается `signal["position"]` | Роутить `type=="sync"` (и `"signal"`) в `signalReceived`; разбирать `data.status.position` с фолбэком на `data.position` |
| F-06 | **P0** | `networkmanager.cpp:519-529` vs `:543-545` | Ветка `CreateRoom` **не вызывает `connectToSSE()`** — хост комнаты не подписывается на поток событий: не видит `peer_joined`/`peer_left`, не получает эхо синхронизации. Это основной персонаж из `CONCEPT.md` | Вынести `connectToSSE` в общий хелпер и вызвать в обеих ветках (`CreateRoom`, `JoinRoom`) |
| F-07 | **P0** | `mpvwidget.cpp:451-465` (внутри `showEvent`) vs `:177` | Лямбда берёт `QMutexLocker locker(&m_mutex)`, затем вызывает `initializeMpv()`, который **тоже** делает `QMutexLocker locker(&m_mutex)`. `QMutex` нерекурсивный → Qt печатает `QMutex::lock: Recursive lock detected`, внутренний locker фактически не захватывает mutex, а на выходе разлочивает **чужой** → взаимное исключение сломано, при выходе второго locker'а будет `unlocking an unlocked mutex` | Убрать внешний `QMutexLocker` в лямбде (достаточно `m_destroying`-проверки; `initializeMpv()` сам берёт lock), либо сменить `m_mutex` на `QRecursiveMutex` |
| F-08 | **P1** | `networkmanager.cpp:436-467` (5 ранних `return`), `:574` | `reply->deleteLater()` вызывается **только** в happy path. Все ранние выходы (ошибка сети, oversized Content-Length, oversized body, невалидный JSON) оставляют `QNetworkReply` живым — он остаётся ребёнком `QNetworkAccessManager` и живёт до конца процесса | Обернуть тело в `QScopedValueRollback`/`std::exchange`-паттерн либо вызывать `reply->deleteLater()` в единой точке выхода (`const auto guard = qScopeGuard([reply]{ reply->deleteLater(); });`) |
| F-09 | **P1** | `networkmanager.cpp:835-854`, `:982-1000` | Retry-состояние **одно глобальное** (`m_pendingRetry`). `sendWithRetry` перезаписывает его на каждый запрос; `handleApiError` ретраит `m_pendingRetry`, а не тот запрос, который упал. При двух одновременных запросах падение первого приводит к **повтору второго** (дублирующий `POST /api/v1/torrents`, `DELETE`, `select`…) | Хранить retry-контекст в самом запросе: `QHash<QNetworkReply*, RetryRequest>` (тот же `m_replyMap`, расширенный структурой), и в `handleApiError` брать контекст по `reply` |
| F-10 | **P1** | `networkmanager.cpp:689-705` (`emit error()`/`emit serverUnavailable()` **под** `m_retryMutex`), `:317-323`, `:339-345` (то же под `m_roomIdMutex`) + `mainwindow.cpp:661-665` (модальный `QMessageBox::warning`) | `emit` выполняется под мьютексом; слот `onNetworkError` открывает **модальный** диалог → крутится reentrant event loop → любой слот, дёргающий `sendWithRetry`/`currentRoomId`/`isInRoom` (например, `m_bufferPollTimer` раз в 2 с, `mainwindow.cpp:323`), блокирует тот же нерекурсивный мьютекс → **дедлок и зависание GUI** | Считать значения под мьютексом, **выйти из scope**, и только потом `emit`. Модальные диалоги заменить на неблокирующие (status bar + системный трей) |
| F-11 | **P1** | `networkmanager.cpp:337-347`, `inetworkmanager.h:115`, `roommanager.cpp` | `sendSignal()` нигде не вызывается; его тело не соответствует бэкенду (`{roomId, signal: base64}`) | Удалить `sendSignal` из интерфейса/реализации/мока (sync идёт через `/api/v1/sync/*`) |
| F-12 | **P1** | `CMakeLists.txt:443-483` | `test_asan` и `test_tsan` собираются из `{TEST_NETWORK_MANAGER_SOURCES, test_networkmanager_gmock.cpp}` — в одном таргете оказываются **два `main()`** (`QTEST_MAIN` и собственный `int main`), и нет `Qt6::Test` в `target_link_libraries` → **линковка падает**. Именно эти таргеты должны были поймать F-07/F-10 | Разделить на два таргета: gmock-тесты под ASan/TSan (без `test_networkmanager.cpp`), Qt-тесты — отдельным бинарём с `Qt6::Test`. Убрать `gmock_main` из таргетов с собственным `main()` |
| F-13 | **P1** | `mainwindow.cpp:464-499`, `:654-659` | Тикет живёт 5 минут (`constants.StreamTicketTTL`). Тикет **не перевыпускается** ни по таймеру, ни по 401/ошибке воспроизведения: `onPlaybackError` просто показывает `QMessageBox`. Через 5 минут любого фильма playback молча умирает без восстановления | Запросить тикет заранее (на ~80 % TTL) и на `MPV_EVENT_END_FILE` с `reason==ERROR` / на 401 от `/stream` — один раз перезапросить тикет и перезагрузить файл с сохранением позиции |
| F-14 | **P1** | `mpvwidget.cpp:280`, `:572`; `CMakeLists.txt:20` | **Утечка тикета в логи**: `qDebug() << "MpvWidget: воспроизведение" << url;` печатает URL потока вместе с `?ticket=<подписанный тикет>`. `CMakeLists.txt` не задаёт `QT_NO_DEBUG_OUTPUT`/`QT_LOGGING_RULES` для Release → вывод идёт в stderr. Второй путь: `MPV_LOG_LEVEL_ERROR` → `tr("MPV: %1")` → `QMessageBox::critical`, а mpv обычно включает URL в текст ошибки | Логировать только `m_serverUrl`+путь без query (`QUrl::setQuery(QString())` перед логом). Тексты ошибок mpv прогонять через редактор redaction (`[?&]ticket=[^&]+` → `?ticket=***`) перед показом |
| F-15 | **P1** | `mpvwidget.cpp:422-437` | `event()` перехватывает **любой** `QEvent::User` и возвращает `true`, ничего не делая. Кто угодно, кто сделает `postEvent(widget, …User…)`, потеряет событие молча | Удалить перехват (таймер `m_eventTimer` уже дёргает `onMpvEvents`) либо фильтровать по своему `QEvent::Type` |
| F-16 | **P1** | `mainwindow.cpp:548-563`; отсутствие элементов управления скоростью | `CONCEPT.md` требует «при перемотке и **изменении скорости** приложение догоняет общий момент **плавно**, а не рывком, с учётом задержки сети». Реализовано ровно обратное: мгновенный `seek(position)`, элементов управления скоростью нет вообще | Добавить «плавный догон»: целевая позиция = `position + RTT/2`, скорость mpv временно поднимается (например ×1.5) до момента схождения; добавить контрол скорости с пробросом в комнату |
| F-17 | **P2** | `networkmanager.cpp:45-54`, `:770-828` | Не задана `QNetworkRequest::RedirectPolicyAttribute`. Политика по умолчанию — `NoLessSafeRedirectPolicy`, но поведение заголовков `X-Access-Token`/`X-Client-ID` при кросс-хостовом редиректе нигде не контролируется приложением | Явно выставить политику (`ManualRedirectPolicy` или `NoLessSafeRedirectPolicy`) и проверять, что хост редиректа совпадает с `m_serverUrl.host()`, иначе — отказ |
| F-18 | **P2** | `networkmanager.cpp:672` | Режим `SSL_MODE=allow-self-signed` принимает self-signed **только** при `m_serverUrl.host() == "localhost"`. Сервер из `CONCEPT.md`/USER_GUIDE поднимают по LAN-IP/имени хоста → в dev-режиме TLS не заработает вовсе | Разрешать self-signed для любого хоста, но **никогда** `HostNameMismatch` (как сейчас), и логировать; либо добавить явный `--allow-self-signed-for <host>` |
| F-19 | **P2** | `systemtray.cpp:18` | `m_menu = new QMenu(nullptr)` — без родителя; `QSystemTrayIcon::setContextMenu` не берёт владение, `~SystemTray` меню не удаляет → утечка `QMenu` + 3 `QAction` + иконок | `new QMenu(this)` |
| F-20 | **P2** | `mainwindow.cpp:89-95`, `:321-327` | Список торрентов запрашивается только при старте и при `onServerAvailable`; периодического обновления нет. Прогресс/статус/скорость в UI **замерзают** навсегда | Добавить таймер периодического `listTorrents()` (2–5 с) и `TorrentModel::updateTorrentFromJson` вместо полного сброса модели |
| F-21 | **P2** | `mainwindow.cpp:411-439` | `onFilesReceived` не проверяет, что ответ соответствует текущему торренту: при быстром переключении список файлов от «старого» ответа перетирает «новый», `m_fileSizes` рассинхронизируется с моделью | Сравнивать `torrentId` с `m_torrentManager->currentTorrentId()` и игнорировать устаревшие ответы |
| F-22 | **P2** | `mainwindow.cpp:594-607` | На каждый тик `sliderMoved` уходит синхронный-по-воле HTTP `POST /buffer/position` → до 60 запросов/сек на перемотке (и ровно на пределе rate-limit 60/мин backend'а) | Отправлять позицию по debounce-таймеру (≥500 мс) либо только в `sliderReleased` |
| F-23 | **P2** | `mainwindow.cpp:661-665` | На **каждую** ошибку открывается модальный `QMessageBox`. При retry/backoff и обрывах SSE пользователь получит лавину диалогов | Дедуплицировать по сообщению с cooldown; некритичные ошибки — в status bar/трей, модалка только для действий пользователя |
| F-24 | **P2** | `inetworkmanager.h`, `torrentmanager.h:41`, `roommanager.h:39` | Интерфейс неполон (нет сигнала `fileSelected`) и **фактически не используется**: `TorrentManager`/`RoomManager` принимают конкретный `NetworkManager*`, поэтому `MockNetworkManager` нельзя подставить — ради чего он и написан | Перевести менеджеры на `INetworkManager*`, добавить `fileSelected` в интерфейс; тогда gmock-тесты реально что-то проверят |
| F-25 | **P2** | `mainwindow.cpp:759-775` | `closeEvent` вызывает `leaveRoom()` (асинхронный POST) и сразу завершает приложение → запрос не успевает уйти, сервер считает пира в комнате до таймаута | Перед выходом дать SSE/POST завершиться (таймаут ~300 мс на `waitForBytesWritten`/`QEventLoop`), либо отправлять leave синхронно |
| F-26 | **P2** | `main.cpp:249-255` | В бинарник зашит дефолтный `--jwt-secret "dev-secret-key-…"` и передаётся бэкенду, который JWT не использует | Убрать флаг и дефолтную строку целиком; при необходимости — только чтение из окружения без fallback |
| F-27 | **P2** | `networkmanager.h:52-53`, `networkmanager.cpp:963-980` | Мёртвые `RequestType::Login/Register`, недостижимая ветка `Register` в `handleApiError`; текст 401 —英文 «Authentication required — please log in», хотя логина нет | Удалить enum-значения и ветку; текст заменить на «Неверный или отсутствующий токен доступа — перезапустите сервер» |
| F-28 | **P2** | `CMakeLists.txt:443-483`; `Makefile` (`test-frontend-gmock`, `contract-test-frontend`, `e2e-test`) | Рецепты собирают несуществующие таргеты `test_torrentmanager_gmock`, `test_roommanager_gmock`, `test_networkmanager_contract`, `test_torrentmanager_contract`, `test_roommanager_contract`, `test_e2e`; `\|\| true` маскирует ошибку → CI зелёный на пустоте | Либо добавить таргеты, либо убрать рецепты; `\|\| true` в test-рецептах убрать |
| F-29 | **P2** | `test_integration.cpp:13-18`, `CMakeLists.txt:520` | «Integration test» — заглушка `QVERIFY(true)`, и при этом зарегистрирован как `IntegrationTest` в ctest, создавая иллюзию покрытия | Удалить до появления настоящих тестов либо реализовать против `tests/e2e/qt_headless/test_e2e_headless.cpp` (который сейчас вообще не подключён к сборке) |
| F-30 | **P2** | `test_mpvwidget.cpp` (весь файл) | Ни одного содержательного теста: `QVERIFY(&widget != nullptr)`, `QVERIFY(true)`, `m_mpvAvailable` вычисляется и **не используется** (пропуск тестов при отсутствии mpv не реализован) | Тесты на реальное поведение: `initializeMpv()` → `ready()`, `play()` → `durationChanged`, `seek()` → фактическая позиция; либо явный `QSKIP` при `!m_mpvAvailable` |
| F-31 | **P2** | `test_networkmanager.cpp:384-453` | Четыре теста — театр: `testEmptyMagnetUri`/`testEmptyRoomName`/`testSpecialCharactersInRoomName` проверяют `QVERIFY(m_manager != nullptr)` / `QVERIFY(true)`; `testUnauthorized/forbidden/internalServerError` проверяют `parseJson`, а не обработку ошибок | Тесты на `error`-сигнал через локальный `QTcpServer`, отдающий 401/500, с `QSignalSpy` на `NetworkManager::error` и проверкой числа попыток retry |
| F-32 | **P2** | `roomdialog.cpp:189-201` | `password()` и `isHost()` зависят от **текущей вкладки**, а не от режима диалога: если пользователь переключит вкладку, `MainWindow::onCreateRoom` получит пустое имя или пароль от другой вкладки | Возвращать значения по `m_mode`, а не по `currentIndex()` |
| F-33 | **P3** | `CMakeLists.txt:281` и `:488` | `option(BUILD_GMOCK …)` объявлен дважды | Убрать второе объявление |
| F-34 | **P3** | `mpvwidget.cpp:141-161` | `paintGL()` вызывает `makeCurrent()`/`doneCurrent()` вокруг `mpv_render_context_render()`; внутри `QOpenGLWidget::paintGL` контекст уже текущую, а `doneCurrent()` выходит из вызова, на который Qt опирается при блите FBO | Оставить только `mpv_render_context_render(...)` |
| F-35 | **P3** | `mpvwidget.cpp` целиком | Опрос событий таймером 30 мс (`MPV_EVENT_TIMER_MS`) вместо `mpv_set_wakeup_callback()` + `mpv_wakeup()`; `mpv_render_context_set_update_callback` не используется, `mpv_render_context_update()` не вызывается | Wakeup-callback (будит GUI-поток) + `update()` из render-callback — стандартная схема mpv; убрать 30-миллисекундный таймер |
| F-36 | **P3** | `mpvwidget.cpp:279`, `:381-389` | `m_paused` пишется в `play()` **без** мьютекса, тогда как `pause()/resume()/processMpvEvent` — под мьютексом; `position()`/`duration()`/`isPaused()` читают `m_mpv` без блокировки до её взятия | Единый стиль: брать `m_mutex` во всех путях доступа к `m_mpv`/кэшированным полям |
| F-37 | **P3** | `mpvwidget.cpp:119-124` | Ошибка `mpv_render_context_create()` только логируется; `m_mpvGL` остаётся `nullptr` → `paintGL` молча ничего не рисует, пользователь видит чёрное окно без объяснения | Пробросить ошибку в сигнал `error()` и в status bar |
| F-38 | **P3** | `main.cpp:355-359` | `stopGoServer()` блокирует GUI до 5.3 с (`waitForFinished(5000)` + 300 мс) на выходе | Сократить до 1 с или завершать процесс асинхронно до выхода из `main` |
| F-39 | **P3** | `mainwindow.cpp:480-481` | В английском комментарии остался CJK-фрагмент: `…молча делать вид, что播放 начался` | Переписать комментарий |
| F-40 | **P3** | `torrentmodel.h:57-68` | `toJson()` сериализует `qint64` как `double` — потеря точности >2^53 и неверный тип для целочисленных полей при round-trip | Сериализовать через `QString::number` / `QJsonValue(qint64)`-совместимый путь |

---

## Что реально хорошо

Чтобы не выглядело придиркой ко всему подряд — здесь действительно сильные решения:

- **Санитизация перед построением URL сделана по требованию `AGENTS.md` и сделана правильно.**
  `buildApiPath()` (`networkmanager.cpp:35-43`), `streamUrl()` (`:379-394`) и
  `requestStreamTicket()` (`:396-413`) прогоняют пользовательские ID через
  `[^a-fA-F0-9]`, отказываются на пустом результате, и это **идемпотентно** для реального
  формата ID (info hash 40 hex, room ID 32 hex — подтверждено pact'ом и
  `validation.ValidateRoomID`). Это редкий случай, когда «strip, а не escape» безопасно,
  и оно здесь проверено по обе стороны контракта.
- **`setServerUrl()` (`networkmanager.h:234-253`) — образцовая проверка ввода**: пустой URL
  отклоняется, недопустимая схема (`file://`, `ftp://`) отклоняется с явным предупреждением
  *и не повышается молча до https* (это ровно тот случай, где «helpful» upgrade уводил бы токен
  на другой хост), а http на не-localhost даёт предупреждение о передаче токена открыто.
- **TLS-политика в `onSslErrors` (`networkmanager.cpp:656-685`) продумана до мелочей**: в
  `AllowSelfSigned` принимаются **только** `SelfSignedCertificate` и
  `SelfSignedCertificateInChain`, `HostNameMismatch` не принимается никогда (это ровно
  правильная граница — самоподписанный сертификат для MITM не годится), и режим включается
  только через явный `SSL_MODE=allow-self-signed`, по умолчанию `Strict`.
- **Обработка кодов возврата mpv.** Везде, где есть риск, результат проверяется:
  `mpv_create()`, `mpv_initialize()` (с `mpv_terminate_destroy` на ошибке),
  `mpv_set_option_string` в `setOpt`, `mpv_command_async` для `loadfile` и `seek`,
  `mpv_render_context_create`. Для C-API, где код возврата игнорируют сплошь и рядом, это
  выше среднего.
- **Порядок разрушения в `~MpvWidget` (`mpvwidget.cpp:53-94`) правильный**: сначала
  атомарный флаг `m_destroying`, потом снятие render-callback, потом остановка таймеров, и
  только затем — под мьютексом — `mpv_render_context_free()` **до** `mpv_terminate_destroy()`.
  Это ровно тот порядок, который требует libmpv, и он выдержан.
- **`QPointer`-дисциплина в асинхронных путях.** `m_sseReply` — `QPointer`,
  SSE-лямбда захватывает `QPointer<NetworkManager> self` и проверяет `if (!self) return;`
  перед каждым обращением (`networkmanager.cpp:878-903`), `g_serverProcess` — `QPointer<QProcess>`
  с проверками в каждом колбэке (`main.cpp:258-321`). Это редкая внимательность.
- **Защита от DoS на входе.** Лимит 10 МБ на тело ответа (плюс дублирующая проверка по
  `Content-Length`), 64 КБ на тело ошибки, 1 МБ / 10000 итераций / 100 МБ на SSE-поток,
  лимит на размер `.torrent` (1 МБ, совпадает с `validation.MaxTorrentFileSize`),
  `MaxBytesReader` симметрично на стороне бэкенда. Плюс rate-limit 60/мин на бэкенде.
- **Backoff с jitter.** `calculateRetryDelay()` и SSE-reconnect используют
  `base * 2^attempt * (0.5 + rand())` с потолком — корректный ant thundering-herd.
- **`RequestType` в `RetryRequest`, а не регулярка по URL** — переход к явной типизации
  маршрутов был правильным шагом; проблема в том, что контекст общий (F-09), а не в самом
  решении.
- **Единый `INetworkManager` + gmock-мок как стратегия** — правильная идея, даже если
  подключение не доведено до конца (F-24).
- **Валидация входных сигналов комнаты.** `RoomManager::onSignalReceived`
  (`roommanager.cpp:118-139`) проверяет `action` по белому списку `{"play","pause","seek"}`
  и проверяет `std::isfinite(position) && position >= 0.0` — именно та проверка, которую
  обычно забывают на пути «данные от соседа по комнате».
- **Встраивание backend с проверкой целостности.** `extractEmbeddedBackend()` с SHA-256
  (`EMBEDDED_BACKEND_SHA256`), `QTemporaryDir` + `QTemporaryFile` вместо предсказуемого
  имени, и явный `message(WARNING ...)` в CMake, если хеш не задан.
- **Graceful shutdown по сигналам сделан правильно с точки зрения async-signal-safety**:
  обработчики только ставят `QAtomicInt`, а `QApplication::quit()` вызывается из таймера в
  event loop (`main.cpp:91-94`, `:545-553`) — это редкое понимание того, что Qt-API нельзя
  трогать из обработчика сигнала.
- **Безопасная передача пароля.** `createRoom`/`joinRoom` (`networkmanager.cpp:264-300`)
  отказываются отправлять пароль по http на не-localhost хост — сознательное решение,
  а не упущение.
- **Hardening сборки** в `CMakeLists.txt`: `-fstack-protector-strong -D_FORTIFY_SOURCE=2`,
  `-Wl,-z,relro,-z,now`, `/guard:cf /DYNAMICBASE /NXCOMPAT /HIGHENTROPYVA`.
- **`testSelectFileEmitsFileSelected`** (`test_networkmanager.cpp:631-673`) — единственный
  настоящий end-to-end тест фронтенда: поднимает `QTcpServer`, дожидается асинхронного ответа
  через `QTRY_COMPARE_WITH_TIMEOUT` и проверяет **все три** аргумента сигнала, включая
  построенный URL. Это правильный шаблон; остальные тесты должны выглядеть так же.

---

## Рекомендуемые work items

Порядок — по зависимостям: сначала то, что разблокирует проверку всего остального, затем
функциональность, затем качество.

1. **Собрать фронтенд в CI и починить тестовую обвязку.**
   *Что:* добавить в `.github/workflows` шаг `cmake -DBUILD_TESTS=ON && make && ctest` для
   Linux; попутно исправить `test_asan`/`test_tsan` (F-12), убрать `|| true` из test-рецептов
   `Makefile` (F-28), удалить дубль `option(BUILD_GMOCK)` (F-33), удалить заглушку
   `test_integration` (F-29).
   *Зачем:* весь остальной список невозможно верифицировать без компилятора; ASan/TSan —
   единственный автоматический способ поймать F-07/F-10.
   *Размер:* **S** (правки CMake/Makefile + один CI-шаг).
   *Файлы:* `frontend/CMakeLists.txt`, `Makefile`, `.github/workflows/*`.

2. **Починить `mpv_observe_property` (F-01) и вызов событий.**
   *Что:* после `mpv_initialize()` подписаться на `time-pos` (DOUBLE), `duration` (DOUBLE),
   `pause` (FLAG), `eof-reached` (FLAG); проверить код возврата каждого вызова.
   *Зачем:* без этого не работают ни позиция, ни длительность, ни перемотка, ни синхронизация —
   то есть не работает продукт. Это **самый дешёвый** фикс с **самым большим** эффектом.
   *Размер:* **S** (10–15 строк).
   *Файлы:* `frontend/src/mpvwidget.cpp`.

3. **Починить разрыв сигнальных цепочек комнаты (F-02, F-06) и SSE-маршрутизацию (F-05).**
   *Что:* соединить `NetworkManager::roomCreated/roomJoined/roomLeft` → слоты `RoomManager`,
   которые пишут `m_currentRoomId`/`m_isHost` и ре-эмитят; открывать SSE в ветке `CreateRoom`;
   роутить `type=="sync"` в `signalReceived` и читать позицию из `data.status.position`.
   *Зачем:* это ровно те три обрыва, которые убивают вторую главную фичу `CONCEPT.md`.
   *Размер:* **M** (30–50 строк + тесты на разрыв/сборку цепочки).
   *Файлы:* `frontend/src/roommanager.{h,cpp}`, `frontend/src/networkmanager.cpp`,
   `frontend/src/mainwindow.cpp`.

4. **Починить контракт списков (F-04) и ID комнаты при join (F-03).**
   *Что:* разбирать конверты `{torrents:…}`/`{files:…}`; брать `roomId` из собственного запроса,
   а не из ответа (и/или попросить бэкенду вернуть `id` в `POST /rooms/join`).
   *Зачем:* сейчас UI пустой всегда; join не открывает SSE. Дёшево, высокий эффект.
   *Размер:* **S** (10–20 строк + 2 теста).
   *Файлы:* `frontend/src/networkmanager.cpp`, `frontend/src/test_networkmanager.cpp`,
   и (опционально) `backend/internal/api/handlers_room.go` — за пределами моего write-scope,
   нужно согласование.

5. **Убрать вызовы сигналов под мьютексом (F-10) и починить retry-контекст (F-09), утечку reply (F-08).**
   *Что:* вынести `emit` за пределы `QMutexLocker`-scope (все 5 мест в `networkmanager.cpp`);
   перенести retry-контекст в `QHash<QNetworkReply*, RetryRequest>`; добавить безусловный
   `reply->deleteLater()` через scope-guard.
   *Зачем:* дедлок с модальным диалогом — это зависание приложения, а дублирующий retry —
   это двойное добавление торрента или повторное удаление.
   *Размер:* **M**.
   *Файлы:* `frontend/src/networkmanager.{h,cpp}`.

6. **Починить рекурсивную блокировку в `showEvent` (F-07).**
   *Что:* убрать внешний `QMutexLocker` в лямбде (`initializeMpv()` уже синхронизирован сам).
   *Зачем:* взаимное исключение в mpv-виджете сейчас не работает; это единственная правка,
   *полностью локализованная в одной функции.
   *Размер:* **S** (2 строки).
   *Файлы:* `frontend/src/mpvwidget.cpp`.

7. **Жизненный цикл stream-тикета (F-13, F-14).**
   *Что:* предварительный запрос тикета на ~80 % TTL; одноразовый перевыпуск на 401/ошибке
   `END_FILE` с восстановлением позиции; редакция `ticket=` в `qDebug` и в тексте ошибок mpv;
   URL-экранирование тикета (`QUrl::toPercentEncoding`) и запрет точки в `--client-id`
   (иначе `strings.Split(ticket,".")` даёт ≠4 части на бэкенде).
   *Зачем:* сейчас любое видео умирает через 5 минут, а тикет оседает в логах.
   *Размер:* **M**.
   *Файлы:* `frontend/src/mpvwidget.cpp`, `frontend/src/mainwindow.cpp`,
   `frontend/src/networkmanager.cpp`.

8. **Перевести менеджеры на `INetworkManager` и дописать интерфейс (F-24).**
   *Что:* `fileSelected` в интерфейс; `TorrentManager`/`RoomManager` принимают
   `INetworkManager*`; после этого gmock-тесты начинают проверять реальные связи.
   *Зачем:* сейчас mock существует только ради mock'а; это блокирует нормальное
   тестирование P2P-логики, которая как раз и сломана.
   *Размер:* **M**.
   *Файлы:* `frontend/src/interfaces/inetworkmanager.h`, `frontend/src/torrentmanager.h`,
   `frontend/src/roommanager.h`, `frontend/src/mainwindow.cpp`.

9. **Переписать тесты, которые ничего не проверяют (F-30, F-31).**
   *Что:* вместо `QVERIFY(m_manager != nullptr)` — `QSignalSpy` на `NetworkManager::error`
   против локального `QTcpServer`, отдающего 401/500, с проверкой числа попыток; в
   `test_mpvwidget.cpp` — реальные проверки mpv-поведения либо явный `QSKIP`.
   *Зачем:* зелёный ctest сейчас не означает ничего; именно поэтому P0 пролетели мимо CI.
   *Размер:* **M**.
   *Файлы:* `frontend/src/test_networkmanager.cpp`, `frontend/src/test_mpvwidget.cpp`.

10. **Плавный догон и управление скоростью (F-16).**
    *Что:* целевая позиция с поправкой на RTT/2, временное ускорение mpv до схождения;
    контрол скорости с пробросом в комнату.
    *Зачем:* прямое требование `CONCEPT.md`, сейчас не реализовано вовсе (синхронизация
    делает рывок).
    *Размер:* **M** (UI) + **S** (сетевая часть).
    *Файлы:* `frontend/src/mainwindow.{h,cpp}`, `frontend/src/mpvwidget.{h,cpp}`,
    `frontend/src/roommanager.cpp`.

11. **Мелкие исправления P2/P3 (F-17 … F-20, F-22, F-23, F-25 … F-27, F-32, F-34 … F-40).**
    *Что:* redirect-policy, AllowSelfSigned для не-localhost, утечка `QMenu`, периодический
    `listTorrents()`, дедупликация ошибок без модалок, debounce для `buffer/position`,
    `closeEvent` с ожиданием leave, удаление мёртвого `sendSignal`/`RequestType::Login`,
    починка `RoomDialog::password()`, `paintGL` без `makeCurrent`, mpv wakeup-callback.
    *Зачем:* каждый пункт дешёвый; в сумме снимают шум, предупреждения и часть P2-рисков.
    *Размер:* **L** суммарно, но каждый элемент — **S**.
    *Файлы:* перечисленные в таблице findings.

12. **Синхронизировать `docs/API.md` с фактическим контрактом (зона docs-агента).**
    *Что:* `GET /api/v1/torrents` и `/files` возвращают конверт (не массив);
    `POST /api/v1/rooms/join` возвращает только `{"message":…}`; `GET /buffer/info` возвращает
    `buffered_percent` и ещё 8 полей; `POST /rooms/signal` принимает `{roomId, signal}` с
    `signal` в base64; SSE-событие синхронизации — `type:"sync"`; раздел «Security» всё ещё
    обещает JWT/CSRF/bcrypt, что противоречит `AGENTS.md` и коду.
    *Зачем:* `docs/API.md` сейчас вводит в заблуждение именно в тех местах, где фронтенд
    и разошёлся с сервером (строки 164-178, 249-261, 380-385, 481-486, 520-524, 680-689).
    *Размер:* **S**.
    *Файлы:* `docs/API.md` (вне моего write-scope — передать docs-агенту).

---

### Приложение: быстрая карта «что сломалось и почему»

```
Пользователь: «добавил торрент, нажал play»
  → POST /select ..................... OK
  → POST /stream-ticket .............. OK  (ticket + torrentId совпадают)
  → libmpv GET /stream?ticket= ....... OK  (но тикет утёк в qDebug, через 5 мин протухнет)

Пользователь: «двойной клик по торренту»
  → GET /files ....................... СЛОМАНО (конверт {files:…} ≠ массив)
     → список файлов пуст → выбор файла невозможен
  → список торрентов ................. СЛОМАНО (то же) → окно всегда пустое

Пользователь: «перемотал»
  → duration == 0 ................... СЛОМАНО (нет mpv_observe_property)
     → onSeek() выходит по m_duration <= 0 → перемотка не работает

Пользователь: «создал комнату / вошёл в комнату»
  → RoomManager.roomCreated ........ СЛОМАНО (сигнал не эмитится)
     → isInRoom() == false навсегда
     → UI комнаты не обновляется, кнопка «Покинуть» не активируется
     → syncPlay/Pause/Seek не отправляются никогда
  → SSE при create .................. СЛОМАНО (connectToSSE не вызывается)
  → SSE при join .................... СЛОМАНО (в ответе нет id → путь /rooms//events)
  → входящие sync-события ........... СЛОМАНО (тип "sync", а обрабатывается "signal")
```