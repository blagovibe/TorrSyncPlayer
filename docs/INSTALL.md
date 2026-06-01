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
- **RAM:** 4 ГБ
- **Диск:** 500 МБ для установки + место для кэша торрентов
- **Сеть:** стабильное интернет-соединение

### Рекомендуемые требования

- **ОС:** Windows 11, Ubuntu 22.04+, macOS 13+
- **RAM:** 8 ГБ
- **Диск:** SSD с 10 ГБ свободного места
- **Сеть:** 100 Мбит/с или выше

## Установка зависимостей

### Backend (Go)

#### Ubuntu/Debian

```bash
# Установка Go 1.25+
wget https://go.dev/dl/go1.25.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

#### macOS

```bash
brew install go@1.25
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
git clone https://github.com/yourname/torrplayer.git
cd torrplayer
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
mkdir build
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

1. Перейдите на страницу [Releases](https://github.com/yourname/torrplayer/releases)
2. Скачайте архив для вашей ОС
3. Распакуйте в удобное место

### Установка

#### Linux

```bash
tar -xzf torrplayer-linux-amd64.tar.gz -C /opt/
sudo ln -s /opt/torrplayer/bin/torrplayer /usr/local/bin/
```

#### macOS

```bash
unzip torrplayer-macos-amd64.zip
cp -r TorrPlayer.app /Applications/
```

#### Windows

Распакуйте архив и запустите `TorrPlayer.exe`.

## Docker

### Сборка образа

```bash
docker build -t torrplayer:latest .
```

### Запуск контейнера

```bash
docker run -d \
    --name torrplayer \
    -p 8889:8889 \
    -v torrplayer-data:/data \
    torrplayer:latest
```

### Docker Compose

```bash
docker-compose up -d
```

## Настройка

### Переменные окружения

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `PORT` | 8889 | Порт HTTP сервера |
| `DATA_DIR` | ./data | Директория для данных |
| `JWT_SECRET` | (пусто) | Секрет для JWT токенов |
| `LOG_LEVEL` | info | Уровень логирования |
| `LOG_FORMAT` | text | Формат логов (text/json) |
| `TLS_CERT` | (пусто) | Путь к TLS сертификату |
| `TLS_KEY` | (пусто) | Путь к TLS ключу |

### Конфигурационный файл

Создайте `config.yaml` в директории данных:

```yaml
server:
  port: 8889
  data_dir: ./data
  
auth:
  jwt_secret: "your-secret-key-here"
  
logging:
  level: info
  format: json
  
tls:
  enabled: false
  cert_file: ""
  key_file: ""
```

### Запуск как службы

#### Linux (systemd)

```bash
sudo cp torrplayer.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable torrplayer
sudo systemctl start torrplayer
```

#### macOS (launchd)

```bash
cp com.torrplayer.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.torrplayer.plist
```

#### Windows (NSSM)

```bash
nssm install TorrPlayer "C:\path\to\torrplayer.exe"
nssm start TorrPlayer
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
docker pull torrplayer:latest
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
sudo systemctl stop torrplayer
sudo systemctl disable torrplayer
sudo rm /etc/systemd/system/torrplayer.service
sudo rm -rf /opt/torrplayer
```

### macOS

```bash
launchctl unload ~/Library/LaunchAgents/com.torrplayer.plist
rm ~/Library/LaunchAgents/com.torrplayer.plist
rm -rf /Applications/TorrPlayer.app
```

### Windows

1. Остановите службу: `nssm stop TorrPlayer`
2. Удалите службу: `nssm remove TorrPlayer`
3. Удалите директорию установки

### Docker

```bash
docker-compose down
docker rmi torrplayer:latest
docker volume rm torrplayer-data
```
