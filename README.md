# TorrSyncPlayer

[![CI](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/ci.yml/badge.svg)](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/ci.yml)
[![Release](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/release.yml/badge.svg)](https://github.com/blagovibe/TorrSyncPlayer/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Десктопный торрент-плеер с P2P синхронизацией воспроизведения.

## Возможности

- **Потоковое воспроизведение** — мгновенный старт просмотра без полной загрузки
- **P2P комнаты** — синхронный просмотр с друзьями через WebRTC
- **Безопасность** — JWT аутентификация (HS256), CSRF защита, rate limiting, bcrypt пароли
- **Метрики** — Prometheus метрики для мониторинга
- **Буферизация** — LRU кэш с приоритетами загрузки pieces
- **Docker** — готовая multi-stage Docker конфигурация
- **CI/CD** — GitHub Actions с golangci-lint, clang-tidy, tests, coverage ≥60%
- **Swagger** — интерактивная документация API на `/swagger/`

## Стек

- **Backend:** Go 1.24+, anacrolix/torrent v1.58.1, pion/webrtc v4, go-chi/chi/v5, golang-jwt/jwt/v5
- **Frontend:** C++17, Qt 6.5+, libmpv, CMake 3.16+
- **Build:** Make (backend), CMake (frontend)
- **CI/CD:** GitHub Actions
- **Docker:** multi-stage build (golang:1.25-alpine → alpine:3.19)

## Документация

- [API документация](docs/API.md) — полное описание REST API (22 маршрута)
- [Архитектура](docs/ARCHITECTURE.md) — описание архитектуры backend и frontend
- [Руководство пользователя](docs/USER_GUIDE.md) — инструкция по использованию
- [Руководство по установке](docs/INSTALL.md) — установка и настройка
- [Changelog](CHANGELOG.md) — история изменений
- [Swagger UI](http://localhost:8889/swagger/) — интерактивная API документация (при запущенном сервере)

## Быстрый старт

### Backend

```bash
cd backend
make build
make run
```

Сервер запустится на порту 8889.

### Frontend

```bash
cd frontend
./build.sh  # Linux/macOS
build.bat   # Windows
```

### Docker

```bash
docker-compose up -d
```

## Запуск

```bash
# Терминал 1
cd backend && make run

# Терминал 2
cd frontend/build && ./TorrSyncPlayer
```

## Структура проекта

```
TorrSyncPlayer/
├── backend/           # Go backend (HTTP API + P2P + Torrent)
│   ├── cmd/server/    # Точка входа (main.go, 408 строк)
│   ├── internal/
│   │   ├── api/       # HTTP API (router, handlers, middleware, tests)
│   │   ├── auth/      # JWT аутентификация (HS256, bcrypt, token revocation)
│   │   ├── buffer/    # LRU кэш, приоритеты pieces
│   │   ├── constants/ # Все магические числа вынесены в константы
│   │   ├── errors/    # AppError, ErrorType
│   │   ├── metrics/   # Prometheus метрики
│   │   ├── models/    # Модели данных
│   │   ├── p2p/       # WebRTC P2P сервис, комнаты
│   │   ├── storage/   # In-memory хранилище
│   │   ├── sync/      # Синхронизация воспроизведения с компенсацией задержки
│   │   ├── torrent/   # Управление торрентами + HTTP streaming
│   │   ├── validation/# Валидация входных данных
│   │   └── version/   # Информация о версии
│   ├── pkg/logger/    # slog-based логгер
│   ├── docs/          # Swagger спецификация (swagger.yaml, swagger.json, docs.go)
│   ├── Makefile
│   └── go.mod
│
├── frontend/          # Qt/C++ frontend
│   ├── src/           # Исходный код
│   │   ├── main.cpp
│   │   ├── mainwindow.h/.cpp
│   │   ├── mpvwidget.h/.cpp
│   │   ├── networkmanager.h/.cpp
│   │   ├── torrentmodel.h/.cpp
│   │   ├── torrentmanager.h/.cpp
│   │   ├── roommanager.h/.cpp
│   │   ├── roomdialog.h/.cpp
│   │   ├── systemtray.h/.cpp
│   │   ├── utils.h/.cpp
│   │   ├── inetworkmanager.h
│   │   ├── test_torrentmodel.cpp
│   │   └── test_networkmanager.cpp
│   ├── resources/     # Ресурсы (иконки и т.д.)
│   ├── CMakeLists.txt
│   └── build.sh / build.bat
│
├── docs/              # Документация
│   ├── API.md         # API документация
│   ├── ARCHITECTURE.md # Архитектура
│   ├── INSTALL.md     # Руководство по установке
│   └── USER_GUIDE.md  # Руководство пользователя
│
├── .github/           # GitHub Actions workflows
│   └── workflows/
│       ├── ci.yml     # CI pipeline (lint, test, build, coverage)
│       └── release.yml # Release pipeline
│
├── Dockerfile         # Multi-stage Docker образ
├── docker-compose.yml # Docker Compose конфигурация (backend + Prometheus + Grafana)
├── Makefile           # Корневой Makefile
├── CHANGELOG.md       # История изменений
├── CONTRIBUTING.md    # Руководство для контрибьюторов
├── AGENTS.md          # Руководство для AI-агентов
└── LICENSE            # MIT лицензия
```

## API

### Основные endpoints

| Метод | Путь | Описание | Аутентификация |
|-------|------|----------|----------------|
| GET | `/health` | Health check | Нет |
| GET | `/api/v1/version` | Версия сервера | Нет |
| GET | `/metrics` | Prometheus метрики | Нет |
| GET | `/api/v1/csrf-token` | Получить CSRF токен | Нет |
| POST | `/api/v1/auth/register` | Регистрация | Нет |
| POST | `/api/v1/auth/login` | Вход | Нет |
| POST | `/api/v1/auth/logout` | Выход | Нет |
| GET | `/api/v1/torrents` | Список торрентов | JWT |
| POST | `/api/v1/torrents` | Добавить торрент | JWT |
| DELETE | `/api/v1/torrents/{id}` | Удалить торрент | JWT |
| GET | `/api/v1/torrents/{id}/files` | Список файлов | JWT |
| POST | `/api/v1/torrents/{id}/select` | Выбрать файл | JWT |
| GET | `/api/v1/torrents/{id}/stream` | Стриминг файла | JWT |
| POST | `/api/v1/torrents/{id}/buffer/position` | Установить позицию буфера | JWT |
| GET | `/api/v1/torrents/{id}/buffer/info` | Информация о буфере | JWT |
| POST | `/api/v1/rooms` | Создать комнату | JWT |
| POST | `/api/v1/rooms/join` | Присоединиться | JWT |
| POST | `/api/v1/rooms/leave` | Покинуть комнату | JWT |
| POST | `/api/v1/rooms/signal` | WebRTC сигнал | JWT |
| GET | `/api/v1/rooms/{roomID}/events` | SSE события | JWT |
| POST | `/api/v1/sync/play` | Синхр. play | JWT |
| POST | `/api/v1/sync/pause` | Синхр. pause | JWT |
| POST | `/api/v1/sync/seek` | Синхр. seek | JWT |
| GET | `/api/v1/sync/status` | Статус синхр. | JWT |
| GET | `/api/v1/health/detailed` | Детальный health check | JWT |

Полная документация API доступна в [docs/API.md](docs/API.md) и в Swagger UI на `/swagger/`.

## Тестирование

```bash
# Backend тесты
cd backend
make test

# Backend тесты с coverage
go test -cover ./...

# Frontend тесты
cd frontend/build
ctest --output-on-failure
```

## Известные ограничения

1. **In-memory хранилище** — UserStore и TokenRevocationStore не имеют персистентности (данные теряются при перезапуске)
2. **Нет интеграции с БД** — для production требуется подключение базы данных
3. **Buffer Service** — не имеет unit-тестов
4. **Frontend тесты** — MainWindow и MpvWidget не имеют unit-тестов
5. **SSL в NetworkManager** — в debug-режиме SSL ошибки игнорируются, production-поведение не реализовано
6. **--server-url флаг** — при запуске frontend с `--server-url` URL парсится, но не передаётся в NetworkManager

## Лицензия

[MIT](LICENSE)
