.PHONY: build run clean test lint help check-version test-race

# Переменные
BINARY_NAME=torrsyncplayer
BUILD_DIR=./build
MAIN_FILE=./cmd/server/main.go

# Версионирование
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

# LDFLAGS для версионирования
LDFLAGS=-ldflags "-X github.com/blagovibe/TorrSyncPlayer/backend/internal/version.Version=$(VERSION) \
	-X github.com/blagovibe/TorrSyncPlayer/backend/internal/version.Commit=$(COMMIT) \
	-X github.com/blagovibe/TorrSyncPlayer/backend/internal/version.BuildTime=$(BUILD_TIME)"

# Цель по умолчанию
all: check-version build

## check-version: Проверить версию Go
check-version:
	@echo "Проверка версии Go..."
	@go version
	@go version | grep -q "go1.2[0-9]" || echo "Warning: Go 1.20+ required"

## build: Собрать проект
build:
	@echo "Сборка $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "Сборка завершена: $(BUILD_DIR)/$(BINARY_NAME)"

## build-prod: Собрать оптимизированную версию
build-prod:
	@echo "Сборка $(BINARY_NAME) (production)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -ldflags "-s -w" -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "Сборка завершена: $(BUILD_DIR)/$(BINARY_NAME)"

## run: Запустить сервер
run:
	@echo "Запуск $(BINARY_NAME)..."
	go run $(MAIN_FILE)

## dev: Запустить сервер в режиме разработки
dev:
	@echo "Запуск $(BINARY_NAME) (dev mode)..."
	LOG_LEVEL=debug go run $(MAIN_FILE)

## test: Запустить тесты
test:
	@echo "Запуск тестов..."
	go test ./...

## test-verbose: Запустить тесты с подробным выводом
test-verbose:
	@echo "Запуск тестов (verbose)..."
	go test -v ./...

## test-race: Запустить тесты с детектором гонок (требуется gcc)
test-race:
	@echo "Запуск тестов с race detector..."
	@which gcc > /dev/null 2>&1 || (echo "Ошибка: gcc не найден. Установите MinGW или TDM-GCC для использования race detector." && exit 1)
	CGO_ENABLED=1 go test -race -v ./...

## test-coverage: Запустить тесты с отчётом о покрытии
test-coverage:
	@echo "Запуск тестов с покрытием..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Отчёт о покрытии: coverage.html"

## test-short: Запустить только быстрые тесты (без интеграционных)
test-short:
	@echo "Запуск быстрых тестов..."
	go test -short ./...

## clean: Очистить сборку
clean:
	@echo "Очистка..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Очистка завершена"

## lint: Запустить линтер
lint:
	@echo "Запуск линтера..."
	go vet ./...

## fmt: Форматировать код
fmt:
	@echo "Форматирование кода..."
	go fmt ./...

## tidy: Обновить зависимости
tidy:
	@echo "Обновление зависимостей..."
	go mod tidy

## deps: Установить зависимости
deps:
	@echo "Установка зависимостей..."
	go mod download

## help: Показать справку
help:
	@echo "Доступные команды:"
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## /  /'
