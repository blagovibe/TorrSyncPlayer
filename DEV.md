# Dev — как устроен TorrSyncPlayer

Что проект делает и зачем — в [CONCEPT.md](CONCEPT.md). Этот файл отвечает на
вопрос «как его собрать, запустить и править» и не дублирует то, что и так
выводится из кода.

## Почему здесь нет справочника API

Формы запросов и ответов **не описаны вручную**. В коде стоят аннотации
`@Router`, `@Param`, `@Success`, из которых генерируется спецификация:

- `/swagger/index.html` на запущенном сервере — интерактивный вариант;
- `backend/docs/swagger.json` и `swagger.yaml` — тот же самый OpenAPI.

Спека перегенерируется командой и проверяется в CI на свежесть: если
аннотации разошлись с файлом, сборка падает. Поэтому она не может разойтись с
кодом — а ручной справочник может и разойдётся.

Чтобы посмотреть справочник, запустите сервер и откройте `/swagger/`.
Фильтр `TRUSTED_PROXIES`, список SSE-событий и правила rate limit описаны
ниже — это то, чего в спеке нет и что из кода не выводится.

Перегенерация после правки аннотаций:

```bash
cd backend
go run github.com/swaggo/swag/cmd/swag@v1.16.6 \
  init --generalInfo cmd/server/main.go --output docs --parseDependency
```

## Системные требования

| | Минимум | Рекомендуется |
|---|---|---|
| ОС | Windows 10+, Ubuntu 20.04+, macOS 12+ | Windows 11, Ubuntu 22.04+, macOS 13+ |
| ОЗУ | 8 ГБ | 16 ГБ |
| Диск | 500 МБ | SSD, 1 ГБ свободно |

Данные торрентов по умолчанию лежат **в памяти** — объём ограничен
`--memory-capacity` (максимум 256 ГБ). На диск их кладут только явно,
см. «Хранение данных».

## Зависимости

### Бэкенд — Go 1.26+

```bash
# Ubuntu/Debian
wget https://go.dev/dl/go1.26.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# macOS
brew install go
```

Windows — установщик с [go.dev](https://go.dev/dl/).

### Фронтенд — Qt 6 и libmpv

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install -y \
    build-essential cmake ninja-build \
    qt6-base-dev qt6-multimedia-dev \
    libmpv-dev libgl1-mesa-dev

# macOS
brew install qt@6 mpv cmake ninja
```

Windows — Qt 6.5+ с [qt.io](https://www.qt.io/download) и libmpv через vcpkg.

> **libmpv — зависимость во время выполнения.** Без неё приложение
> собирается, но **воспроизведение отключено**: при выборе файла показывается
> предупреждение вместо видео. Об этом честно сообщает сам `MainWindow`.

## Сборка

Версия Go закреплена в `GO_VERSION` во всех workflow — это единственное
место, где она меняется.

```bash
# Бэкенд → backend/build/torrsyncplayer
cd backend && make build

# Фронтенд
cd frontend && mkdir -p build && cd build
cmake .. -G Ninja -DCMAKE_BUILD_TYPE=Release
ninja
```

## Запуск

```bash
cd backend/build
./torrsyncplayer
```

Сервер печатает в лог случайный **токен доступа** при каждом старте. Он
передаётся клиентам в заголовке `X-Access-Token`; ни аккаунтов, ни паролей, ни
регистрации нет. Переопределить можно флагом `--access-token`.

### Флаги

| Флаг | Назначение |
|---|---|
| `--port` | порт прослушивания (по умолчанию `8889`) |
| `--addr` | адрес прослушивания |
| `--token` | зафиксировать токен вместо случайного |
| `--data-dir` | каталог для персистентных данных |
| `--disk-storage` | писать куски торрента на диск (требует `--data-dir`) |
| `--memory-capacity` | лимит памяти под куски, до 256 ГБ |
| `--tls-cert`, `--tls-key` | сертификат и ключ для TLS |
| `--tls` | включить TLS с самоподписанным сертификатом |
| `--auto-tls` | сгенерировать временный самоподписанный сертификат |
| `--enable-profiling` | pprof |

### Переменные окружения

`PORT`, `DATA_DIR`, `DISK_STORAGE`, `MEMORY_CAPACITY`, `LOG_LEVEL`,
`TLS_CERT`, `TLS_KEY`, `API_RATE_LIMIT`, `API_RATE_BURST`,
`TRUSTED_PROXIES`.

Адрес прослушивания и сам токен задаются **только флагами** (`--addr`,
`--token`) — переменных окружения для них нет.

`TRUSTED_PROXIES` — список CIDR, которым **доверять** заголовку
`X-Forwarded-For`. Пока переменная не задана, заголовок игнорируется
полностью: иначе любой клиент из приватной сети мог бы подменить свой
IP. Формат — `10.0.0.0/8,192.168.0.0/16`.

## Архитектура

Два приложения: Go-бэкенд и Qt/C++-фронтенд. Фронтенд не содержит
бизнес-логики — он ходит в бэкенд по HTTP и получает события комнаты по SSE.

### Бэкенд (Go)

Слои: `cmd/server` (сборка сервисов и graceful shutdown) → `internal/api`
(роутинг, обработчики, middleware) → сервисы предметной области
(`internal/*`) → `pkg/` (журнал и формирование ответов).

| Пакет | Ответственность |
|---|---|
| `internal/api` | Роутинг chi, обработчики, middleware (security headers, recovery, CORS, content-type, логирование, rate limit), константы путей |
| `internal/auth` | Per-process access token, constant-time compare, HMAC stream tickets |
| `internal/torrent` | Управление торрентами, HTTP-стриминг через anacrolix/torrent |
| `internal/buffer` | LRU-буфер с приоритетами частей для немедленного воспроизведения |
| `internal/p2p` | Комнаты, участники, сессии; брокер событий комнаты по SSE |
| `internal/sync` | Синхронизация позиции воспроизведения с компенсацией задержки |
| `internal/persistence` | JSON-файлы: комнаты и состояние синхронизации |
| `internal/storage` | Абстракция хранилища данных торрентов |
| `internal/validation` | Общая валидация входных данных (идентификаторы комнат, размеры) |
| `internal/models` | Общие типы данных и структуры ответов |
| `internal/errors` | Структурированные ошибки AppError с типом ошибки |
| `internal/metrics` | Метрики Prometheus |
| `internal/constants` | Все магические числа проекта |
| `internal/utils` | Общие утилиты (идентификаторы, Debouncer) |
| `internal/version` | Информация о версии сервера |
| `internal/interfaces.go` | Интерфейсы, через которые api обращается к сервисам |
| `internal/contract` | Pact-проверка контракта (за build-тегом `contract`) |
| `pkg/logger` | slog-журнал с контекстом операции |
| `pkg/response` | Формирование JSON-ответов |

> У `internal/persistence` остались `SaveUsers` и `SaveRevokedTokens` — это
> наследие от схемы с аккаунтами и отзывами токенов. **Их никто не вызывает**;
> живые вызовы — только `SaveRooms` и `SaveSync`. Удалить вместе с
> соответствующими типами моделей.

Порядок middleware: SecurityHeaders → Recovery → CORS → ContentType →
Logger → RateLimit → Token.

Транспорт синхронизации: клиент шлёт команды по REST, события комнаты
получает по SSE через сервер. Прямых соединений между участниками нет, STUN и
TURN не нужны.

### Поток «добавил торрент и смотрю»

```
Frontend                 Backend                 Torrent Client
   │                        │                        │
   │  POST /torrents        │                        │
   │  {magnetUri}           │                        │
   │───────────────────────►│                        │
   │                        │  AddMagnet()           │
   │                        │───────────────────────►│
   │                        │  GotInfo()             │
   │                        │◄───────────────────────│
   │  201 {torrentInfo}     │                        │
   │◄───────────────────────│                        │
   │  GET /torrents/{id}/files                       │
   │───────────────────────►│                        │
   │  200 {files[]}         │                        │
   │◄───────────────────────│                        │
   │  POST /torrents/{id}/select                     │
   │  {fileIndex}           │                        │
   │───────────────────────►│  SetPriority()         │
   │                        │───────────────────────►│
   │  GET /torrents/{id}/stream                      │
   │───────────────────────►│  ServeFile()           │
   │                        │───────────────────────►│
   │  200 (video stream)    │                        │
   │◄───────────────────────│                        │
   │  MpvWidget.play(url)   │                        │
```

### Поток «комната и синхронизация»

```
User A (Host)            Backend                  User B (Peer)
   │                        │                        │
   │  POST /rooms           │                        │
   │  {name, password}      │                        │
   │───────────────────────►│                        │
   │  201 {roomInfo}        │                        │
   │◄───────────────────────│                        │
   │  GET /rooms/{id}/events│                        │
   │  (SSE connect)         │                        │
   │◄═══════════════════════│                        │
   │                        │    POST /rooms/join    │
   │                        │    {roomId, password}  │
   │                        │◄───────────────────────│
   │  SSE: peer_joined      │    200 {message, id}   │
   │◄═══════════════════════│───────────────────────►│
   │                        │    GET /rooms/{id}/events
   │                        │    (SSE connect)       │
   │                        │═══════════════════════►│
   │  POST /sync/play       │                        │
   │───────────────────────►│                        │
   │  200 {syncStatus}      │                        │
   │◄───────────────────────│                        │
   │  BroadcastSync (SSE) ──────────────────────────►│
```

### События SSE

Сервер шлёт `peer_joined`, `peer_left`, `signal`, `sync` и `ping` (последний
не несёт данных, нужен чтобы прокси не рвали соединение).

### Компенсация задержки

`internal/sync` догоняет общий момент плавно, а не рывком:

```
1. Взять статус удалённого участника (позиция, метка времени, playing)
2. Ожидаемая позиция:
   - играет: expected = position + elapsed - latency
   - пауза:  expected = position
3. Плавная подстройка:
   - |diff| > maxPositionJump (2 сек): position += diff * 0.3
   - иначе: position = expected
4. Синхронизировать состояние play/pause
```

### Хранение данных

По умолчанию куски торрента лежат в памяти. Чтобы писать на диск:

```bash
./torrsyncplayer --data-dir ./data --disk-storage
```

или через `DATA_DIR` + `DISK_STORAGE=true`. Кучи пишутся в
`<DATA_DIR>/torrents`. Лимит памяти — `--memory-capacity`, максимум 256 ГБ
(`MaxMemoryStorageCapacity`).

## Фронтенд (Qt/C++)

- `mainwindow` — окно, список торрентов, диалоги комнаты
- `mpvwidget` — воспроизведение видео через libmpv
- `networkmanager` — HTTP-обмен с бэкендом, SSE-подписка на события комнаты
- `roommanager` — состояние комнаты на стороне клиента
- `torrentmanager` / `torrentmodel` — управление торрентами и их отображение
- `utils` — вспомогательные функции (форматирование времени и размера)
- `systemtray` — значок в системном лотке
- `interfaces/` — интерфейсы для тестируемости, `mocks/` — их заглушки

### Многопоточность libmpv

Все вызовы `mpv_*` защищены `QMutex`. События mpv буферизуются и
эмитятся в главном потоке через `QTimer`, чтобы Qt-сигналы не приходили из
чужого потока. Перемотка отложена на debounce-таймер, чтобы быстрая
перемотка мышью не превращалась в поток команд.

Отложенная перемотка переживает открытие файла: команда `seek` уходит в mpv
через 300 мс, а `loadfile` асинхронен, и поток из торрента открывается
дольше. Если файл к моменту команды ещё не загружен, mpv её отбрасывает —
поэтому `MPV_EVENT_FILE_LOADED` применяет неприменённую позицию заново.

## Технологический стек

| Слой | Технологии и версии |
|---|---|
| Бэкенд | Go 1.26, anacrolix/torrent v1.61.0, go-chi/chi/v5, swaggo/swag v1.16.6 |
| Фронтенд | C++17, Qt 6, libmpv, CMake 3.16+ |
| Тесты бэкенда | go test, testify, race-детектор, go-mutesting, pact-go |
| Тесты фронтенда | Qt Test, gtest/gmock, sanitizers (ASan/TSan/UBSan) |
| Сборка | Make (бэкенд), CMake (фронтенд) |
| CI/CD | GitHub Actions: lint, тесты, контракт, мутации, sanitizers, сборка на Linux/macOS/Windows, CodeQL, security-сканеры |
| Лицензия | MIT |

## Принципы разработки

1. Магические числа живут только в `internal/constants/constants.go`.
2. Ошибки — структурированные, из `internal/errors`; тип проверяется через
   `errors.As`, прямым type assertion пользоваться нельзя.
3. Интерфейсы объявляются в `internal/interfaces.go`, а не рядом с
   реализацией: `api` не должен знать устройство сервисов.
4. Аккаунтов нет. Токен доступа один на процесс, сравнение —
   `crypto/subtle.ConstantTimeCompare`. Идентификаторы комнат
   нормализуются в нижний регистр.
5. Всё, что приходит от пользователя, валидируется и санируется; в URL
   попадают только проверенные значения.
6. Члены классов во фронтенде именуются с префиксом `m_`, методы — в
   camelCase.
7. События комнаты идут через сервер; прямые P2P-соединения не вводятся.
8. То, что выводится из кода, не переписывается руками: формы API живут в
   Swagger, метрики — в `internal/metrics`.

## Проверки должны уметь краснеть

Зелёная сборка ценна только тогда, когда сломанная может стать красной.
Поэтому вне осознанного исключения запрещены:

- `|| true` в рецепте Makefile;
- `continue-on-error: true` у шага или джобы CI;
- `-` в конце команды в рецепте;
- тест, который проверяет только не-nil или повторяет собственную логику.

Два бага дошли до `main` при полностью зелёном пайплайне именно поэтому: SSE
отдавал 500, потому что обёртка middleware теряла `http.Flusher`, и подпись
stream-тикета считалась над пустой строкой, то есть не проверяла ничего.
Ни одна проверка этого не видела.

Если проверка выглядит нестабильной — чинить нестабильность, а не
заглушать её.

## Структура репозитория

```
TorrSyncPlayer/
├── CONCEPT.md         # зачем проект существует и его границы
├── DEV.md             # этот файл: как собрать, запустить и править
├── README.md          # входная точка: что это и как начать
├── CHANGELOG.md       # журнал изменений (append-only)
├── backend/           # Go-бэкенд
│   ├── cmd/server/    # точка входа
│   ├── internal/      # пакеты предметной области и HTTP-слой
│   ├── pkg/           # журнал и ответы
│   └── docs/          # сгенерированная спецификация Swagger
├── frontend/          # Qt/C++-фронтенд
│   ├── src/           # исходники
│   └── resources/     # ресурсы
├── tests/             # chaos (toxiproxy), нагрузка (k6), e2e (Playwright, Qt headless)
├── pacts/             # контракт между фронтендом и бэкендом
└── .github/           # workflow и шаблоны
```

## Соглашения по тестам

Бэкенд:

```bash
cd backend
make test              # весь набор
make test-race         # с race-детектором
make test-coverage     # покрытие
make test-fuzz         # fuzzing
make test-mutation     # мутационное покрытие
```

Фронтенд:

```bash
cd frontend/build
ctest --output-on-failure
```

Контракт (нужна нативная библиотека pact FFI, качается в CI автоматически):

```bash
cd backend
go test -tags contract -run PactProvider ./internal/contract/...
```

Мутационное покрытие ограничено пакетами `validation`, `sync`, `p2p`,
`persistence`, `utils`, `buffer` — полный прогон по всему бэкенду занимает
часы ради числа, которое никто не смотрит. Порог задан в джобе
Mutation Testing и должен пересчитываться при изменении списка пакетов.

`make test-all` собирает фронтенд и запускает контракт. Pact-потребитель на
фронтенде не реализован, поэтому соответствующий шаг честно печатает
`SKIPPED` — он не собирает несуществующие таргеты и не может стать красным
ни по какой причине.

## Запуск как службы

### Linux (systemd)

`/etc/systemd/system/torrsyncplayer.service`:

```ini
[Unit]
Description=TorrSyncPlayer Server
After=network.target

[Service]
Type=simple
User=torrsyncplayer
WorkingDirectory=/opt/TorrSyncPlayer
ExecStart=/opt/TorrSyncPlayer/build/torrsyncplayer
Restart=on-failure
RestartSec=5
Environment=PORT=8889

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now torrsyncplayer
```

### macOS (launchd)

`~/Library/LaunchAgents/com.torrsyncplayer.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
    "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.torrsyncplayer</string>
    <key>ProgramArguments</key>
    <array>
        <string>/path/to/torrsyncplayer</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
```

```bash
launchctl load ~/Library/LaunchAgents/com.torrsyncplayer.plist
```

### Windows (NSSM)

```bash
nssm install TorrSyncPlayer "C:\path\to\torrsyncplayer.exe"
nssm start TorrSyncPlayer
```

## Docker

```bash
cd backend
docker build -t torrsyncplayer-server .

# в памяти
docker run -d --name torrsyncplayer \
  -p 8889:8889 -e LOG_LEVEL=info \
  torrsyncplayer-server

# с постоянным каталогом
docker run -d --name torrsyncplayer \
  -p 8889:8889 \
  -v torrsync-data:/data \
  -e DATA_DIR=/data -e LOG_LEVEL=info \
  torrsyncplayer-server

# с TLS
docker run -d --name torrsyncplayer \
  -p 8889:8889 \
  -v /path/to/certs:/certs:ro \
  -e TLS_CERT=/certs/cert.pem \
  -e TLS_KEY=/certs/key.pem \
  torrsyncplayer-server --tls
```