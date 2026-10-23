---
id: bug-0156
type: bug
status: open
title: `make test-fuzz` не может покраснеть — цель подхватывает четыре фаззера и глушит код возврата
refs:
  - "DEV.md#Соглашения по тестам"
  - "DEV.md#Проверки должны уметь краснеть"
created: 2026-10-23
---

## Суть

В `backend/Makefile:100–102` цель `test-fuzz` запускает
`go test -fuzz=Fuzz -fuzztime=30s ./internal/validation/...` и завершает
конвейер `| head -100`. В `backend/internal/validation/fuzz_test.go` четыре
фаззера (`FuzzValidateMagnetURI`, `FuzzValidateUsername`, `FuzzValidatePassword`,
`FuzzValidateFileIndex`), а `-fuzz` принимает ровно один шаблон: `go test`
отказывается словами «matches more than one», `head` получает пустой/короткий
вывод и возвращает ноль. Цель зелёная всегда.

Это прямо против правила способа «проверки должны уметь краснеть»: фаззинг не
проверяет ничего, а CI-джоба видит успех.

## Обсуждение

2026-10-23: Найдено аудитом. Проверено чтением `backend/Makefile` и списка
`func Fuzz` в `fuzz_test.go`. Лечение: гонять каждый фаззер отдельной строкой
(`-fuzz=^FuzzX$`) или через `-fuzztime` по одному, без `| head`, чтобы код
возврата доезжал; тест, что цель падает при сломанном фаззере.
