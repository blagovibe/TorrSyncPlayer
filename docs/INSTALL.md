# Руководство по установке TorrSyncPlayer

## Содержание

1. [Системные требования](#системные-требования)
2. [Установка зависимостей](#установка-зависимостей)
3. [Сборка из исходников](#сборка-из-исходников)
4. [Установка из релизов](#установка-из-релизов)
5. [Docker](#docker)
6. [Настройка](#настройка)
7. [Обновление](#обновление)

## Системные требования

### Минимальные требования

- **ОС:** Windows 10+, Ubuntu 20.04+, macOS 12+
- **RAM:** 8 ГБ (данные торрентов хранятся в оперативной памяти)
- **Диск:** 500 МБ для установки
- **Сеть:** стабильное интернет-соединение

> **Примечание:** Все данные торрентов хранятся в оперативной памяти (in-memory storage). Убедитесь, что у вас достаточно RAM для загружаемого контента.

### Рекомендуемые требования

- **ОС:** Windows 11, Ubuntu 22.04+, macOS 13+
- **RAM:** 16 ГБ (для комфортной работы с большими торрентами)
- **Диск:** SSD с 1 ГБ свободного места
- **Сеть:** 100 Мбит/с или выше

## Установка зависимостей

### Backend (Go)

#### Ubuntu/Debian

```bash
# Установка Go 1.24+
wget https://go.dev/dl/go1.24.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

#### macOS

```bash
brew install go@1.24
```

#### Windows

Скачайте и установите Go с [официального сайта](https://go.dev/dl/).

### Frontend (Qt + libmpv)

#### Ubuntu/Debian

```bash
sudo apt update
sudo apt install -y \
    build-essential \
    cmake \
    ninja-build \
    qt6-base-dev \
    qt6-multimedia-dev \
    libmpv-dev \
    libgl1-mesa-dev
```

#### macOS

```bash
brew install qt@6 mpv cmake ninja
```

#### Windows

1. Установите Qt 6.5+ с [официального сайта](https://www.qt.io/download)
2. Установите libmpv через vcpkg или скачайте бинарники

## Сборка из исходников

### Клонирование репозитория

```bash
git clone https://github.com/blagovibe/TorrSyncPlayer.git
cd TorrSyncPlayer
```

### Сборка backend

```bash
cd backend
make build
```

Исполняемый файл будет в `backend/build/` или `backend/bin/`.

### Сборка frontend

#### Linux/macOS

```bash
cd frontend
mkdir -p build
cd build
cmake .. -G Ninja -DCMAKE_BUILD_TYPE=Release
ninja
```

#### Windows

```bash
cd frontend
mkdir build
cd build
cmake .. -G "Visual Studio 17 2022" -A x64
cmake --build . --config Release
```

### Сборка всего проекта

```bash
make all
```

## Установка из релизов

### Скачивание

1. Перейдите на страницу [Releases](https://github.com/blagovibe/TorrSyncPlayer/releases)
2. Скачайте архив для вашей ОС:
   - `TorrSyncPlayer-linux-x64.tar.gz` — Linux x64
   - `TorrSyncPlayer-windows-x64.zip` — Windows x64
   - `TorrSyncPlayer-macos-arm64.tar.gz` — macOS ARM64
3. Распакуйте в удобное место

### Установка

#### Linux

```bash
tar -xzf TorrSyncPlayer-linux-x64.tar.gz -C /opt/
sudo ln -s /opt/TorrSyncPlayer/bin/server /usr/local/bin/torrserver
```

#### macOS

```bash
tar -xzf TorrSyncPlayer-macos-arm64.tar.gz -C /Applications/
```

#### Windows

Распакуйте архив и запустите `TorrSyncPlayer.exe`.

## Docker

### Сборка образа

```bash
docker build -t torrsyncplayer:latest .
```

### Запуск контейнера

```bash
docker run -d \
    --name torrsyncplayer \
    -p 8889:8889 \
    torrsyncplayer:latest
```

### Docker Compose

```bash
# Запуск только backend
docker-compose up -d

# Запуск backend + Prometheus + Grafana
docker-compose --profile monitoring up -d
```

### Docker Compose сервисы

| Сервис | Порт | Описание |
|--------|------|----------|
| backend | 8889 | TorrSyncPlayer backend |
| prometheus | 9090 | Prometheus метрики (profile: monitoring) |
| grafana | 3000 | Grafana дашборды (profile: monitoring) |

## Настройка

### Переменные окружения

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `PORT` | 8889 | Порт HTTP сервера |
| `JWT_SECRET` | (пусто) | Секрет для JWT токенов |
| `LOG_LEVEL` | info | Уровень логирования (debug/info/warn/error) |
| `LOG_FORMAT` | text | Формат логов (text/json) |
| `TLS_CERT` | (пусто) | Путь к TLS сертификату |
| `TLS_KEY` | (пусто) | Путь к TLS ключу |

### Флаги командной строки

| Флаг | По умолчанию | Описание |
|------|--------------|----------|
| `--port` | 8889 | Порт HTTP сервера |
| `--jwt-secret` | (пусто) | Секрет для JWT токенов |
| `--tls` | false | Включить TLS |
| `--auto-tls` | false | Генерировать self-signed сертификат |
| `--enable-profiling` | false | Включить pprof на порту 6060 |

### Запуск с параметрами

```bash
# С указанием порта
./server --port 8080

# С TLS
./server --tls --tls-cert /path/to/cert.pem --tls-key /path/to/key.pem

# С автогенерацией TLS сертификата
./server --auto-tls

# С профилированием
./server --enable-profiling
```

### Запуск как службы

#### Linux (systemd)

```bash
sudo cp torrsyncplayer.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable torrsyncplayer
sudo systemctl start torrsyncplayer
```

#### macOS (launchd)

```bash
cp com.torrsyncplayer.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.torrsyncplayer.plist
```

#### Windows (NSSM)

```bash
nssm install TorrSyncPlayer "C:\path\to\server.exe"
nssm start TorrSyncPlayer
```

## Обновление

### Обновление из исходников

```bash
git pull origin main
make clean
make all
```

### Обновление Docker

```bash
docker pull torrsyncplayer:latest
docker-compose up -d
```

### Обновление из релизов

1. Скачайте новую версию
2. Остановите текущую версию
3. Замените файлы
4. Запустите новую версию

## Удаление

### Linux

```bash
sudo systemctl stop torrsyncplayer
sudo systemctl disable torrsyncplayer
sudo rm /etc/systemd/system/torrsyncplayer.service
sudo rm -rf /opt/TorrSyncPlayer
```

### macOS

```bash
launchctl unload ~/Library/LaunchAgents/com.torrsyncplayer.plist
rm ~/Library/LaunchAgents/com.torrsyncplayer.plist
rm -rf /Applications/TorrSyncPlayer
```

### Windows

1. Остановите службу: `nssm stop TorrSyncPlayer`
2. Удалите службу: `nssm remove TorrSyncPlayer`
3. Удалите директорию установки

### Docker

```bash
docker-compose down
docker rmi torrsyncplayer:latest
```
