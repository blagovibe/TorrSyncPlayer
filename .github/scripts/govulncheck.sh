#!/usr/bin/env bash
# The vulnerability scan, with the same three attempts as the modules.
#
# The scan fetches the vulnerability database over the network, and a broken
# connection to it is the weather, not a finding. A finding and a failure to
# look are told apart by the exit code: 3 means vulnerabilities were found, 0
# means the scan is clean, and everything else means the scan did not finish.
# Only the finding may fail the job — the same rule the modules and the tool
# installs follow, for the same reason: a job that goes red for someone
# else's reasons teaches everyone to ignore red.
#
#   govulncheck.sh <patterns...>
set -euo pipefail

ATTEMPTS=3
# The wait between attempts. It is a variable so that the test of this
# script does not spend half a minute sleeping.
WAIT=${GOVULNCHECK_RETRY_WAIT:-10}

for attempt in $(seq 1 "$ATTEMPTS"); do
  set +e
  output=$(govulncheck "$@" 2>&1)
  code=$?
  set -e

  printf '%s\n' "$output"

  if [ "$code" -eq 0 ]; then
    exit 0
  fi
  if [ "$code" -eq 3 ]; then
    # A finding. The job goes red, and the reason is on the screen.
    echo "govulncheck found vulnerabilities"
    exit 3
  fi
  if [ "$attempt" -eq "$ATTEMPTS" ]; then
    echo "govulncheck did not finish after $ATTEMPTS attempts (exit $code)" >&2
    exit "$code"
  fi
  echo "govulncheck did not finish (exit $code), trying again" >&2
  sleep "$WAIT"
done
