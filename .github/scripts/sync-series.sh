#!/usr/bin/env bash
# Records one measurement of the room's divergence in the series and checks
# the series against its threshold.
#
# The series is a table in measurements/sync.md, one row per run. The
# threshold: no fewer than 90% of the last 20 runs kept the divergence within
# the promise — one second, the goal's own number. Below it, the script writes
# a card on the board — the board is files in the repository, so a card is a
# file, and a number nobody chose.
#
# The promise's number is read from the measurement itself
# (frontend/internal/ui/sync_measurement_test.go), so the series and the gate
# cannot say different things: a check that answers a question nobody asked
# is worse than no check.
#
#   sync-series.sh record <log> <series-file> <measurement-source>
#   sync-series.sh check  <series-file> <repo-root> <measurement-source>
set -euo pipefail

THRESHOLD_RUNS=20
THRESHOLD_RATE=90

# promise reads the limit out of the measurement's source: the divergence the
# goal allows, in seconds.
promise() {
  local source=$1
  sed -n 's/^const steadyDivergenceLimit = \([0-9.]*\)$/\1/p' "$source" | head -1
}

record() {
  local log=$1 series=$2 source=$3
  local limit divergence landing outcome row

  limit=$(promise "$source")
  if [ -z "$limit" ]; then
    echo "sync-series: the promise's number is not in $source" >&2
    exit 1
  fi

  # `go test -v` prefixes every line of the test's output with "file:line: ",
  # so the fields are looked for anywhere in the line, not only at its start.
  field() {
    grep -E "$1:" "$log" | tail -1 | sed "s/.*$1:[[:space:]]*//" || true
  }
  divergence=$(field steadyDivergence)
  landing=$(field landingSeconds)
  outcome=$(field outcome)

  if [ -n "$divergence" ] && [ -n "$landing" ] && \
     awk -v d="$divergence" -v l="$landing" -v m="$limit" 'BEGIN { exit !(d + 0 <= m + 0 && l + 0 <= m + 0) }'; then
    row="within the promise"
  else
    row="outside the promise"
  fi

  local when
  when=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  printf '| %s | %s | %s | %s |\n' "$when" "${divergence:-?}" "${landing:-?}" "$row" >> "$series"
  printf '| %s | %s | %s | %s |\n' "$when" "${divergence:-?}" "${landing:-?}" "$row"
}

check() {
  local series=$1 root=$2 source=$3
  local limit total good rate

  limit=$(promise "$source")
  if [ -z "$limit" ]; then
    echo "sync-series: the promise's number is not in $source" >&2
    exit 1
  fi

  total=$(grep -c '^| 20' "$series" || true)
  if [ "$total" -lt "$THRESHOLD_RUNS" ]; then
    echo "the series has $total run(s): the threshold needs $THRESHOLD_RUNS before it says anything"
    return 0
  fi

  local window
  window=$(tail -n "$THRESHOLD_RUNS" "$series")
  good=$(printf '%s\n' "$window" | grep -c '| within the promise |' || true)

  local rate
  rate=$((100 * good / total))
  echo "the last $total runs: $good kept the divergence within ${limit}s ($rate%)"

  if [ "$rate" -lt "$THRESHOLD_RATE" ]; then
    write_card "$root" "$total" "$good" "$rate" "$limit"
    return 1
  fi
  return 0
}

# write_card puts a card on the board: the number is one more than the highest
# the board and the journal already use, so it is never taken twice.
write_card() {
  local root=$1 total=$2 good=$3 rate=$4 limit=$5
  local board="$root/.project-board"
  local number slug file

  mkdir -p "$board"
  number=$( {
    ls "$board" 2>/dev/null | grep -oE '^[a-z]+-[0-9]{4}' | grep -oE '[0-9]{4}' || true
    grep -oE '\b(idea|bug|deviation)-[0-9]{4}\b' "$root/CHANGELOG.md" 2>/dev/null | grep -oE '[0-9]{4}' || true
  } | sort -n | tail -1 )
  # 10# says the number is decimal: without it bash reads a zero-padded
  # number as octal, and 0146 becomes 102 — a card numbered from a number
  # nobody used.
  number=$(( 10#${number:-0} + 1 ))
  slug="the-room-drifted-apart"
  file="$board/bug-$(printf '%04d' "$number")-$slug.md"

  cat > "$file" <<EOF
---
id: bug-$(printf '%04d' "$number")
type: bug
status: open
title: Комната расходилась дальше обещания — $good из $total последних замеров держали секунду
refs:
  - "CONCEPT.md#Синхронные комнаты — каждый качает из роя сам"
  - "DEV.md#Соглашения по тестам"
created: $(date -u +%Y-%m-%d)
---

## Суть

Ряд измерений расхождения синхронизации показал: из последних $total замеров
комната держала расхождение в пределах $limit секунды в $good случаях, это
$rate% при пороге в $THRESHOLD_RATE%. Предел записан в цели, и ниже него
замер обязан открывать карточку — вот она.

## Обсуждение

$(date -u +%Y-%m-%d): Создано автоматически измерением \`sync-measurement\`. Ряд:
\`measurements/sync.md\`. Решение — за владельцем: это наш код, или это
окружение, на котором он бежит.
EOF

  echo "the threshold is broken: a card was written to ${file#$root/}"
}

case "${1:-}" in
  record) record "${2:?log}" "${3:?series}" "${4:?measurement source}" ;;
  check) check "${2:?series}" "${3:?repo root}" "${4:?measurement source}" ;;
  *) echo "usage: $0 record <log> <series> <source> | $0 check <series> <repo root> <source>" >&2; exit 2 ;;
esac
