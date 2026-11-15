---
id: bug-0176
type: bug
status: open
title: backend тянет go.opentelemetry.io/otel v1.45.0 — открытый dependabot alert (medium)
refs:
  - "DEV.md#Ворота репозитория"
created: 2026-11-15
---

## Суть

Открытый dependabot alert (state=open, 2026-11-15): `go.opentelemetry.io/otel/trace`
— medium severity, «AttributeValueLengthLimit is fully bypassed by attribute values
composed of valid U+FFFD runes»; исправлено в **1.47.0**. Бэкенд пинил
`go.opentelemetry.io/otel{,/metric,/trace} v1.45.0` (`// indirect`); фронт уже на
v1.47.0.

## Обсуждение

2026-11-15: Найдено свежей сверкой (`gh api .../dependabot/alerts?state=open` →
1 alert; code-scanning → 0). Правка: `backend/go.mod` поднят до v1.47.0 всей
семьёй (`otel`, `otel/log`, `otel/metric`, `otel/trace`), `go mod tidy`,
`go build ./...` зелёный локально. Карточка закрывается после зелёного CI на
линии (security-scan/govulncheck на той же линии) — и затем dependabot alert
обязан закрыться сам (исправление на main).
