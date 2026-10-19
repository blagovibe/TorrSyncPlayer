#!/usr/bin/env bash
# Records one meeting in the series and checks the series against its
# threshold.
#
# The series is a table in measurements/rendezvous.md, one row per run. The
# threshold: no fewer than 90% of the last 20 runs met within 90 seconds.
# Below it, the script writes a card on the board — the board is files in the
# repository, so a card is a file, and a number nobody chose.
#
#   rendezvous-series.sh record <log> <series-file>
#   rendezvous-series.sh check  <series-file> <repo-root>
#
# `record` appends a row and prints it. `check` prints the verdict and, when
# the threshold is broken, writes the card and prints its path.

set -euo pipefail

THRESHOLD_RUNS=20
THRESHOLD_RATE=90
THRESHOLD_SECONDS=90

record() {
  local log=$1 series=$2
  local outcome waited seconds row

  outcome=$(grep -E '^outcome:' "$log" | tail -1 | sed 's/^outcome:[[:space:]]*//' || true)
  waited=$(grep -E '^waited:' "$log" | tail -1 | sed 's/^waited:[[:space:]]*//' || true)
  seconds=$(grep -E '^waitedSeconds:' "$log" | tail -1 | sed 's/^waitedSeconds:[[:space:]]*//' || true)

  if [ "$outcome" = "the room met" ] && [ -n "$seconds" ] && [ "$seconds" -le "$THRESHOLD_SECONDS" ]; then
    row="met"
  else
    row="did not meet"
  fi

  local when
  when=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  printf '| %s | %s | %s | %s |\n' "$when" "$row" "${seconds:-?}" "${waited:-?}" >> "$series"
  printf '| %s | %s | %s | %s |\n' "$when" "$row" "${seconds:-?}" "${waited:-?}"
}

check() {
  local series=$1 root=$2
  local total met good

  total=$(grep -c '^| 20' "$series" || true)
  if [ "$total" -lt "$THRESHOLD_RUNS" ]; then
    echo "the series has $total run(s): the threshold needs $THRESHOLD_RUNS before it says anything"
    return 0
  fi

  local window
  window=$(tail -n "$THRESHOLD_RUNS" "$series")
  met=$(printf '%s\n' "$window" | grep -c '| met |' || true)
  # The seconds are compared as numbers: after the spaces are trimmed awk
  # holds a string, and "300" <= "90" is true in letters.
  good=$(printf '%s\n' "$window" | awk -F'|' '{gsub(/[ ]/, "", $4); if ($4 ~ /^[0-9]+$/ && ($4 + 0) <= '"$THRESHOLD_SECONDS"') n++} END {print n+0}')

  local rate
  rate=$((100 * good / total))
  echo "the last $total runs: $good met within ${THRESHOLD_SECONDS}s ($rate%)"

  if [ "$rate" -lt "$THRESHOLD_RATE" ]; then
    write_card "$root" "$total" "$good" "$rate"
    return 1
  fi
  return 0
}

# write_card puts a card on the board: the number is one more than the highest
# the board and the journal already use, so it is never taken twice.
write_card() {
  local root=$1 total=$2 good=$3 rate=$4
  local board="$root/.project-board"
  local number slug file

  number=$( {
    ls "$board" 2>/dev/null | grep -oE '^[a-z]+-[0-9]{4}' | grep -oE '[0-9]{4}' || true
    grep -oE '\b(idea|bug|deviation)-[0-9]{4}\b' "$root/CHANGELOG.md" 2>/dev/null | grep -oE '[0-9]{4}' || true
  } | sort -n | tail -1 )
  number=$(( ${number:-0} + 1 ))
  slug="the-room-gathered-$rate-percent-of-the-time"
  file="$board/bug-$(printf '%04d' "$number")-$slug.md"

  cat > "$file" <<EOF
---
id: bug-$(printf '%04d' "$number")
type: bug
status: open
title: Комната собиралась реже, чем обещает порог — $rate% последних $total замеров уложились в $THRESHOLD_SECONDS секунд
refs:
  - "CONCEPT.md#Синхронные комнаты — каждый качает из роя сам"
  - "DEV.md#Соглашения по тестам"
created: $(date -u +%Y-%m-%d)
---

## Суть

Ряд измерений публичной сети показал: из последних $total замеров комната
собралась за $THRESHOLD_SECONDS секунд или быстрее в $good случаях, это $rate%
при пороге в $THRESHOLD_RATE%. Порог записан в способе, и ниже него замер
обязан открывать карточку — вот она.

## Обсуждение

$(date -u +%Y-%m-%d): Создано автоматически измерением \`rendezvous.yml\`. Ряд:
\`measurements/rendezvous.md\`. Решение — за владельцем: это чистая сеть, или
это наш код, или и то и другое.
EOF

  echo "the threshold is broken: a card was written to ${file#$root/}"
}

case "${1:-}" in
  record) record "${2:?log}" "${3:?series}" ;;
  check) check "${2:?series}" "${3:?repo root}" ;;
  *) echo "usage: $0 record <log> <series> | $0 check <series> <repo root>" >&2; exit 2 ;;
esac
