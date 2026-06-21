# Root Makefile for TorrSyncPlayer

.PHONY: all backend frontend test clean build

all: backend

backend:
	$(MAKE) -C backend build

frontend:
	mkdir -p frontend/build
	cd frontend/build && cmake .. -DCMAKE_BUILD_TYPE=Release && $(MAKE) -C frontend/build

test:
	$(MAKE) -C backend test

test-race:
	$(MAKE) -C backend test-race

build: backend frontend

clean:
	$(MAKE) -C backend clean
	rm -rf frontend/build
