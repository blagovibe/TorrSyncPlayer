#!/usr/bin/env bash
# A Go tool, installed with the same three attempts as the modules.
#
# Installing a tool is a network operation like any other: the checksum
# database's connection broke mid-tile once, and the mutation-testing job
# went red for a reason that said nothing about the code it was testing. The
# tool is pinned by version, so the retry installs the same thing — and the
# last error is the one that is reported.
#
#   .github/scripts/install-go-tool.sh github.com/owner/tool/cmd/tool@v1.2.3
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <package>@<version>" >&2
  exit 2
fi

for attempt in 1 2 3; do
  if go install "$1"; then
    exit 0
  fi
  if [ "$attempt" -eq 3 ]; then
    echo "go install $1 failed after $attempt attempts" >&2
    exit 1
  fi
  echo "go install $1 failed (attempt $attempt), trying again" >&2
  sleep 10
done
