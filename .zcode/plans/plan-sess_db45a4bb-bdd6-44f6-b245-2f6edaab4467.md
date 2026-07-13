# План исправления CI для PR #27

## Проблемы:
1. **Sanitizer Tests** - ASan и TSan конфликтуют (нельзя комбинировать в одном бинаре)
2. **End-to-End Tests** - `npm ci` требует `package-lock.json` которого нет
3. **Mutation Testing** - таймаут 1 час недостаточен

## Решения:

### 1. frontend/CMakeLists.txt (sanitizer configuration)
Изменить секцию `# ── Sanitizers ──────────────────────────────────────────────────────────`:
- Убрать `-fsanitize=address -fsanitize=thread` из глобальных флагов
- Оставить только `-fsanitize=undefined` как базовые флаги
- ASan/TSan будут применяться только к отдельным целям тестов

### 2. .github/workflows/ci.yml (E2E npm install)
Изменить:
```yaml
- name: Install Qt6 and Playwright dependencies
  run: npm install --prefix tests/e2e/playwright
```

### 3. .github/workflows/ci.yml (Mutation testing timeout)
```yaml
timeout-minutes: 120  # Увеличить с 60
```

## Результат:
- Все критические проверки (Frontend Build, Test Backend, Lint Backend) уже проходят
- После исправлений PR можно будет слить в main без деградации качества