# Руководство для контрибьюторов TorrSyncPlayer

Спасибо за интерес к проекту TorrSyncPlayer! Это руководство поможет вам начать работу с кодовой базой и внести свой вклад.

## Содержание

1. [Требования к окружению](#требования-к-окружению)
2. [Настройка рабочей среды](#настройка-рабочей-среды)
3. [Процесс сборки](#процесс-сборки)
4. [Стиль кода](#стиль-кода)
5. [Запуск тестов](#запуск-тестов)
6. [Процесс создания Pull Request](#процесс-создания-pull-request)
7. [Соглашения по коммитам](#соглашения-по-коммитам)
8. [Структура проекта](#структура-проекта)

---

## Требования к окружению

### Backend (Go)

| Компонент | Минимальная версия | Рекомендуемая |
|-----------|-------------------|---------------|
| Go        | 1.25+             | Последняя stable |
| Make      | 4.0+              | Последняя      |

### Frontend (C++/Qt)

| Компонент | Минимальная версия | Рекомендуемая |
|-----------|-------------------|---------------|
| C++       | C++17             | C++20         |
| Qt        | 6.5+              | Последняя LTS |
| CMake     | 3.16+             | Последняя      |
| libmpv    | 0.35+             | Последняя      |

### Операционные системы

- **Windows:** Windows 10+ с MSVC 2022 или MinGW
- **Linux:** Ubuntu 20.04+ / Fedora 35+ / Arch Linux
- **macOS:** macOS 12+ с Xcode Command Line Tools

---

## Настройка рабочей среды

### 1. Клонирование репозитория

```bash
git clone https://github.com/blagovibe/TorrSyncPlayer.git
cd TorrSyncPlayer
```

### 2. Установка Go (Backend)

**Ubuntu/Debian:**
```bash
wget https://go.dev/dl/go1.25.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

**macOS:**
```bash
brew install go@1.25
```

**Windows:**
Скачайте и установите с [официального сайта](https://go.dev/dl/).

### 3. Установка Qt и libmpv (Frontend)

**Ubuntu/Debian:**
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

**macOS:**
```bash
brew install qt@6 mpv cmake ninja
```

**Windows:**
1. Установите Qt 6.5+ с [официального сайта](https://www.qt.io/download)
2. Установите libmpv через vcpkg или скачайте бинарники

### 4. Проверка установки

```bash
# Go
go version

# CMake
cmake --version

# Qt (проверка через qmake)
qmake --version
```

---

## Процесс сборки

### Сборка Backend

```bash
cd backend
make build
```

Исполняемый файл будет в `backend/build/`.

### Сборка Frontend

**Linux/macOS:**
```bash
cd frontend
mkdir -p build
cd build
cmake .. -G Ninja -DCMAKE_BUILD_TYPE=Release
ninja
```

**Windows:**
```bash
cd frontend
mkdir build
cd build
cmake .. -G "Visual Studio 17 2022" -A x64
cmake --build . --config Release
```

### Сборка всего проекта

```bash
# Из корня проекта
make all
```

### Очистка

```bash
# Backend
cd backend
make clean

# Frontend
cd frontend/build
ninja clean

# Всё
make clean
```

---

## Стиль кода

### Go (Backend)

#### Форматирование

- Используйте `gofmt` для форматирования кода
- Используйте `go vet` для статического анализа
- Настройки редактора в [`.editorconfig`](../.editorconfig)

```bash
# Форматирование
gofmt -w .

# Проверка
go vet ./...
```

#### Соглашения

1. **Именование:**
   - CamelCase для экспортируемых функций и типов
   - camelCase для неэкспортируемых
   - Аббревиатуры в верхнем регистре: `HTTPClient`, `URLParser`

2. **Обработка ошибок:**
   ```go
   // Всегда оборачивайте ошибки
   if err != nil {
       return fmt.Errorf("описание контекста: %w", err)
   }
   ```

3. **Логирование:**
   ```go
   // Используйте структурированный логгер
   logger.Info("Операция выполнена", "key1", value1, "key2", value2)
   logger.Error("Ошибка", "error", err)
   ```

4. **Комментарии:**
   - Комментарии к коду на русском языке
   - Экспортируемые функции должны иметь godoc-комментарии
   ```go
   // AddMagnet добавляет торрент по magnet-ссылке.
   // Параметр ctx - контекст для отмены операции.
   // Возвращает информацию о торренте или ошибку.
   func (s *Service) AddMagnet(ctx context.Context, magnetURI string) (*models.TorrentInfo, error) {
   ```

5. **Константы:**
   - Выносите магические числа в именованные константы
   ```go
   const (
       gracefulShutdownTimeout = 30 * time.Second
       dataDirPermissions     = 0755
   )
   ```

### C++ (Frontend)

#### Форматирование

- Настройки редактора в [`.editorconfig`](../.editorconfig)
- Используйте clang-format для автоматического форматирования

#### Соглашения

1. **Именование (Qt стиль):**
   - camelCase для методов: `addTorrent()`, `onPlayPause()`
   - camelCase для переменных: `m_torrentList`, `m_network`
   - PascalCase для классов: `MainWindow`, `NetworkManager`

2. **Макросы Qt:**
   ```cpp
   class MainWindow : public QMainWindow
   {
       Q_OBJECT  // Всегда в начале класса с сигналами/слотами
   public:
       // ...
   signals:
       void torrentAdded(const QJsonObject &torrent);
   private slots:
       void onAddTorrent();
   };
   ```

3. **Комментарии:**
   - Комментарии к коду на русском языке
   - Используйте Doxygen-стиль для документации классов
   ```cpp
   /**
    * @brief Добавить торрент по magnet-ссылке
    * @param magnetUri Magnet-ссылка на торрент
    */
   void addTorrent(const QString &magnetUri);
   ```

4. **Управление памятью:**
   - Используйте родительские объекты Qt для автоматического удаления
   - Для не-Qt объектов используйте `std::unique_ptr` или `std::shared_ptr`

---

## Запуск тестов

### Backend тесты

```bash
cd backend

# Запуск всех тестов
make test

# Запуск тестов с покрытием
go test -cover ./...

# Запуск тестов конкретного пакета
go test ./internal/torrent/...
go test ./internal/p2p/...
go test ./internal/sync/...
go test ./internal/auth/...
go test ./internal/validation/...

# Запуск с подробным выводом
go test -v ./...

# Генерация отчёта о покрытии
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Frontend тесты

```bash
cd frontend/build

# Запуск тестов через CTest
ctest --output-on-failure

# Или напрямую
./test_networkmanager
./test_torrentmodel
```

### Интеграционные тесты

```bash
cd backend

# Запуск интеграционных тестов
go test -tags=integration ./internal/api/...
```

---

## Процесс создания Pull Request

### 1. Создание ветки

```bash
# Создайте ветку от main
git checkout -b feature/your-feature-name
# или
git checkout -b fix/your-bug-fix
```

### 2. Внесение изменений

- Пишите чистый, читаемый код
- Следуйте соглашениям по стилю кода
- Добавляйте тесты для новой функциональности
- Обновляйте документацию при необходимости

### 3. Перед отправкой

```bash
# Убедитесь, что все тесты проходят
make test

# Проверьте форматирование
gofmt -l .
go vet ./...

# Проверьте, что сборка проходит
make all
```

### 4. Создание Pull Request

1. Запушите ветку:
   ```bash
   git push origin feature/your-feature-name
   ```

2. Создайте Pull Request на GitHub

3. Заполните шаблон PR:
   - **Описание:** Что было изменено и почему
   - **Тип изменения:** Feature / Bug fix / Documentation / Refactor
   - **Тестирование:** Как были протестированы изменения
   - **Связанные issues:** Ссылки на связанные задачи

### 5. Code Review

- Ожидайте ревью от мейнтейнеров
- Вносите исправления по комментариям
- Убедитесь, что CI проходит

---

## Соглашения по коммитам

Используем формат [Conventional Commits](https://www.conventionalcommits.org/ru/):

### Формат

```
<тип>(<область>): <краткое описание>

<подробное описание (опционально)>

<footer (опционально)>
```

### Типы коммитов

| Тип | Описание |
|-----|----------|
| `feat` | Новая функциональность |
| `fix` | Исправление бага |
| `docs` | Изменения в документации |
| `style` | Форматирование (не влияет на код) |
| `refactor` | Рефакторинг кода |
| `test` | Добавление/изменение тестов |
| `chore` | Вспомогательные изменения |
| `perf` | Улучшение производительности |
| `ci` | Изменения в CI/CD |

### Примеры

```
feat(torrent): добавлена поддержка magnet-ссылок

- Реализована валидация формата magnet URI
- Добавлена обработка таймаута получения метаданных
- Добавлены тесты для новых функций

Closes #123
```

```
fix(p2p): исправлен баг с закрытием DataChannel

DataChannel не закрывался корректно при выходе из комнаты,
что приводило к утечке ресурсов.

Fixes #456
```

```
docs(readme): обновлено руководство по установке

- Добавлены инструкции для Windows
- Исправлены ссылки на документацию
```

```
refactor(sync): вынесены магические числа в константы

- maxPositionJump = 5.0
- smoothAdjustmentRatio = 0.3
- msPerSecond = 1000.0
```

---

## Структура проекта

```
TorrSyncPlayer/
├── backend/                    # Go backend
│   ├── cmd/server/             # Точка входа
│   │   └── main.go
│   ├── internal/               # Внутние пакеты
│   │   ├── api/                # HTTP API
│   │   ├── auth/               # Аутентификация
│   │   ├── constants/          # Константы
│   │   ├── errors/             # Обработка ошибок
│   │   ├── metrics/            # Prometheus метрики
│   │   ├── models/             # Модели данных
│   │   ├── p2p/                # P2P сервис
│   │   ├── sync/               # Сервис синхронизации
│   │   ├── torrent/            # Торрент сервис
│   │   ├── validation/         # Валидация
│   │   ├── version/            # Версия
│   │   └── interfaces.go       # Интерфейсы
│   ├── pkg/logger/             # Логгер
│   ├── Makefile
│   └── go.mod
│
├── frontend/                   # Qt/C++ frontend
│   ├── src/                    # Исходный код
│   │   ├── main.cpp
│   │   ├── mainwindow.h/.cpp
│   │   ├── mpvwidget.h/.cpp
│   │   ├── networkmanager.h/.cpp
│   │   ├── torrentmodel.h/.cpp
│   │   ├── torrentmanager.h/.cpp
│   │   ├── roommanager.h/.cpp
│   │   ├── roomdialog.h/.cpp
│   │   ├── systemtray.h/.cpp
│   │   └── ...
│   ├── resources/              # Ресурсы
│   ├── CMakeLists.txt
│   └── build.sh / build.bat
│
├── docs/                       # Документация
│   ├── ARCHITECTURE.md
│   ├── API.md
│   ├── INSTALL.md
│   └── USER_GUIDE.md
│
├── .editorconfig               # Настройки редактора
├── .gitignore
├── CHANGELOG.md
├── CONTRIBUTING.md
├── LICENSE
├── Makefile
├── README.md
├── Dockerfile
└── docker-compose.yml
```

---

## Полезные ссылки

- [Документация по архитектуре](docs/ARCHITECTURE.md)
- [API документация](docs/API.md)
- [Руководство пользователя](docs/USER_GUIDE.md)
- [Руководство по установке](docs/INSTALL.md)
- [История изменений](CHANGELOG.md)

---

## Контакты

Если у вас есть вопросы, создайте [Issue](https://github.com/blagovibe/TorrSyncPlayer/issues) на GitHub.
