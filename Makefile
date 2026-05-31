# TorrSyncPlayer Wails Makefile

.PHONY: build dev clean install generate test

# Переменные
APP_NAME=TorrSyncPlayer
BUILD_DIR=build/bin
FRONTEND_DIR=frontend

# Установка зависимостей
install:
	cd $(FRONTEND_DIR) && npm install
	go mod download

# Генерация Wails bindings
generate:
	wails generate module

# Режим разработки
dev:
	wails dev

# Сборка для текущей платформы
build:
	wails build

# Сборка для Windows
build-windows:
	wails build -platform windows/amd64

# Сборка для Linux
build-linux:
	wails build -platform linux/amd64

# Сборка для macOS
build-macos:
	wails build -platform darwin/amd64

# Сборка для всех платформ
build-all: build-windows build-linux build-macos

# Очистка
clean:
	rm -rf $(BUILD_DIR)
	rm -rf $(FRONTEND_DIR)/dist
	rm -rf $(FRONTEND_DIR)/node_modules

# Тесты
test:
	go test ./...

test-verbose:
	go test -v ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Линтинт
lint:
	go vet ./...
	cd $(FRONTEND_DIR) && npm run lint

# Запуск в режиме разработки с отладкой
dev-debug:
	wails dev -debug

# Сборка с отладочной информацией
build-debug:
	wails build -debug

# Сборка для production
build-prod:
	wails build -ldflags "-s -w"

# Создание иконок (если нужно)
icons:
	wails generate icons
