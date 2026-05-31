# TorrSyncPlayer - Руководство разработчика

## Требования

- Go 1.21+
- Node.js 20+
- Wails CLI
- Git

## Установка Wails CLI

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

## Структура проекта

```
torrsyncplayer/
├── main.go              # Точка входа
├── app.go               # Wails приложение
├── services.go          # TorrentService
├── p2p_service.go       # P2PService
├── sync_service.go      # SyncService
├── interfaces.go        # Интерфейсы сервисов
├── models.go            # Общие типы данных
├── services_test.go     # Тесты TorrentService
├── p2p_service_test.go  # Тесты P2PService
├── sync_service_test.go # Тесты SyncService
├── go.mod
├── go.sum
├── wails.json
├── Makefile
├── frontend/            # React приложение
│   ├── src/
│   │   ├── components/
│   │   ├── hooks/
│   │   ├── services/
│   │   ├── types/
│   │   ├── utils/
│   │   └── __tests__/
│   └── package.json
└── .github/workflows/   # CI/CD
```

## Разработка

### Запуск в режиме разработки

```bash
wails dev
```

### Генерация bindings

При изменении Go методов необходимо перегенерировать bindings:

```bash
wails generate module
```

### Сборка

```bash
# Для текущей платформы
wails build

# Для Windows
wails build -platform windows/amd64

# Для Linux
wails build -platform linux/amd64

# Для macOS
wails build -platform darwin/amd64
```

## Архитектура

### Сервисы
- **TorrentService** - управление торрентами и HTTP-стримингом
- **P2PService** - WebRTC P2P соединения и синхронизация
- **SyncService** - синхронизация воспроизведения между пирами

### Интерфейсы
Все сервисы реализуют интерфейсы из `interfaces.go` для обеспечения
тестируемости и слабой связанности.

### Backend (Go)

- **TorrentService** - работа с торрентами (anacrolix/torrent)
- **P2PService** - P2P соединения (pion/webrtc)
- **SyncService** - синхронизация воспроизведения

### Frontend (React)

- **Hooks** - useTorrent, useP2P, useSync, useWails
- **Components** - HomePage, RoomPage, VideoPlayer, StatusBar
- **Services** - wails-api для вызова Go функций

### События

События отправляются из Go во фронтенд через Wails Events:

- `torrent:added` - торрент добавлен
- `torrent:progress` - прогресс загрузки
- `torrent:completed` - загрузка завершена
- `p2p:room_created` - комната создана
- `p2p:room_joined` - подключение к комнате
- `p2p:peer_connected` - пир подключен
- `p2p:peer_disconnected` - пир отключен
- `sync:state_changed` - изменение состояния воспроизведения
- `sync:play/pause/seek` - команды управления

## Тестирование

### Go тесты
```bash
make test              # Запуск тестов
make test-verbose      # Подробный вывод
make test-coverage     # С coverage отчетом
```

### Frontend тесты
```bash
cd frontend
npm run test           # Запуск в watch режиме
npm run test:run       # Один запуск
npm run test:coverage  # С coverage отчетом
```

## Сборка для production

```bash
# Оптимизированная сборка
wails build -ldflags "-s -w"

# Сборка с NSIS установщиком (Windows)
wails build -nsis
```

## Устранение неполадок

### Wails не найден

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

### Ошибки сборки фронтенда

```bash
cd frontend
rm -rf node_modules
npm install
```

### Ошибки Go модуля

```bash
go mod tidy
go mod download
```
