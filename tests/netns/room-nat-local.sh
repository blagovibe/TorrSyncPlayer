#!/usr/bin/env bash
# The same scenario as room-nat.sh, on a machine that cannot be root.
#
# The scenario across the namespaces needs privileges: network namespaces,
# iptables, a masquerade. On a laptop that means sudo, and on a contributor's
# machine it usually means "not at all" — which is how the scenario stayed
# red for a long time while nobody could run it. The way out is a user
# namespace: inside one, the process has the capabilities the scenario needs
# over the namespaces it creates, and it needs nothing from the host.
#
#   tests/netns/room-nat-local.sh [test-pattern]
#
# The default pattern is the scenario itself. The run takes seconds, which is
# the point: a scenario that can only be tried in a twenty-five-minute CI
# cycle is a scenario nobody tries.
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
PATTERN=${1:-TestARoomAcrossNAT}
BINARY=$(mktemp -u /tmp/room-nat-local.XXXXXX)

cleanup() { rm -f "$BINARY"; }
trap cleanup EXIT

cd "$REPO_ROOT/frontend"
go test -c -o "$BINARY" ./internal/integration/

# The namespaces must be visible to every process in the run, and they are
# bind-mounted under /var/run/netns — so /var/run is a tmpfs of our own
# inside the mount namespace, and nothing outside can see or touch it.
exec unshare --user --map-root-user --net --mount --pid --fork --mount-proc bash -c '
  set -euo pipefail
  mount -t tmpfs none /var/run
  mkdir -p /var/run/netns
  cd "'"$REPO_ROOT"'"
  tests/netns/room-nat.sh up
  cd frontend
  FYNE_DRIVER=offscreen CGO_ENABLED=1 TORRSYNC_NETNS=1 \
    "'"$BINARY"'" -test.v -test.timeout 12m -test.run "'"$PATTERN"'"
'
