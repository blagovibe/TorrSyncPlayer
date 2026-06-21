# Root Makefile for TorrSyncPlayer

.PHONY: all backend frontend test clean build release

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

## release: Create and publish a release
# Usage: make release VERSION=v1.0.0
# Requires: clean working tree, a CHANGELOG.md [Unreleased] section, GitHub origin remote
release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make release VERSION=v1.0.0"; \
		exit 1; \
	fi
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: working tree is not clean. Commit or stash changes first."; \
		exit 1; \
	fi
	@if git rev-parse "$(VERSION)" >/dev/null 2>&1; then \
		echo "Error: tag $(VERSION) already exists."; \
		exit 1; \
	fi
	@echo "Releasing $(VERSION)..."
	@sed -i.bak "0,/^## \[Unreleased\]/{s/^## \[Unreleased\]/## [$(VERSION)] - $$(date -u '+%Y-%m-%d')/}" CHANGELOG.md
	@rm -f CHANGELOG.md.bak
	@git add CHANGELOG.md
	@git commit -m "chore: release $(VERSION)"
	@git tag "$(VERSION)"
	@git push origin main
	@git push origin "$(VERSION)"
	@echo "Release $(VERSION) pushed. GitHub Actions will build and publish binaries."
