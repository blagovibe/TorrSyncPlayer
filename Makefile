.PHONY: all backend frontend clean test lint help

# Цель по умолчанию
all: backend frontend

## backend: Собрать backend
backend:
	$(MAKE) -C backend build

## frontend: Собрать frontend
frontend:
	$(MAKE) -C frontend build

## test: Запустить все тесты
test:
	$(MAKE) -C backend test
	$(MAKE) -C frontend test

## lint: Запустить линтеры
lint:
	$(MAKE) -C backend lint
	$(MAKE) -C frontend lint

## clean: Очистить сборку
clean:
	$(MAKE) -C backend clean
	$(MAKE) -C frontend clean

## run-backend: Запустить backend сервер
run-backend:
	$(MAKE) -C backend run

## run-frontend: Запустить frontend
run-frontend:
	$(MAKE) -C frontend run

## dev: Запустить в режиме разработки
dev:
	$(MAKE) -C backend dev

## help: Показать справку
help:
	@echo "Доступные команды:"
	@echo "  make all          - Собрать backend и frontend"
	@echo "  make backend      - Собрать backend"
	@echo "  make frontend     - Собрать frontend"
	@echo "  make test         - Запустить все тесты"
	@echo "  make lint         - Запустить линтеры"
	@echo "  make clean        - Очистить сборку"
	@echo "  make run-backend  - Запустить backend сервер"
	@echo "  make run-frontend - Запустить frontend"
	@echo "  make dev          - Запустить в режиме разработки"
