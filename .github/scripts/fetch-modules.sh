#!/usr/bin/env bash
# The modules a job needs, fetched before anything is built.
#
# The runner's module cache is warm for the versions the repository has, and
# cold for the ones a dependency bump has just brought. A download that
# breaks halfway is the network's business, not the code's — but it must not
# be able to fail a job that says nothing about the code: one broken stream
# from the module proxy cost the scenario across the namespaces a red run,
# and the code it was testing had not changed.
#
# Three attempts, and the last error is the one that is reported.
#
#   .github/scripts/fetch-modules.sh backend frontend
set -euo pipefail

for module_dir in "$@"; do
  for attempt in 1 2 3; do
    if (cd "$module_dir" && go mod download all); then
      break
    fi
    if [ "$attempt" -eq 3 ]; then
      echo "go mod download failed in $module_dir after $attempt attempts" >&2
      exit 1
    fi
    echo "go mod download failed in $module_dir (attempt $attempt), trying again" >&2
    sleep 10
  done
done
