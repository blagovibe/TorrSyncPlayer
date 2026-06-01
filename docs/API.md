# TorrSyncPlayer API Documentation

## Обзор

TorrSyncPlayer предоставляет HTTP REST API для управления торрентами, P2P комнатами и синхронизацией воспроизведения.

- **Базовый URL:** `http://localhost:8889`
- **Версия API:** v1
- **Формат:** JSON
- **Аутентификация:** JWT токен (для защищённых endpoints)

## Аутентификация

### POST /api/v1/auth/register

Регистрация нового пользователя.

**Запрос:**
```json
{
  "username": "user123",
  "password": "securepassword"
}
```

**Ответ (201):**
```json
{
  "id": "uuid",
  "username": "user123",
  "token": "jwt_token_here"
}
```

### POST /api/v1/auth/login

Вход в систему.

**Запрос:**
```json
{
  "username": "user123",
  "password": "securepassword"
}
```

**Ответ (200):**
```json
{
  "token": "jwt_token_here",
  "expires_at": "2024-01-01T00:00:00Z"
}
```

## Health Check

### GET /health

Базовая проверка здоровья сервера.

**Ответ (200):**
```json
{
  "status": "ok",
  "uptime": 3600.5,
  "version": "1.0.0",
  "services": {
    "torrent": "ok",
    "p2p": "ok",
    "sync": "ok"
  }
}
```

### GET /health/detailed

Расширенная проверка здоровья с проверкой состояния сервисов.

**Ответ (200):**
```json
{
  "status": "ok",
  "services": {
    "torrent": "ok",
    "p2p": "ok",
    "sync": "ok"
  },
  "version": "1.0.0"
}
```

**Ответ (503) при проблемах:**
```json
{
  "status": "degraded",
  "services": {
    "torrent": "ok",
    "p2p": "unavailable",
    "sync": "ok"
  },
  "version": "1.0.0"
}
```

## Torrent API

### GET /api/v1/torrents

Получить список всех торрентов.

**Параметры запроса:**
- `limit` (int, optional) - Количество элементов (по умолчанию 20, максимум 100)
- `offset` (int, optional) - Смещение (по умолчанию 0)

**Ответ (200):**
```json
{
  "torrents": [
    {
      "id": "info_hash",
      "name": "Movie Name",
      "size": 1073741824,
      "progress": 0.75,
      "status": "downloading"
    }
  ],
  "total": 10,
  "limit": 20,
  "offset": 0
}
```

### POST /api/v1/torrents

Добавить торрент по magnet-ссылке.

**Запрос:**
```json
{
  "magnetUri": "magnet:?xt=urn:btih:..."
}
```

**Ответ (201):**
```json
{
  "id": "info_hash",
  "name": "Movie Name",
  "size": 1073741824,
  "progress": 0.0,
  "status": "loading"
}
```

**Ошибки:**
- `400` - Неверный формат magnet URI
- `408` - Таймаут получения метаданных

### DELETE /api/v1/torrents/{id}

Удалить торрент.

**Ответ (200):**
```json
{
  "message": "Торрент удалён"
}
```

**Ошибки:**
- `404` - Торрент не найден

### GET /api/v1/torrents/{id}/files

Получить список файлов торрента.

**Параметры запроса:**
- `limit` (int, optional) - Количество элементов
- `offset` (int, optional) - Смещение

**Ответ (200):**
```json
{
  "files": [
    {
      "index": 0,
      "name": "movie.mp4",
      "size": 1073741824,
      "path": "/path/to/movie.mp4"
    }
  ],
  "total": 5,
  "limit": 20,
  "offset": 0
}
```

**Ошибки:**
- `404` - Торрент не найден

### POST /api/v1/torrents/{id}/select

Выбрать файл для стриминга.

**Запрос:**
```json
{
  "fileIndex": 0
}
```

**Ответ (200):**
```json
{
  "message": "Файл выбран"
}
```

**Ошибки:**
- `400` - Неверный индекс файла
- `404` - Торрент не найден

### GET /api/v1/torrents/{id}/stream

Стриминг выбранного файла.

**Заголовки ответа:**
- `Content-Type`: MIME тип файла
- `Accept-Ranges: bytes` - Поддержка Range запросов

**Поддерживаемые форматы:**
- Видео: mp4, mkv, avi, webm, mov, wmv, flv
- Аудио: mp3, aac, wav, ogg, flac
- Субтитры: srt, ass, ssa

**Ошибки:**
- `400` - Файл не выбран
- `404` - Торрент не найден

## Room API

### POST /api/v1/rooms

Создать новую комнату.

**Запрос:**
```json
{
  "name": "My Room",
  "password": "optional_password"
}
```

**Ответ (201):**
```json
{
  "id": "room_id",
  "name": "My Room",
  "hostID": "peer_id",
  "peerCount": 1
}
```

**Ошибки:**
- `400` - Некорректное название комнаты

### POST /api/v1/rooms/join

Присоединиться к комнате.

**Запрос:**
```json
{
  "roomId": "room_id",
  "password": "room_password"
}
```

**Ответ (200):**
```json
{
  "message": "Присоединились к комнате"
}
```

**Ошибки:**
- `400` - Неверный ID комнаты
- `401` - Неверный пароль
- `404` - Комната не найдена

### POST /api/v1/rooms/leave

Покинуть комнату.

**Ответ (200):**
```json
{
  "message": "Вышли из комнаты"
}
```

**Ошибки:**
- `400` - Не подключены к комнате

### POST /api/v1/rooms/signal

Отправить WebRTC сигнал.

**Запрос:**
```json
{
  "signal": "base64_encoded_signal_data"
}
```

**Ответ (200):**
```json
{
  "message": "Сигнал отправлен"
}
```

**Ошибки:**
- `400` - Не в комнате

### GET /api/v1/rooms/events

Подключиться к SSE потоку событий комнаты.

**Заголовки ответа:**
- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`

**События:**
- `connected` - Подключение установлено
- `peer_joined` - Пир присоединился
- `peer_left` - Пир покинул комнату
- `signal` - WebRTC сигнал
- `ping` - Ping для поддержания соединения
- `timeout` - Таймаут соединения

**Пример события:**
```
event: peer_joined
data: {"type": "peer_joined", "peerId": "peer_id", "roomId": "room_id"}
```

## Sync API

### POST /api/v1/sync/play

Запустить синхронизированное воспроизведение.

**Ответ (200):**
```json
{
  "isPlaying": true,
  "position": 120.5,
  "duration": 3600.0,
  "timestamp": 1704067200000
}
```

### POST /api/v1/sync/pause

Приостановить воспроизведение.

**Ответ (200):**
```json
{
  "isPlaying": false,
  "position": 125.0,
  "duration": 3600.0,
  "timestamp": 1704067205000
}
```

### POST /api/v1/sync/seek

Синхронизировать перемотку.

**Запрос:**
```json
{
  "position": 300.0
}
```

**Ответ (200):**
```json
{
  "isPlaying": true,
  "position": 300.0,
  "duration": 3600.0,
  "timestamp": 1704067500000
}
```

**Ошибки:**
- `400` - Некорректная позиция

### GET /api/v1/sync/status

Получить текущий статус синхронизации.

**Ответ (200):**
```json
{
  "isPlaying": true,
  "position": 120.5,
  "duration": 3600.0,
  "timestamp": 1704067200000
}
```

## Коды ошибок

| Код | Описание |
|-----|----------|
| 400 | Неверный запрос (Bad Request) |
| 401 | Не авторизован (Unauthorized) |
| 403 | Доступ запрещён (Forbidden) |
| 404 | Ресурс не найден (Not Found) |
| 408 | Таймаут запроса (Request Timeout) |
| 409 | Конфликт (Conflict) |
| 429 | Слишком много запросов (Too Many Requests) |
| 500 | Внутренняя ошибка сервера (Internal Server Error) |
| 503 | Сервис недоступен (Service Unavailable) |

## Формат ошибок

Все ошибки возвращаются в формате JSON:

```json
{
  "code": 404,
  "message": "Торрент не найден"
}
```

## Rate Limiting

- **Auth endpoints:** 10 запросов/минуту
- **API endpoints:** 60 запросов/минуту (burst 10)

Заголовки ответа:
- `X-RateLimit-Limit` - Лимит запросов
- `X-RateLimit-Remaining` - Оставшиеся запросы
- `X-RateLimit-Reset` - Время сброса лимита

## CORS

API поддерживает CORS для следующих источников:
- `http://localhost:*` (разработка)
- Настраивается через переменную окружения `CORS_ORIGINS`

## SSE (Server-Sent Events)

Для получения событий в реальном времени используйте SSE:

```javascript
const eventSource = new EventSource('/api/v1/rooms/events');

eventSource.addEventListener('peer_joined', (e) => {
  const data = JSON.parse(e.data);
  console.log('Пир присоединился:', data.peerId);
});

eventSource.addEventListener('signal', (e) => {
  const data = JSON.parse(e.data);
  handleSignal(data);
});
```

## WebSocket (опционально)

Для WebRTC signaling можно использовать WebSocket:

```
ws://localhost:8889/api/v1/ws
```
