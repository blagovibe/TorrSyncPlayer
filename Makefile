.PHONY: all backend frontend clean test lint help check

# Default target
all: backend frontend

## backend: Build backend
backend:
	$(MAKE) -C backend build

## frontend: Build frontend
frontend:
	$(MAKE) -C frontend build

## test: Run all tests
test:
	$(MAKE) -C backend test
	$(MAKE) -C frontend test

## check: Run all checks (vet, test, race)
check:
	$(MAKE) -C backend check

## lint: Run linters
lint:
	$(MAKE) -C backend lint
	$(MAKE) -C frontend lint

## clean: Clean build artifacts
clean:
	$(MAKE) -C backend clean
	$(MAKE) -C frontend clean

## run-backend: Run backend server
run-backend:
	$(MAKE) -C backend run

## run-frontend: Run frontend
run-frontend:
	$(MAKE) -C frontend run

## dev: Run in development mode
dev:
	$(MAKE) -C backend dev

## help: Show help
help:
	@echo "Available commands:"
	@echo "  make all          - Build backend and frontend"
	@echo "  make backend      - Build backend"
	@echo "  make frontend     - Build frontend"
	@echo "  make test         - Run all tests"
	@echo "  make check        - Run vet, tests, and race detection"
	@echo "  make lint         - Run linters"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make run-backend  - Run backend server"
	@echo "  make run-frontend - Run frontend"
	@echo "  make dev          - Run in development mode"
