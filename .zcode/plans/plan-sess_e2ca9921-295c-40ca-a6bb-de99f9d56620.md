# План исправления CI и CodeQL (без деградации)

## Проблема 1: CI Workflow - golangci-lint failures

### 1.1. router.go:62 - gofmt formatting
**Файл:** `backend/internal/api/router.go:62`
**Проблема:** Неправильные отступы (смешанная табуляция/пробелы)
**Решение:** Выполнить `gofmt -w backend/internal/api/router.go`
**Риск:** Нулевой - только форматирование

### 1.2. persistence_test.go:39,47,167 - gosec G101 false positives
**Файл:** `backend/internal/persistence/persistence_test.go`
**Проблема:** Строки с bcrypt-подобными хешами вызывают G101 (hardcoded credentials)
**Решение:** Добавить `#nosec G101` директивы к строкам с тестовыми хешами:
```go
PasswordHash: "$2a$12$hashhashhashhashhashhashhashhashhashhash", // #nosec G101
```
**Риск:** Нулевой - подавление только в тестах

### 1.3. store_test.go:268 - staticcheck QF1003
**Файл:** `backend/internal/auth/store_test.go:265-277`
**Проблема:** Рекомендует использовать tagged switch на `tt.name`
**Решение:** Рефакторинг if-else в switch statement (см. ниже)
**Риск:** Нулевой - тот же самый код

### 1.4. validation.go:27 - unused variable
**Файл:** `backend/internal/validation/validation.go:27`
**Проблема:** `magnetParamsRegex` объявлен, но не используется
**Решение:** Использовать regex для валидации параметров вместо ручной проверки, либо удалить
**Риск:** Низкий - текущая валидация работает, просто неэффективно

## Проблема 2: CodeQL Workflow - C++ linker errors

### 2.1. MpvWidget OpenGL methods without libmpv
**Файлы:** `frontend/src/mpvwidget.h`, `frontend/src/mpvwidget.cpp`
**Проблема:** Virtual методы `initializeGL()`, `resizeGL()`, `paintGL()` объявлены как override, но не компилируются без libmpv
**Решение:** Добавить заглушки вне `#ifdef HAS_MPV_RENDER`:
```cpp
// В .h файле - сделать методы virtual без условной компиляции
// В .cpp файле - добавить пустые реализации в #else ветку
```
**Риск:** Нулевой - пустые реализации для headless сборки

## Проблема 3: Security Workflow - actions/cache@v7

### 3.1. security.yml action version
**Файл:** `.github/workflows/security.yml:37`
**Проблема:** `actions/cache@v7` не существует
**Решение:** Заменить на `actions/cache@v4`
**Риск:** Нулевой - обновление к существующей версии

## Приоритет исправления:
1. **HIGH:** security.yml action version (блокирует всю security workflow)
2. **HIGH:** persistence_test.go G101 (блокирует lint-backend job)
3. **HIGH:** MpvWidget stubs (блокирует CodeQL C++ анализ)
4. **HIGH:** router.go gofmt (блокирует lint-backend job)
5. **MEDIUM:** store_test.go QF1003 (качество кода)
6. **MEDIUM:** validation.go unused var (чистота кода)