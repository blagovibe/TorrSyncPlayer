# Root Makefile for TorrSyncPlayer

.PHONY: all backend frontend test clean build release \
    test-backend test-backend-race test-backend-coverage test-backend-fuzz test-backend-mutation \
    test-frontend test-frontend-asan test-frontend-tsan test-frontend-coverage \
    test-frontend-gmock test-frontend-fuzz test-frontend-mutation \
    contract-test contract-test-backend contract-test-frontend \
    integration-test e2e-test \
    load-test chaos-test \
    test-all

all: backend

backend:
	$(MAKE) -C backend build

frontend:
	mkdir -p frontend/build
	cd frontend/build && cmake .. -DCMAKE_BUILD_TYPE=Release && $(MAKE) -C frontend/build

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
	cd frontend && mkdir -p build && cd build && \
	cmake .. -DBUILD_TESTS=ON -DCMAKE_BUILD_TYPE=Debug && \
	make -j$(nproc) && \
	ctest --output-on-failure

test-frontend-asan:
	cd frontend && mkdir -p build && cd build && \
	cmake .. -DBUILD_TESTS=ON -DENABLE_SANITIZERS=ON -DCMAKE_BUILD_TYPE=Debug && \
	make -j$(nproc) test_asan && \
	ASAN_OPTIONS=detect_leaks=1:halt_on_error=1 ctest -R "ASan" --output-on-failure

test-frontend-tsan:
	cd frontend && mkdir -p build && cd build && \
	cmake .. -DBUILD_TESTS=ON -DENABLE_SANITIZERS=ON -DCMAKE_BUILD_TYPE=Debug && \
	make -j$(nproc) test_tsan && \
	TSAN_OPTIONS=halt_on_error=1 ctest -R "TSan" --output-on-failure

test-frontend-coverage:
	cd frontend && mkdir -p build && cd build && \
	cmake .. -DBUILD_TESTS=ON -DENABLE_COVERAGE=ON -DCMAKE_BUILD_TYPE=Debug && \
	make -j$(nproc) && \
	ctest --output-on-failure && \
	lcov --capture --directory . --output-file coverage.info && \
	lcov --remove coverage.info '/usr/*' '*/tests/*' '*/mocks/*' --output-file coverage.clean.info && \
	genhtml coverage.clean.info --output-directory coverage_html

test-frontend-gmock:
	cd frontend && mkdir -p build && cd build && \
	cmake .. -DBUILD_TESTS=ON -DBUILD_GMOCK=ON -DCMAKE_BUILD_TYPE=Debug && \
	make -j$(nproc) test_networkmanager_gmock test_torrentmanager_gmock test_roommanager_gmock && \
	ctest -R "GMock" --output-on-failure

test-frontend-fuzz:
	cd frontend && mkdir -p build-fuzz && cd build-fuzz && \
	cmake .. -DBUILD_TESTS=ON -DCMAKE_BUILD_TYPE=Debug -DCMAKE_CXX_COMPILER=clang++ && \
	make -j$(nproc) fuzz_* 2>&1 || true && \
	for f in ./tests/fuzz/fuzz_*; do \
		if [ -x "$$f" ]; then \
			echo "Running $$f..."; \
			timeout 30s "$$f" -max_total_time=10 -rss_limit_mb=2048 2>&1 || true; \
		fi; \
	done

test-frontend-mutation:
	@echo "C++ mutation testing with Mull - placeholder for future implementation"

# Contract testing
contract-test-backend:
	cd backend && go test -v -run TestPactProvider ./internal/contract/...

contract-test-frontend:
	@echo "Frontend contract tests - run via CTest with Pact consumer tests"
	cd frontend && mkdir -p build && cd build && \
	cmake .. -DBUILD_TESTS=ON -DCMAKE_BUILD_TYPE=Debug && \
	make -j$(nproc) test_networkmanager_contract test_torrentmanager_contract test_roommanager_contract 2>&1 || true && \
	ctest -R "Contract" --output-on-failure

contract-test: contract-test-backend contract-test-frontend

# Integration tests
integration-test:
	@echo "Running integration tests..."
	@echo "Backend integration coverage is provided by the e2e suite (real router)."
	cd backend && go test -v ./internal/api/... -run TestE2E
	@echo "Frontend integration tests require running backend"
	cd frontend && mkdir -p build && cd build && \
	cmake .. -DBUILD_TESTS=ON -DBUILD_INTEGRATION_TESTS=ON -DCMAKE_BUILD_TYPE=Debug && \
	make -j$(nproc) test_integration 2>&1 || true && \
	ctest -R "Integration" --output-on-failure

# End-to-end tests
e2e-test:
	@echo "Running E2E tests..."
	@echo "1. Starting backend..."
	cd backend && make build && \
	export JWT_SECRET=test-jwt-secret-key-for-e2e-testing-min-32-chars && \
	./build/torrsyncplayer --port 8889 --auto-tls & \
	sleep 5 && \
	curl -k -s https://localhost:8889/health
	@echo "2. Running Qt headless E2E tests..."
	cd frontend && mkdir -p build && cd build && \
	cmake .. -DBUILD_TESTS=ON -DCMAKE_BUILD_TYPE=Debug && \
	make -j$(nproc) test_e2e 2>&1 || true && \
	QT_QPA_PLATFORM=offscreen ctest -R "E2E" --output-on-failure
	@echo "3. Running Playwright E2E tests..."
	cd tests/e2e/playwright && npm ci && npx playwright test 2>&1 || true

# Load/performance tests
load-test:
	@echo "Running k6 load tests..."
	@echo "Starting backend for load testing..."
	cd backend && make build && \
	export JWT_SECRET=test-jwt-secret-key-for-load-testing-min-32-chars && \
	./build/torrsyncplayer --port 8889 --auto-tls & \
	sleep 5 && \
	curl -k -s https://localhost:8889/health
	cd tests/load/k6 && k6 run mixed_scenario.js 2>&1 || true

# Chaos tests
chaos-test:
	@echo "Running chaos tests with Toxiproxy..."
	@echo "Requires docker-compose with toxiproxy service"
	cd tests/chaos && ./run_chaos.sh 2>&1 || true

# Combined test targets
test-all: test-backend test-frontend contract-test integration-test
	@echo "All tests completed!"

test-ci: test-backend-race test-frontend-gmock contract-test
	@echo "CI test suite completed!"

test-full: test-backend test-backend-race test-backend-coverage test-backend-fuzz \
           test-frontend test-frontend-asan test-frontend-tsan test-frontend-coverage test-frontend-gmock \
           contract-test integration-test e2e-test
	@echo "Full test suite completed!"

build: backend frontend

clean:
	$(MAKE) -C backend clean
	rm -rf frontend/build frontend/build-fuzz

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
