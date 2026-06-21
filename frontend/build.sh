#!/bin/bash
# ── Скрипт сборки TorrSyncPlayer для Linux/macOS ───────────────────────────
# Требования: Qt6, libmpv, CMake 3.16+, pkg-config

set -e  # Выход при ошибке

# ── Цвета для вывода ─────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# ── Функции ───────────────────────────────────────────────────────────
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# ── Проверка зависимостей ────────────────────────────────────────────
check_dependency() {
    if ! command -v "$1" &> /dev/null; then
        log_error "$1 не найден. Установите его."
        exit 1
    fi
}

log_info "Проверка зависимостей..."
check_dependency cmake
check_dependency pkg-config

# Проверка Qt6
if ! pkg-config --exists Qt6Core; then
    log_error "Qt6 не найден. Установите Qt6 development packages."
    exit 1
fi

# Проверка libmpv
if ! pkg-config --exists libmpv; then
    log_error "libmpv не найден. Установите libmpv-dev."
    exit 1
fi

log_info "Все зависимости найдены"

# ── Параметры сборки ─────────────────────────────────────────────────
BUILD_TYPE="${1:-Release}"
BUILD_DIR="build"
JOBS=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)

log_info "Тип сборки: $BUILD_TYPE"
log_info "Параллельных задач: $JOBS"

# ── Создание директории сборки ───────────────────────────────────────
if [ -d "$BUILD_DIR" ]; then
    log_warn "Директория $BUILD_DIR существует, очистка..."
    rm -rf "$BUILD_DIR"
fi

mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

# ── Конфигурация CMake ───────────────────────────────────────────────
log_info "Конфигурация CMake..."
cmake .. \
    -DCMAKE_BUILD_TYPE="$BUILD_TYPE" \
    -DCMAKE_EXPORT_COMPILE_COMMANDS=ON

# ── Сборка ───────────────────────────────────────────────────────────
log_info "Сборка..."
cmake --build . --parallel "$JOBS"

# ── Результат ────────────────────────────────────────────────────────
if [ -f "./TorrSyncPlayer" ]; then
    log_info "Сборка завершена успешно!"
    log_info "Исполняемый файл: $(pwd)/TorrSyncPlayer"
    
    # Копирование ресурсов если нужно
    if [ -d "../resources" ]; then
        log_info "Копирование ресурсов..."
        cp -r ../resources .
    fi
else
    log_error "Сборка завершилась с ошибкой"
    exit 1
fi

# ── Запуск (опционально) ─────────────────────────────────────────────
if [ "${2:-}" = "run" ]; then
    log_info "Запуск TorrSyncPlayer..."
    ./TorrSyncPlayer
fi
