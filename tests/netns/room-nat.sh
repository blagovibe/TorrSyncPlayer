#!/usr/bin/env bash
# Two participants, two network namespaces, one NAT between them.
#
# This is the room behind a NAT — the case the public DHT is supposed to carry
# and that no loopback test can pose. The host's namespace and the guest's
# namespace sit on different subnets, both reach the internet through
# masquerading, and forwarding between the two subnets is dropped: neither
# participant can dial the other directly. What is left is the path the ADR
# promises — hole punching, and the relay when punching fails — and the room
# has to work over it.
#
# Usage:
#   sudo tests/netns/room-nat.sh up      # create, print nothing, exit non-zero on failure
#   sudo tests/netns/room-nat.sh down    # remove everything
#   sudo tests/netns/room-nat.sh status  # addresses, for a human
#
# On a machine that cannot be root, tests/netns/room-nat-local.sh runs the
# whole scenario in a user namespace: the same namespaces, the same rules,
# the same test, and it takes seconds instead of a CI cycle.
#
# The script fails loudly. A scenario that quietly set up nothing and passed is
# the exact failure this repository keeps having to undo.

set -euo pipefail

HOST_NS=${HOST_NS:-ts-host}
GUEST_NS=${GUEST_NS:-ts-guest}
HOST_VETH=${HOST_VETH:-tsh0}
GUEST_VETH=${GUEST_VETH:-tsg0}
HOST_CIDR=10.77.1
GUEST_CIDR=10.77.2

need_root() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "room-nat: must run as root (it creates network namespaces)" >&2
    exit 1
  fi
}

require_tools() {
  # ping is not decoration: the isolation check below proves the two sides
  # cannot reach each other, and a check that passes because the tool is
  # missing proves nothing at all.
  for tool in ip iptables ping; do
    command -v "$tool" >/dev/null 2>&1 || {
      echo "room-nat: $tool is required and not installed" >&2
      exit 1
    }
  done
}

down() {
  ip netns del "$HOST_NS" 2>/dev/null || true
  ip netns del "$GUEST_NS" 2>/dev/null || true
  ip link del "$HOST_VETH" 2>/dev/null || true
  ip link del "$GUEST_VETH" 2>/dev/null || true
  # Masquerade and forward rules go with the interfaces, but the explicit
  # forward drop is removed anyway: a leftover rule would silently shape the
  # next run on this machine.
  iptables -D FORWARD -s "$HOST_CIDR.0/24" -d "$GUEST_CIDR.0/24" -j DROP 2>/dev/null || true
  iptables -D FORWARD -s "$GUEST_CIDR.0/24" -d "$HOST_CIDR.0/24" -j DROP 2>/dev/null || true
  iptables -t nat -D POSTROUTING -s "$HOST_CIDR.0/24" -j MASQUERADE 2>/dev/null || true
  iptables -t nat -D POSTROUTING -s "$GUEST_CIDR.0/24" -j MASQUERADE 2>/dev/null || true
  echo "room-nat: torn down"
}

up() {
  down >/dev/null 2>&1 || true

  ip netns add "$HOST_NS"
  ip netns add "$GUEST_NS"

  # Each namespace gets one veth with the host side left in the host namespace.
  ip link add "$HOST_VETH" type veth peer name eth0 netns "$HOST_NS"
  ip link add "$GUEST_VETH" type veth peer name eth0 netns "$GUEST_NS"

  ip addr add "$HOST_CIDR.1/24" dev "$HOST_VETH"
  ip link set "$HOST_VETH" up

  ip -n "$HOST_NS" addr add "$HOST_CIDR.2/24" dev eth0
  ip -n "$HOST_NS" link set eth0 up
  ip -n "$HOST_NS" link set lo up

  ip addr add "$GUEST_CIDR.1/24" dev "$GUEST_VETH"
  ip link set "$GUEST_VETH" up

  ip -n "$GUEST_NS" addr add "$GUEST_CIDR.2/24" dev eth0
  ip -n "$GUEST_NS" link set eth0 up
  ip -n "$GUEST_NS" link set lo up

  # Both reach the internet the way a real behind-the-NAT machine does: one
  # way out, nothing in.
  sysctl -qw net.ipv4.ip_forward=1
  iptables -t nat -A POSTROUTING -s "$HOST_CIDR.0/24" -j MASQUERADE
  iptables -t nat -A POSTROUTING -s "$GUEST_CIDR.0/24" -j MASQUERADE

  # And they cannot reach each other. This single rule is what makes the
  # scenario the thing it claims to be: without it the two participants share
  # a host and connect directly, and "works behind NAT" is a claim about
  # nothing.
  iptables -A FORWARD -s "$HOST_CIDR.0/24" -d "$GUEST_CIDR.0/24" -j DROP
  iptables -A FORWARD -s "$GUEST_CIDR.0/24" -d "$HOST_CIDR.0/24" -j DROP

  # Verify the claim instead of assuming it — both ways. A direct path between
  # the participants would make this a test of nothing; a dead bridge would
  # make it a test that hangs for its full timeout and then fails for a
  # reason nobody wrote down.
  if ip netns exec "$GUEST_NS" ping -c1 -W2 "$HOST_CIDR.2" >/dev/null 2>&1; then
    echo "room-nat: the namespaces can reach each other directly — the scenario is not a NAT" >&2
    exit 1
  fi
  if ! ip netns exec "$GUEST_NS" ping -c1 -W2 "$GUEST_CIDR.1" >/dev/null 2>&1; then
    echo "room-nat: the guest namespace cannot reach its own gateway — the bridge is down" >&2
    exit 1
  fi

  echo "room-nat: up (host ${HOST_NS} at ${HOST_CIDR}.2, guest ${GUEST_NS} at ${GUEST_CIDR}.2, no path between them)"
}

case "${1:-}" in
  up) need_root; require_tools; up ;;
  down) need_root; down ;;
  status)
    ip netns list
    ip -n "$HOST_NS" -o -4 addr show 2>/dev/null || true
    ip -n "$GUEST_NS" -o -4 addr show 2>/dev/null || true
    ;;
  *)
    echo "usage: $0 {up|down|status}" >&2
    exit 2
    ;;
esac