# Changelog

Все значимые изменения в проекте документируются в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и этот проект следует [Semantic Versioning](https://semver.org/lang/ru/).

## [Не выпущено]

### Добавлено

- Добавлен `.editorconfig` с настройками для Go, C++, CMake, Makefile, JSON, YAML
- Добавлен code coverage в CI pipeline (Go + C++ с Codecov интеграцией)
- Добавлена проверка coverage для PR (минимум 60%)
- Добавлены константы для магических чисел в torrent, p2p и sync сервисах
- Добавлен заголовок лицензии MIT во все исходные файлы (.go, .cpp, .h)
- Создано руководство пользователя (`docs/USER_GUIDE.md`)
- Создано руководство по установке (`docs/INSTALL.md`)
- Создана документация по архитектуре (`docs/ARCHITECTURE.md`)
- Создано руководство для контрибьюторов (`CONTRIBUTING.md`)
- Добавлена валидация roomID в API endpoints
- Добавлены тесты для валидации (`internal/validation/validation_test.go`)
- Добавлены тесты для auth handlers (`internal/auth/handlers_test.go`)
- Добавлены тесты для API handlers (`internal/api/handlers_test.go`)
- Добавлены тесты для torrent service (`internal/torrent/service_test.go`)
- Добавлены тесты для p2p service (`internal/p2p/service_test.go`)
- Добавлены тесты для sync service (`internal/sync/service_test.go`)
- Добавлен пакет `buffer` — LRU кэш с приоритетами загрузки pieces
- Добавлен пакет `storage` — in-memory хранилище
- Добавлен пакет `errors` — AppError, ErrorType
- Добавлен пакет `constants` — все магические числа вынесены в константы
- Добавлена CSRF защита (token store с TTL 1h)
- Добавлен rate limiting (10 req/min для auth, 60 req/min для API)
- Добавлена JWT аутентификация (HS256, 24h TTL, JTI для revocation)
- Добавлен bcrypt (cost=12) для хеширования паролей
- Добавлен in-memory UserStore и TokenRevocationStore
- Добавлены Prometheus метрики
- Добавлен Swagger UI на `/swagger/`
- Добавлена поддержка TLS 1.2+
- Добавлен pprof на порту 6060 (опционально)
- Добавлен graceful shutdown (30s timeout)
- Добавлен retry logic в NetworkManager (exponential backoff, max 3)
- Добавлен seek debounce в MpvWidget
- Добавлены SSE для real-time событий комнаты
- Добавлена интеграция с Docker Compose (Prometheus + Grafana profiles)

### Изменено

- Улучшена документация README.md с полным списком endpoints и ссылками
- Обновлён CI pipeline с поддержкой coverage отчётов
- Обновлён CHANGELOG с последними изменениями
- Обновлена версия Go до 1.24 в go.mod
- Обновлены зависимости до актуальных версий

### Исправлено

- Вынесены магические числа в именованные константы:
  - `torrent/service.go`: `gracefulShutdownTimeout`, `dataDirPermissions`
  - `p2p/service.go`: `eventChannelSize`, `sseTimeout`, `ssePingInterval`, `peerIDLength`
  - `sync/service.go`: `maxPositionJump`, `smoothAdjustmentRatio`, `msPerSecond`
- Исправлен баг с RoomEvents роутом: изменён путь с `/api/v1/rooms/events` на `/api/v1/rooms/{roomID}/events`
- Исправлен Signal handler: добавлена правильная обработка WebRTC сигналов
- Исправлен LogoutHandler: добавлена корректная обработка выхода из системы

### Безопасность

- JWT аутентификация с отзывом токенов
- bcrypt хеширование паролей (cost=12)
- CSRF токены с TTL 1h
- Rate limiting (10 req/min для auth, 60 req/min для API)
- Security headers (X-Content-Type-Options, X-Frame-Options, HSTS)
- CORS политики
- Валидация всех входных данных
- TLS 1.2+ поддержка

### Известные проблемы

1. `frontend/src/main.cpp:336` — при запуске с `--server-url` URL парсится, но не передаётся в NetworkManager
2. `frontend/src/networkmanager.cpp:301` — SSL ошибки игнорируются в debug-режиме, production-поведение не реализовано
3. In-memory UserStore/TokenRevocationStore — без персистентности
4. Нет интеграции с базой данных
5. Buffer Service не имеет unit-тестов
6. Frontend MainWindow/MpvWidget не имеют unit-тестов

## [1.0.0] - 2025-06-01

### Добавлено

- Базовая функциональность торрент-клиента на базе anacrolix/torrent
- HTTP REST API сервер на Go с chi router
- P2P соединения через WebRTC (pion/webrtc v4)
- Синхронизация воспроизведения с компенсацией задержки
- JWT аутентификация для пользователей и пиров
- Парольная защита комнат с bcrypt хешированием
- SSE (Server-Sent Events) для событий комнаты в реальном времени
- Qt/C++ frontend с libmpv видеоплеером
- Системный трей интеграция
- Graceful shutdown для всех сервисов
- Структурированное логирование
- CSRF защита
- Rate limiting для API
- CORS поддержка
- Health check endpoints
- Prometheus метрики

### Безопасность

- JWT аутентификация
- bcrypt хеширование паролей
- CSRF токены
- Rate limiting
- Security headers
- CORS политики
- Валидация входных данных

### Архитектура

- Микросервисная архитектура с независимыми сервисами
- Потокобезопасность через sync.RWMutex
- Graceful shutdown с таймаутами
- Контекст-ориентированное управление
