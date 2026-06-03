# Архитектура TorrSyncPlayer

## Содержание

1. [Обзор архитектуры](#обзор-архитектуры)
2. [Диаграмма компонентов](#диаграмма-компонентов)
3. [Backend архитектура](#backend-архитектура)
4. [Frontend архитектура](#frontend-архитектура)
5. [Схема коммуникации](#схема-коммуникации)
6. [Модель данных](#модель-данных)

---

## Обзор архитектуры

TorrSyncPlayer — десктопное приложение для потокового воспроизведения медиаконтента через торренты с возможностью синхронного просмотра между пользователями.

Архитектура следует клиент-серверной модели с P2P элементами:

- **Backend (Go)** — HTTP API сервер, управляющий торрентами, P2P комнатами и синхронизацией
- **Frontend (Qt/C++)** — десктопное приложение с видеоплеером на базе libmpv
- **P2P (WebRTC)** — прямое соединение между пирами для обмена данными синхронизации

---

## Диаграмма компонентов

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              TorrSyncPlayer                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        Frontend (Qt/C++)                            │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌───────────────────────────┐ │    │
│  │  │  MainWindow   │  │  MpvWidget   │  │     NetworkManager        │ │    │
│  │  │              │  │  (libmpv)    │  │  - HTTP REST API          │ │    │
│  │  │ - UI layout  │  │              │  │  - SSE events             │ │    │
│  │  │ - User input │  │ - Playback   │  │  - Retry logic            │ │    │
│  │  │ - State mgmt │  │ - Rendering  │  │  - Exponential backoff    │ │    │
│  │  └──────┬───────┘  └──────┬───────┘  └─────────────┬─────────────┘ │    │
│  │         │                 │                         │               │    │
│  │  ┌──────┴───────┐  ┌──────┴───────┐  ┌─────────────┴─────────────┐ │    │
│  │  │TorrentManager│  │  RoomManager │  │       TorrentModel         │ │    │
│  │  │              │  │              │  │  - Data model for list     │ │    │
│  │  │ - Add/Remove │  │ - Create/Join│  │  - Qt model/view           │ │    │
│  │  │ - Select file│  │ - Leave room │  │                            │ │    │
│  │  └──────────────┘  └──────────────┘  └───────────────────────────┘ │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                    │                                         │
│                          HTTP REST API / SSE                                 │
│                                    │                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                          Backend (Go)                               │    │
│  │                                                                     │    │
│  │  ┌──────────────────────────────────────────────────────────────┐   │    │
│  │  │                      API Layer (chi router)                   │   │    │
│  │  │  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐ │   │    │
│  │  │  │   Auth     │ │  Torrent   │ │   Room     │ │   Sync     │ │   │    │
│  │  │  │  Handlers  │ │  Handlers  │ │  Handlers  │ │  Handlers  │ │   │    │
│  │  │  └────────────┘ └────────────┘ └────────────┘ └────────────┘ │   │    │
│  │  │                                                              │   │    │
│  │  │  Middleware: SecurityHeaders → Recovery → CORS → Logger → CSRF│   │    │
│  │  └──────────────────────────────────────────────────────────────┘   │    │
│  │                                    │                                 │    │
│  │  ┌──────────────────────────────────────────────────────────────┐   │    │
│  │  │                      Service Layer                            │   │    │
│  │  │  ┌──────────────┐  ┌──────────────┐  ┌────────────────────┐  │   │    │
│  │  │  │   Torrent    │  │     P2P      │  │       Sync         │  │   │    │
│  │  │  │   Service    │  │   Service    │  │      Service       │  │   │    │
│  │  │  │              │  │              │  │                    │  │   │    │
│  │  │  │ - anacrolix/ │  │ - pion/webrtc│  │ - Play/Pause/Seek  │  │   │    │
│  │  │  │   torrent    │  │ - Rooms      │  │ - Latency comp.    │  │   │    │
│  │  │  │ - Magnet     │  │ - Peers      │  │ - Smooth adjust    │  │   │    │
│  │  │  │ - Streaming  │  │ - JWT auth   │  │                    │  │   │    │
│  │  │  └──────────────┘  └──────────────┘  └────────────────────┘  │   │    │
│  │  │  ┌──────────────┐  ┌──────────────┐                          │   │    │
│  │  │  │   Buffer     │  │   Storage    │                          │   │    │
│  │  │  │   Service    │  │   Service    │                          │   │    │
│  │  │  │              │  │              │                          │   │    │
│  │  │  │ - LRU cache  │  │ - In-memory  │                          │   │    │
│  │  │  │ - Priorities │  │ - Users      │                          │   │    │
│  │  │  └──────────────┘  └──────────────┘                          │   │    │
│  │  └──────────────────────────────────────────────────────────────┘   │    │
│  │                                    │                                 │    │
│  │  ┌──────────────────────────────────────────────────────────────┐   │    │
│  │  │                    Cross-cutting Concerns                     │   │    │
│  │  │  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐ │   │    │
│  │  │  │   Auth     │ │ Validation │ │  Metrics   │ │  Logging   │ │   │    │
│  │  │  │  Package   │ │  Package   │ │ (Prometheus│ │  Package   │ │   │    │
│  │  │  └────────────┘ └────────────┘ └────────────┘ └────────────┘ │   │    │
│  │  └──────────────────────────────────────────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘

        ┌──────────────────────────────────────────────────────────┐
        │                    P2P (WebRTC DataChannel)               │
        │                                                          │
        │    Peer A ◄──────────────────────────────────► Peer B    │
        │                                                          │
        │    - Синхронизация позиции воспроизведения               │
        │    - Обмен состоянием play/pause/seek                    │
        │    - STUN для NAT traversal                              │
        └──────────────────────────────────────────────────────────┘
```

---

## Backend архитектура

### Структура пакетов

```
backend/
├── cmd/server/           # Точка входа (main.go)
│
├── internal/
│   ├── api/              # HTTP API слой
│   │   ├── router.go     # Маршрутизация (chi)
│   │   ├── handlers.go   # Обработчики запросов
│   │   ├── middleware.go # Middleware (CORS, CSRF, Rate Limit)
│   │   ├── response.go   # Форматирование ответов
│   │   └── paths.go      # Константы путей API
│   │
│   ├── auth/             # Аутентификация
│   │   ├── auth.go       # JWT логика
│   │   ├── handlers.go   # Register/Login/Logout
│   │   ├── middleware.go # JWT middleware
│   │   ├── store.go      # Хранилище пользователей
│   │   └── revocation.go # Отзыв токенов
│   │
│   ├── torrent/          # Торрент сервис
│   │   └── service.go    # Управление торрентами
│   │
│   ├── p2p/              # P2P сервис
│   │   └── service.go    # WebRTC соединения
│   │
│   ├── sync/             # Сервис синхронизации
│   │   └── service.go    # Синхронизация воспроизведения
│   │
│   ├── buffer/           # Буферизация
│   │   └── service.go    # LRU кэш, приоритеты
│   │
│   ├── storage/          # Хранилище
│   │   └── storage.go    # In-memory хранилище
│   │
│   ├── models/           # Модели данных
│   │   └── types.go      # Общие типы
│   │
│   ├── validation/       # Валидация
│   │   └── validation.go # Функции валидации
│   │
│   ├── errors/           # Обработка ошибок
│   │   └── errors.go     # Типы ошибок
│   │
│   ├── metrics/          # Prometheus метрики
│   │   └── metrics.go    # Определения метрик
│   │
│   ├── constants/        # Константы
│   │   └── constants.go  # Магические числа
│   │
│   ├── version/          # Версия
│   │   └── version.go    # Информация о версии
│   │
│   └── interfaces.go     # Интерфейсы сервисов
│
└── pkg/logger/           # Логгер
    └── logger.go         # Структурированное логирование
```

### Взаимодействие сервисов

Сервисы спроектированы как независимые компоненты без DI-контейнера. Взаимодействие происходит через интерфейсы, определённые в [`internal/interfaces.go`](../backend/internal/interfaces.go):

```
┌─────────────────────────────────────────────────────────────────┐
│                        main.go                                  │
│                                                                 │
│  1. Инициализация сервисов:                                     │
│     - logger.Init()                                             │
│     - authService = auth.NewAuthService(jwtSecret)              │
│     - torrentService = torrent.NewService(bufferService)              │
│     - p2pService = p2p.NewService(authService)                  │
│     - syncService = sync.NewService()                           │
│     - bufferService = buffer.NewService()                       │
│                                                                 │
│  2. Создание роутера:                                           │
│     - router := api.NewRouter(RouterConfig{...})                │
│                                                                 │
│  3. Запуск HTTP сервера:                                        │
│     - http.ListenAndServe(port, router)                         │
│                                                                 │
│  4. Graceful shutdown:                                          │
│     - torrentService.Close()                                    │
│     - p2pService.Close()                                        │
│     - syncService.Close()                                       │
│     - bufferService.Close()                                     │
└─────────────────────────────────────────────────────────────────┘
```

**Принципы:**
- Каждый сервис имеет свой мьютекс для потокобезопасности
- Сервисы не зависят друг от друга напрямую
- Общение между сервисами происходит через HTTP API из frontend
- Graceful shutdown с таймаутами для корректного завершения

### HTTP API слой

**Маршрутизация** (на базе [go-chi/chi](https://github.com/go-chi/chi)):

| Путь | Метод | Описание | Аутентификация |
|------|-------|----------|----------------|
| `/health` | GET | Базовый health check | Нет |
| `/api/v1/version` | GET | Версия сервера | Нет |
| `/metrics` | GET | Prometheus метрики | Нет |
| `/api/v1/csrf-token` | GET | Получить CSRF токен | Нет |
| `/swagger/` | GET | Swagger UI | Нет |
| `/api/v1/auth/register` | POST | Регистрация | Нет |
| `/api/v1/auth/login` | POST | Вход | Нет |
| `/api/v1/auth/logout` | POST | Выход | Нет |
| `/api/v1/torrents` | GET | Список торрентов | JWT |
| `/api/v1/torrents` | POST | Добавить торрент | JWT |
| `/api/v1/torrents/{id}` | DELETE | Удалить торрент | JWT |
| `/api/v1/torrents/{id}/files` | GET | Список файлов | JWT |
| `/api/v1/torrents/{id}/select` | POST | Выбрать файл | JWT |
| `/api/v1/torrents/{id}/stream` | GET | Стриминг файла | JWT |
| `/api/v1/torrents/{id}/buffer/position` | POST | Установить позицию буфера | JWT |
| `/api/v1/torrents/{id}/buffer/info` | GET | Информация о буфере | JWT |
| `/api/v1/rooms` | POST | Создать комнату | JWT |
| `/api/v1/rooms/join` | POST | Присоединиться | JWT |
| `/api/v1/rooms/leave` | POST | Покинуть комнату | JWT |
| `/api/v1/rooms/signal` | POST | WebRTC сигнал | JWT |
| `/api/v1/rooms/{roomID}/events` | GET | SSE события | JWT |
| `/api/v1/sync/play` | POST | Синхр. play | JWT |
| `/api/v1/sync/pause` | POST | Синхр. pause | JWT |
| `/api/v1/sync/seek` | POST | Синхр. seek | JWT |
| `/api/v1/sync/status` | GET | Статус синхр. | JWT |
| `/api/v1/health/detailed` | GET | Детальный health check | JWT |

**Middleware pipeline** (порядок важен):

```
Request → SecurityHeaders → Recovery → CORS → Logger → CSRF → RateLimit → Auth → Handler
```

### P2P/WebRTC слой

**Компоненты** ([`internal/p2p/service.go`](../backend/internal/p2p/service.go)):

```
┌─────────────────────────────────────────────────────────────┐
│                       P2P Service                           │
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │    Room     │  │    Peer     │  │    WebRTC API       │  │
│  │             │  │             │  │                     │  │
│  │ - ID        │  │ - ID        │  │ - PeerConnection    │  │
│  │ - Name      │  │ - UserID    │  │ - DataChannel       │  │
│  │ - HostID    │  │ - Username  │  │ - ICE candidates    │  │
│  │ - Password  │  │ - Conn      │  │ - STUN config       │  │
│  │ - Peers     │  │ - DataCh    │  │                     │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              Event System (SSE)                      │    │
│  │                                                     │    │
│  │  eventChan (buffered 100) → SSE stream → Frontend   │    │
│  │                                                     │    │
│  │  Events: room_created, peer_joined, peer_left,      │    │
│  │          signal, ice_candidate, connected,           │    │
│  │          disconnected, failed, ping                  │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

**Аутентификация пиров:**
- JWT токен передаётся при присоединении к комнате
- Токен валидируется через `authService.ValidateToken()`
- Пир помечается как `Authenticated` после успешной валидации

**STUN серверы для NAT traversal:**
- `stun:stun.l.google.com:19302`
- `stun:stun1.l.google.com:19302`

### Слой синхронизации

**Компоненты** ([`internal/sync/service.go`](../backend/internal/sync/service.go)):

```
┌─────────────────────────────────────────────────────────────┐
│                      Sync Service                           │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │                   SyncStatus                         │    │
│  │  - IsPlaying: bool                                  │    │
│  │  - Position: float64 (секунды)                      │    │
│  │  - Duration: float64 (секунды)                      │    │
│  │  - Timestamp: int64 (Unix ms)                       │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  Методы:                                                    │
│  - Play() → SyncStatus                                      │
│  - Pause() → SyncStatus                                     │
│  - Seek(position) → SyncStatus                              │
│  - GetStatus() → SyncStatus                                 │
│  - SyncWithLatency(peerStatus, latencyMs) → SyncStatus      │
│  - UpdatePosition(position) → error                         │
└─────────────────────────────────────────────────────────────┘
```

**Алгоритм компенсации задержки:**

```
1. Получить статус удалённого пира (position, timestamp, isPlaying)
2. Рассчитать ожидаемую позицию:
   - Если воспроизведение идёт: expected = position + elapsed - latency
   - Если пауза: expected = position
3. Плавная подстройка:
   - Если |diff| > maxPositionJump (2 сек): position += diff * 0.3
   - Иначе: position = expected
4. Синхронизировать состояние play/pause
```

### Слой буферизации

**Компоненты** ([`internal/buffer/service.go`](../backend/internal/buffer/service.go)):

```
┌─────────────────────────────────────────────────────────────┐
│                      Buffer Service                         │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │                    LRU Cache                         │    │
│  │  - DefaultMaxBufferSize: 512 MB                     │    │
│  │  - DefaultBufferPercent: 10%                        │    │
│  │  - DefaultBufferDuration: 60 sec                    │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  ┌─────────────────────────────────────────────────────┐    │
│  │               Piece Priorities                       │    │
│  │  - PiecePriorityNow: 4 (немедленная загрузка)       │    │
│  │  - PiecePriorityHigh: 3 (высокий приоритет)         │    │
│  │  - PiecePriorityNormal: 2 (обычный)                 │    │
│  │  - PiecePriorityReadahead: 1 (предзагрузка)         │    │
│  │  - PiecePriorityNone: 0 (не загружать)              │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

---

## Frontend архитектура

### Структура модулей

```
frontend/src/
├── main.cpp              # Точка входа
├── mainwindow.h/.cpp     # Главное окно
├── mpvwidget.h/.cpp      # Видеоплеер (libmpv)
├── networkmanager.h/.cpp # HTTP клиент
├── torrentmodel.h/.cpp   # Модель данных торрентов
├── torrentmanager.h/.cpp # Менеджер торрентов
├── roommanager.h/.cpp    # Менеджер комнат
├── roomdialog.h/.cpp     # Диалог создания/присоединения
├── systemtray.h/.cpp     # Системный трей
├── inetworkmanager.h     # Интерфейс сетевого менеджера
├── utils.h/.cpp          # Утилиты
├── test_torrentmodel.cpp # Тесты TorrentModel
└── test_networkmanager.cpp # Тесты NetworkManager
```

### Архитектура главного окна

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              MainWindow                                      │
│                                                                             │
│  ┌────────────────────────────┐  ┌────────────────────────────────────────┐ │
│  │      Левая панель          │  │          Правая панель                 │ │
│  │                            │  │                                        │ │
│  │  ┌──────────────────────┐  │  │  ┌──────────────────────────────────┐ │ │
│  │  │   TorrentModel       │  │  │  │         MpvWidget                │ │ │
│  │  │   (QListView)        │  │  │  │                                  │ │ │
│  │  └──────────────────────┘  │  │  │  - mpv_handle                    │ │ │
│  │  ┌──────────────────────┐  │  │  │  - mpv_render_context            │ │ │
│  │   │   File List          │  │  │  │  - OpenGL rendering              │ │ │
│  │  │   (QListView)        │  │  │  └──────────────────────────────────┘ │ │
│  │  └──────────────────────┘  │  │  ┌──────────────────────────────────┐ │ │
│  │  ┌──────────────────────┐  │  │  │    Панель управления             │ │ │
│  │  │  [Magnet Input]      │  │  │  │  [Play/Pause] [Seek] [Time]      │ │ │
│  │  │  [Add Button]        │  │  │  └──────────────────────────────────┘ │ │
│  │  └──────────────────────┘  │  │  ┌──────────────────────────────────┐ │ │
│  └────────────────────────────┘  │  │    Панель комнат                 │ │ │
│                                  │  │  [Create] [Join] [Leave]         │ │ │
│                                  │  └──────────────────────────────────┘ │ │
│                                  └────────────────────────────────────────┘ │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                         Статусная строка                                ││
│  └─────────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────────┘
```

### Взаимодействие с backend

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         NetworkManager                                      │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        HTTP REST API                                 │    │
│  │                                                                     │    │
│  │  Torrent API:    POST/GET/DELETE /api/v1/torrents/*                 │    │
│  │  Room API:       POST /api/v1/rooms/*                               │    │
│  │  Sync API:       POST/GET /api/v1/sync/*                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     SSE (Server-Sent Events)                        │    │
│  │                                                                     │    │
│  │  GET /api/v1/rooms/{roomID}/events                                  │    │
│  │                                                                     │    │
│  │  Events: connected, peer_joined, peer_left, signal, ping, timeout   │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        Retry Logic                                   │    │
│  │                                                                     │    │
│  │  - Exponential backoff: delay = baseDelay * 2^attempt               │    │
│  │  - Max retries: 3 (configurable)                                    │    │
│  │  - Base delay: 1000ms (configurable)                                │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Паттерн запроса:**

```
1. Frontend вызывает метод NetworkManager (например, addTorrent)
2. NetworkManager формирует HTTP запрос и отправляет
3. При получении ответа парсит JSON
4. Испускает сигнал (например, torrentAdded)
5. MainWindow подключён к сигналу и обновляет UI
```

### Видеоплеер (libmpv)

**Компоненты** ([`frontend/src/mpvwidget.h`](../frontend/src/mpvwidget.h)):

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              MpvWidget                                       │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        libmpv Core                                   │    │
│  │                                                                     │    │
│  │  mpv_handle ─── mpv_create()                                        │    │
│  │  mpv_render_context ─── mpv_render_context_create()                 │    │
│  │                                                                     │    │
│  │  Команды: play, pause, seek, getProperty                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    Event Processing                                  │    │
│  │                                                                     │    │
│  │  mpv_event → processMpvEvent() → eventBuffer → emit signals        │    │
│  │                                                                     │    │
│  │  События: positionChanged, durationChanged, playbackFinished, error │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    Thread Safety                                     │    │
│  │                                                                     │    │
│  │  QMutex для защиты mpv_handle                                       │    │
│  │  QTimer для обработки событий в основном потоке                     │    │
│  │  Seek debounce для предотвращения утечек                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Потокобезопасность:**
- Все вызовы mpv защищены `QMutex`
- События mpv буферизуются и эмитятся в основном потоке
- Seek debounce предотвращает утечку при быстрой перемотке

---

## Схема коммуникации

### Последовательность операций

#### Добавление торрента и воспроизведение

```
Frontend                 Backend                 Torrent Client
   │                        │                        │
   │  POST /torrents        │                        │
   │  {magnetUri}           │                        │
   │───────────────────────►│                        │
   │                        │  AddMagnet()           │
   │                        │───────────────────────►│
   │                        │                        │
   │                        │  GotInfo()             │
   │                        │◄───────────────────────│
   │                        │                        │
   │  201 {torrentInfo}     │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  GET /torrents/{id}/files                       │
   │───────────────────────►│                        │
   │  200 {files[]}         │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  POST /torrents/{id}/select                     │
   │  {fileIndex}           │                        │
   │───────────────────────►│                        │
   │                        │  SetPriority()         │
   │                        │───────────────────────►│
   │  200 OK                │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  GET /torrents/{id}/stream                      │
   │───────────────────────►│                        │
   │                        │  ServeFile()           │
   │                        │───────────────────────►│
   │  200 (video stream)    │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  MpvWidget.play(url)   │                        │
   │  (локальный вызов)     │                        │
```

#### Создание комнаты и синхронизация

```
User A (Host)            Backend                  User B (Peer)
   │                        │                        │
   │  POST /rooms           │                        │
   │  {name, password}      │                        │
   │───────────────────────►│                        │
   │  201 {roomInfo}        │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  GET /rooms/{id}/events│                        │
   │  (SSE connect)         │                        │
   │◄═══════════════════════│                        │
   │                        │                        │
   │                        │    POST /rooms/join    │
   │                        │    {roomId, password}  │
   │                        │◄───────────────────────│
   │                        │                        │
   │  SSE: peer_joined      │    200 OK              │
   │◄═══════════════════════│───────────────────────►│
   │                        │                        │
   │                        │    GET /rooms/{id}/events
   │                        │    (SSE connect)       │
   │                        │═══════════════════════►│
   │                        │                        │
   │  POST /sync/play       │                        │
   │───────────────────────►│                        │
   │  200 {syncStatus}      │                        │
   │◄───────────────────────│                        │
   │                        │                        │
   │  WebRTC DataChannel ◄──────────────────────────►│
   │  (P2P sync data)       │                        │
```

### P2P соединение (WebRTC)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         WebRTC Connection Flow                               │
│                                                                             │
│  Host                              Peer                                      │
│    │                                  │                                     │
│    │  1. Create PeerConnection        │                                     │
│    │  2. Create DataChannel           │                                     │
│    │  3. Create Offer (SDP)           │                                     │
│    │                                  │                                     │
│    │  ──── SDP Offer (via SSE) ──────►│                                     │
│    │                                  │  4. Create PeerConnection           │
│    │                                  │  5. Set Remote Description          │
│    │                                  │  6. Create Answer (SDP)             │
│    │                                  │                                     │
│    │  ◄── SDP Answer (via SSE) ──────│                                     │
│    │  7. Set Remote Description       │                                     │
│    │                                  │                                     │
│    │  ──── ICE Candidates ───────────►│                                     │
│    │  ◄─── ICE Candidates ────────────│                                     │
│    │                                  │                                     │
│    │  ════ DataChannel Open ═════════│                                     │
│    │                                  │                                     │
│    │  ════ Sync Data (P2P) ═════════►│                                     │
│    │  ◄═══ Sync Data (P2P) ══════════│                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Модель данных

### Основные структуры

**TorrentInfo** — информация о торренте:
```go
type TorrentInfo struct {
    ID       string  `json:"id"`       // Info hash (hex)
    Name     string  `json:"name"`     // Название торрента
    Progress float64 `json:"progress"` // Прогресс загрузки (0-1)
    Status   string  `json:"status"`   // loading/downloading/seeding
    Size     int64   `json:"size"`     // Размер в байтах
}
```

**FileInfo** — информация о файле в торренте:
```go
type FileInfo struct {
    Index int    `json:"index"` // Индекс файла
    Name  string `json:"name"`  // Имя файла
    Size  int64  `json:"size"`  // Размер в байтах
}
```

**RoomInfo** — информация о P2P комнате:
```go
type RoomInfo struct {
    ID        string `json:"id"`        // ID комнаты
    Name      string `json:"name"`      // Название
    HostID    string `json:"hostId"`    // ID хоста
    PeerCount int    `json:"peerCount"` // Количество пиров
}
```

**SyncStatus** — статус синхронизации:
```go
type SyncStatus struct {
    IsPlaying bool    `json:"isPlaying"` // Воспроизведение активно
    Position  float64 `json:"position"`  // Позиция в секундах
    Duration  float64 `json:"duration"`  // Длительность в секундах
    Timestamp int64   `json:"timestamp"` // Unix timestamp (ms)
}
```

**P2PEvent** — событие P2P:
```go
type P2PEvent struct {
    Type string      `json:"type"` // Тип события
    Data interface{} `json:"data"` // Данные события
}
```

**User** — пользователь:
```go
type User struct {
    ID           string `json:"id"`
    Username     string `json:"username"`
    PasswordHash string `json:"-"`       // bcrypt хеш
    CreatedAt    int64  `json:"createdAt"`
}
```

### Внутренние структуры Backend

**Room** (внутренняя):
```go
type Room struct {
    ID          string
    Name        string
    HostID      string
    HostUserID  string
    Password    string           // bcrypt хеш
    Peers       map[string]*Peer
    CreatedAt   time.Time
    RequireAuth bool
}
```

**Peer** (внутренняя):
```go
type Peer struct {
    ID            string
    UserID        string
    Username      string
    Connection    *webrtc.PeerConnection
    DataChannel   *webrtc.DataChannel
    LastHeartbeat time.Time
    Authenticated bool
}
```

### Связи между сущностями

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            Entity Relationships                              │
│                                                                             │
│  ┌─────────────┐       1:N      ┌─────────────┐                           │
│  │   Torrent   │───────────────►│    File     │                           │
│  │             │                │             │                           │
│  │ - ID (PK)   │                │ - Index     │                           │
│  │ - Name      │                │ - Name      │                           │
│  │ - Status    │                │ - Size      │                           │
│  └─────────────┘                └─────────────┘                           │
│                                                                             │
│  ┌─────────────┐       1:N      ┌─────────────┐                           │
│  │    Room     │───────────────►│    Peer     │                           │
│  │             │                │             │                           │
│  │ - ID (PK)   │                │ - ID (PK)   │                           │
│  │ - HostID    │                │ - UserID    │                           │
│  │ - Password  │                │ - Conn      │                           │
│  └─────────────┘                └─────────────┘                           │
│                                                                             │
│  ┌─────────────┐       1:1      ┌─────────────┐                           │
│  │    User     │───────────────►│  SyncStatus │                           │
│  │             │                │             │                           │
│  │ - ID (PK)   │                │ - Position  │                           │
│  │ - Username  │                │ - Duration  │                           │
│  │ - Token     │                │ - IsPlaying │                           │
│  └─────────────┘                └─────────────┘                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Безопасность

### Аутентификация
- JWT токены для аутентификации пользователей (HS256, 24h TTL)
- bcrypt хеширование паролей комнат (cost=12)
- JTI (JWT ID) для отзыва токенов
- Токены имеют срок действия

### Защита API
- CSRF токены для защиты от межсайтовой подделки (TTL 1h)
- Rate limiting (10 req/min для auth, 60 req/min для API)
- CORS политики
- Security headers (X-Content-Type-Options, X-Frame-Options, HSTS)
- Валидация всех входных данных
- TLS 1.2+ поддержка

### Потокобезопасность
- `sync.RWMutex` в каждом сервисе
- Буферизованные каналы для событий
- Graceful shutdown с таймаутами
- Context для отмены операций
