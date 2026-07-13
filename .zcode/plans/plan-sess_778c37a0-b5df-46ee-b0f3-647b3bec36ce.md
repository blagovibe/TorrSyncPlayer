# Полноценная тестовая инфраструктура для TorrSyncPlayer (Enterprise Grade)

## Цель
Полное покрытие всех слоев: unit, integration, contract, fuzz, mutation, performance, chaos — с автоматизацией в CI/CD.

---

## 1. Backend (Go) — расширение существующего

### 1.1 Unit тесты (уже есть, усилить)
- `go test -race -cover -count=1 ./...` — уже в CI
- **Добавить**: Fuzzing парсеров (`go test -fuzz=FuzzParseTorrent -fuzztime=30s ./internal/torrent/...`)
- **Добавить**: Property-based тесты через `github.com/leanovate/gopter`

### 1.2 Integration тесты (NEW)
```
backend/internal/integration/
├── api_integration_test.go      # Полный HTTP API сценарии
├── p2p_integration_test.go      # P2P room: create, join, signal, sync
├── streaming_integration_test.go # HTTP range requests, mpv buffer
└── testutil/
    ├── testserver.go            # In-memory httptest.Server
    ├── testdb.go                # In-memory хранилище
    └── fixtures/                # .torrent файлы, magnet ссылки
```

### 1.3 Contract Testing (NEW) — **Ключевое для C++ ↔ Go**
- Pact (github.com/pact-foundation/pact-go/v2)
- Контракт коммитится в репозиторий → frontend тесты его валидируют

### 1.4 Mutation Testing (NEW)
- `go-mutesting` — фейлит CI, если mutation score < 80%

---

## 2. Frontend (C++/Qt) — полная переработка

### 2.1 Mock Framework — **Google Mock (gmock)**
- Вынести интерфейсы в `src/interfaces/` (INetworkManager, ITorrentManager, ISyncManager, IMediaPlayer)
- Создать `src/mocks/` с моками
- Переписать unit тесты с использованием моков

### 2.2 Sanitizers в CI
- AddressSanitizer + ThreadSanitizer + UBSan (`-fsanitize=address,thread,undefined`)

### 2.3 Coverage в CI (lcov + Codecov)
- `lcov --capture --directory build --output-file coverage.info`

### 2.4 clang-tidy + clang-format в CI
- Фейлить CI на warnings

---

## 3. End-to-End / Integration тесты (C++ + Go вместе)

### 3.1 Playwright + Qt Headless
```
tests/e2e/
├── playwright/                  # TypeScript тесты критических путей
│   ├── auth.spec.ts
│   ├── torrent_add.spec.ts
│   ├── p2p_room.spec.ts
│   └── streaming.spec.ts
└── qt_headless/                 # C++ Qt WebEngine headless
    ├── test_full_flow.cpp       # Real frontend + real backend
    └── test_p2p_two_clients.cpp # Two QProcess frontends + one backend
```

### 3.2 CI Pipeline для E2E
- Поднимает backend сервис + запускает Qt headless + Playwright тесты

---

## 4. Performance / Load / Chaos Testing (NEW)

### 4.1 k6 Load Tests
- `tests/load/k6/` — torrent add, P2P sync, streaming, mixed scenarios

### 4.2 Chaos Testing (Toxiproxy)
- Network partition, latency injection, packet loss, peer crash

---

## 5. CI/CD Pipeline — новые Jobs
- `contract-test-backend` / `contract-test-frontend` (Pact)
- `mutation-test-backend` / `mutation-test-frontend` (go-mutesting / Mull)
- `fuzz-test-backend` / `fuzz-test-frontend` (go-fuzz / libFuzzer)
- `e2e-tests` (Playwright + Qt headless)
- `load-tests` (k6, scheduled + main branch)
- `chaos-tests` (Toxiproxy, scheduled)

### Quality Gates (Required checks для PR)
- [ ] lint-backend, test-backend (race + cover > 85%)
- [ ] frontend (build + ctest + clang-tidy clean)
- [ ] contract-test-backend + contract-test-frontend
- [ ] mutation-test-backend (score > 80%) + mutation-test-frontend (score > 70%)
- [ ] e2e-tests

---

## 6. Локальная разработка — Makefile цели
```makefile
test-backend-fuzz:      # go test -fuzz...
test-backend-mutation:  # go-mutesting ./...
contract-test-backend:  # go test -run PactProvider...
test-frontend-asan:     # cmake -DENABLE_SANITIZERS=ON && ctest
test-frontend-mutation: # mull-cxx...
fuzz-frontend:          # ./build/tests/fuzz/fuzz_* -max_total_time=60
test-e2e:               # docker-compose -f tests/e2e/docker-compose.yml up
test-all: test-backend test-frontend test-e2e contract-test-backend contract-test-frontend
```

---

## 7. План реализации (5 этапов, ~15 дней)

| Этап | Дни | Делiverables |
|------|-----|--------------|
| 1. Foundation | 1-3 | Frontend coverage + clang-tidy + ASAN/TSAN, gmock, моки, переписанные unit тесты |
| 2. Contract Testing | 4-6 | Pact в backend, pact файл, consumer tests во frontend, CI jobs |
| 3. Integration + E2E | 7-9 | Backend integration тесты, frontend integration, Playwright + Qt headless E2E |
| 4. Advanced Testing | 10-12 | Go fuzzing, libFuzzer, mutation testing, quality gates |
| 5. Performance + Chaos | 13-15 | k6 load тесты, Toxiproxy chaos, scheduled nightly runs |

---

## Итог
После реализации:
- **Любой баг в API контракте** → ловится на этапе PR (Pact)
- **Любой баг в парсерах/обработчиках** → ловится фаззингом
- **Качество тестов** → измеряется мутационным тестированием
- **Реальные пользовательские сценарии** → проверяются E2E тестами
- **Производительность под нагрузкой** → мониторится k6
- **Устойчивость к сбоям сети** → проверяется chaos тестами

Готов начать реализацию с Этапа 1.