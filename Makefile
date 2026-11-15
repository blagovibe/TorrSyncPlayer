# Root Makefile for TorrSyncPlayer
#
# Both halves are Go now: the backend and the Fyne client. There is no CMake in
# this project any more, and a recipe that still calls it can only fail — the
# Qt client it was written for is gone (idea-0067, bug-0069).

.PHONY: all backend frontend test clean build release \
    test-backend test-backend-race test-backend-coverage test-backend-fuzz test-backend-mutation \
    test-frontend test-frontend-race test-frontend-coverage \
    contract-test contract-test-backend contract-test-frontend \
    integration-test \
    load-test chaos-test \
    test-all test-ci test-full

# The frontend needs cgo and a GL toolchain, and its tests need a driver that
# does not require a display. Without FYNE_DRIVER=offscreen the tests wait for a
# window instead of failing, so there would be no green run — and no red one
# either. CGO_ENABLED=1 matches CI.
FYNE_ENV = FYNE_DRIVER=offscreen CGO_ENABLED=1

all: backend

backend:
	$(MAKE) -C backend build

# The client and the backend are a pair that cannot be separated at run time:
# every participant runs its own backend, and the client looks for it beside
# itself (internal/backend.FindBackend). This target builds the pair.
frontend:
	cd frontend && go run ./cmd/build

# Backend test targets
test-backend:
	$(MAKE) -C backend test

test-backend-race:
	$(MAKE) -C backend test-race

test-backend-coverage:
	$(MAKE) -C backend test-coverage

test-backend-fuzz:
	$(MAKE) -C backend test-fuzz

test-backend-mutation:
	$(MAKE) -C backend test-mutation

test-backend-contract:
	$(MAKE) -C backend contract-test

# Frontend test targets
test-frontend:
	cd frontend && $(FYNE_ENV) go test ./...

test-frontend-race:
	cd frontend && $(FYNE_ENV) go test -race ./...

test-frontend-coverage:
	cd frontend && $(FYNE_ENV) go test -coverprofile=coverage.out -covermode=atomic ./...
	cd frontend && go tool cover -func=coverage.out | tail -1

# No frontend fuzz or mutation targets exist, and none are declared. The
# recipes that used to be here were C++ ones for targets that did not exist —
# every command was absorbed by `|| true`, which is why the gap stayed hidden.
# Go fuzzing is real and runs via `make test-backend-fuzz`; keep the list there
# in step with backend/internal/validation.

# Contract testing
#
# The contract package is an ordinary test package: it replays the fixture in
# pacts/frontend-backend.json against the real router (status, Content-Type,
# body shape by matchingRules). No build tag, no cgo, no FFI — `make test-all`
# runs it through test-backend like any other package; this target exists for
# convenience and names it explicitly.
contract-test-backend:
	cd backend && go test -v ./internal/contract/...

# The Pact consumer for the client does not exist, and this is not a dead
# recipe pretending otherwise: the contract is verified from the provider side,
# and the client's conformance to it is what the scenario tests check — by
# running the real backend binary and driving the real client against it. See
# DEV.md, "Соглашения по тестам".
contract-test-frontend:
	@echo "SKIPPED: no Pact consumer on the client; the scenario tests cover conformance"

contract-test: contract-test-backend contract-test-frontend

# Integration tests
#
# Two ends of the same seam. The backend runs its HTTP router for real
# (TestE2E); the frontend starts the real backend binary and drives the real
# client against it. Module tests cannot stand in for either: they feed the
# parser the shape the parser expects, so code and test can agree with each
# other and both disagree with the server — which is exactly how the P2PEvent
# envelope bug survived.
integration-test:
	$(MAKE) -C backend test-integration
	cd frontend && $(FYNE_ENV) go test -timeout 10m ./internal/integration/...

# Load/performance tests
load-test:
	@echo "Running k6 load tests..."
	@echo "Starting backend for load testing..."
	cd backend && make build && \
	./build/torrsyncplayer --port 8889 --auto-tls & \
	sleep 5 && \
	curl -k -s https://localhost:8889/health
	cd tests/load/k6 && k6 run mixed_scenario.js

# Chaos tests
chaos-test:
	@echo "Running chaos tests with Toxiproxy..."
	@echo "Requires docker-compose with toxiproxy service"
	cd tests/chaos && ./run_chaos.sh

# Combined test targets
test-all: test-backend test-frontend contract-test integration-test
	@echo "All tests completed!"

test-ci: test-backend-race test-frontend-race contract-test
	@echo "CI test suite completed!"

test-full: test-backend test-backend-race test-backend-coverage test-backend-fuzz \
           test-frontend test-frontend-race test-frontend-coverage \
           contract-test integration-test
	@echo "Full test suite completed!"

build: backend frontend

clean:
	$(MAKE) -C backend clean
	rm -rf frontend/dist frontend/coverage.out frontend/mutation.log

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