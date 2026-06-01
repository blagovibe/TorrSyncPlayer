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
- Создан этот CHANGELOG

### Изменено

- Улучшена документация README.md с ссылками на документацию
- Обновлён CI pipeline с поддержкой coverage отчётов

### Исправлено

- Вынесены магические числа в именованные константы:
  - `torrent/service.go`: `gracefulShutdownTimeout`, `dataDirPermissions`
  - `p2p/service.go`: `eventChannelSize`, `sseTimeout`, `ssePingInterval`, `peerIDLength`
  - `sync/service.go`: `maxPositionJump`, `smoothAdjustmentRatio`, `msPerSecond`

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
