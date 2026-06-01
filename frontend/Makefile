.PHONY: build run clean test lint help

# Переменные
BUILD_DIR=./build
TARGET=TorrSyncPlayer
CMAKE=cmake
NINJA=ninja

# Определяем ОС
ifeq ($(OS),Windows_NT)
    CMAKE_GENERATOR?="Ninja"
    RUN_CMD=./$(BUILD_DIR)/$(TARGET).exe
else
    CMAKE_GENERATOR?=Ninja
    RUN_CMD=./$(BUILD_DIR)/$(TARGET)
endif

# Цель по умолчанию
all: build

## build: Собрать frontend
build:
	@echo "Сборка $(TARGET)..."
	@mkdir -p $(BUILD_DIR)
	cd $(BUILD_DIR) && $(CMAKE) .. -G $(CMAKE_GENERATOR) -DCMAKE_BUILD_TYPE=Release
	$(CMAKE) --build $(BUILD_DIR)
	@echo "Сборка завершена: $(BUILD_DIR)/$(TARGET)"

## build-debug: Собрать debug версию
build-debug:
	@echo "Сборка $(TARGET) (debug)..."
	@mkdir -p $(BUILD_DIR)
	cd $(BUILD_DIR) && $(CMAKE) .. -G $(CMAKE_GENERATOR) -DCMAKE_BUILD_TYPE=Debug
	$(CMAKE) --build $(BUILD_DIR)
	@echo "Сборка завершена: $(BUILD_DIR)/$(TARGET)"

## run: Запустить frontend
run: build
	@echo "Запуск $(TARGET)..."
	$(RUN_CMD)

## test: Запустить тесты
test: build
	@echo "Запуск тестов..."
	cd $(BUILD_DIR) && ctest --output-on-failure

## clean: Очистить сборку
clean:
	@echo "Очистка..."
	@rm -rf $(BUILD_DIR)
	@echo "Очистка завершена"

## lint: Запустить clang-tidy
lint:
	@echo "Запуск clang-tidy..."
	clang-tidy src/*.cpp -- -I/usr/include/qt6 -I/usr/include/qt6/QtCore -I/usr/include/qt6/QtGui -I/usr/include/qt6/QtWidgets -I/usr/include/qt6/QtNetwork
	@echo "Lint завершён"

## format: Форматировать код через clang-format
format:
	@echo "Форматирование кода..."
	clang-format -i src/*.cpp src/*.h
	@echo "Форматирование завершено"

## help: Показать справку
help:
	@echo "Доступные команды:"
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## /  /'
