# TorrSyncPlayer

[![CI](https://github.com/USERNAME/TorrSyncPlayer/actions/workflows/ci.yml/badge.svg)](https://github.com/USERNAME/TorrSyncPlayer/actions/workflows/ci.yml)
[![Release](https://github.com/USERNAME/TorrSyncPlayer/actions/workflows/release.yml/badge.svg)](https://github.com/USERNAME/TorrSyncPlayer/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Десктопный торрент-плеер с P2P синхронизацией воспроизведения.

## Возможности

- **Потоковое воспроизведение** — мгновенный старт просмотра без полной загрузки
- **P2P комнаты** — синхронный просмотр с друзьями через WebRTC
- **Безопасность** — JWT аутентификация, парольная защита комнат
- **Метрики** — Prometheus метрики для мониторинга
- **Docker** — готовая Docker конфигурация для развёртывания

## Стек

- **Backend:** Go 1.25+, anacrolix/torrent, pion/webrtc v4
- **Frontend:** C++17, Qt 6.5+, libmpv

## Документация

- [Руководство пользователя](docs/USER_GUIDE.md) — инструкция по использованию
- [Руководство по установке](docs/INSTALL.md) — установка и настройка
- [Changelog](CHANGELOG.md) — история изменений

## Быстрый старт

### Backend

```bash
cd backend
make build
make run
```

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
│   ├── cmd/server/    # Точка входа
│   ├── internal/      # Внутренние пакеты
│   │   ├── api/       # HTTP API handlers
│   │   ├── auth/      # Аутентификация
│   │   ├── metrics/   # Prometheus метрики
│   │   ├── models/    # Модели данных
│   │   ├── p2p/       # WebRTC P2P сервис
│   │   ├── sync/      # Сервис синхронизации
│   │   ├── torrent/   # Торрент сервис
│   │   └── version/   # Информация о версии
│   └── pkg/logger/    # Логгер
├── frontend/          # Qt/C++ frontend
│   ├── src/           # Исходный код
│   └── resources/     # Ресурсы (иконки и т.д.)
├── docs/              # Документация
│   ├── USER_GUIDE.md  # Руководство пользователя
│   └── INSTALL.md     # Руководство по установке
├── Dockerfile         # Docker образ
├── docker-compose.yml # Docker Compose конфигурация
└── CHANGELOG.md       # История изменений
```

## API

### Версия
```
GET /api/v1/version
```

### Health Check
```
GET /health
GET /health/detailed
```

### Метрики
```
GET /metrics
```

## Лицензия

[MIT](LICENSE)
