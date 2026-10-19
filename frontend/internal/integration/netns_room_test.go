// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blagovibe/TorrSyncPlayer/frontend/internal/rtc"
)

// The room behind a NAT.
//
// Every other scenario in this package puts its participants on one host, where
// they connect directly no matter what the room does. That is a different
// world from the one the ADR bought: two machines, each behind its own NAT,
// neither able to dial the other. This file poses that world, and the only way
// to pose it is with real network namespaces — so the setup lives in
// tests/netns/room-nat.sh and this test drives the participants inside them.
//
// What it proves, and what it does not:
//
//   - the room converges when the two participants cannot reach each other, and
//     the connection that forms is relayed (`ConnStatus` says so, byte by byte);
//   - the magnet crosses that relayed path;
//   - and nothing about the public DHT is assumed — the rendezvous is the same
//     local testbed every other scenario uses.
//
// It does not prove that a real router, a real ISP or a real firewall behaves
// like these namespaces. It proves the code does not quietly depend on being
// able to dial directly — which is the part that has never been checked and
// cannot be checked without exactly this.
//
// The test skips unless the namespaces were set up on purpose:
//
//	sudo tests/netns/room-nat.sh up
//	TORRSYNC_NETNS=1 FYNE_DRIVER=offscreen go test -run TestARoomAcrossNAT ./internal/integration/
//	sudo tests/netns/room-nat.sh down
//
// The skip is for the local run, not for CI: the workflow sets the namespaces
// up first and fails if it cannot, so a broken setup is a red job rather than
// a quietly skipped scenario.

const (
	natEnvSetup  = "TORRSYNC_NETNS"
	natEnvRole   = "TORRSYNC_PARTICIPANT"
	natEnvInvite = "TORRSYNC_NAT_INVITE"
	natEnvBoot   = "TORRSYNC_NAT_BOOTSTRAP"
	natEnvRelay  = "TORRSYNC_NAT_RELAY"
	// The relayed assertion is this scenario's own claim. The coordinator
	// switches it on; everywhere else it stays off, so the helper can be
	// exercised on a single host — where peers do connect directly —
	// without that path pretending to be a NAT it is not.
	natEnvExpectRelay = "TORRSYNC_NAT_EXPECT_RELAY"
	natHostNS         = "ts-host"
	natGuestNS        = "ts-guest"
	natHostBridge     = "10.77.1.1"
	natGuestBridge    = "10.77.2.1"
)

// TestARoomAcrossNAT runs one participant in each namespace and asks both to
// meet. It is a coordinator: everything asserted about the room happens inside
// the participants, because a room that only works in one process is not the
// claim.
func TestARoomAcrossNAT(t *testing.T) {
	if os.Getenv(natEnvSetup) != "1" {
		t.Skipf("network namespaces are not set up; run `sudo tests/netns/room-nat.sh up` and set %s=1", natEnvSetup)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	relayInfo := startNATRelay(t)

	// Both at once, in their own namespaces, over the relay and with no direct
	// path. Retried with a fresh testbed per attempt: rendezvous on a real
	// network is weather, and the project's own mesh tests wait it out the same
	// way. The relay is created once — it is infrastructure, not state.
	for attempt := 1; attempt <= 3; attempt++ {
		hostOut, guestOut, err := runNATAttempt(ctx, t,
			natParticipant{namespace: natHostNS, bridge: natHostBridge, relay: natRelayFor(natHostBridge, relayInfo), expectRelay: "1"},
			natParticipant{namespace: natGuestNS, bridge: natGuestBridge, relay: natRelayFor(natGuestBridge, relayInfo), expectRelay: "1"},
		)
		if err == nil {
			assert.Contains(t, hostOut, "OK", "the participant behind the first namespace did not finish:\n%s", hostOut)
			assert.Contains(t, guestOut, "OK", "the participant behind the second namespace did not finish:\n%s", guestOut)
			return
		}
		t.Logf("attempt %d of 3 did not meet: %v", attempt, err)
	}
	t.Fatal("the two participants never met across the NAT in 3 attempts")
}

// runNATAttempt is one rendezvous: a fresh testbed, two participants started
// together, and what came of it. Both start at once because a room is two
// people present together — a host that has already given up waiting is not a
// room anybody can walk into.
func runNATAttempt(ctx context.Context, t *testing.T, host, guest natParticipant) (string, string, error) {
	t.Helper()

	inv := rtc.GenerateInvite()
	bedID, bedPort := startNATTestbed(t, inv)
	host.bootstrap = p2pAddr(host.bridge, bedPort, bedID)
	guest.bootstrap = p2pAddr(guest.bridge, bedPort, bedID)
	host.invite, guest.invite = inv, inv

	hostOut, hostErr := startParticipant(ctx, host.namespace, host)
	guestOut, guestErr := startParticipant(ctx, guest.namespace, guest)
	hostBody, guestBody := <-hostOut, <-guestOut
	hostDone, guestDone := <-hostErr, <-guestErr

	if hostDone != nil || guestDone != nil {
		return hostBody, guestBody, fmt.Errorf("a participant failed:\n%s\n%s", hostBody, guestBody)
	}
	if !strings.Contains(hostBody, "OK") || !strings.Contains(guestBody, "OK") {
		return hostBody, guestBody, fmt.Errorf("a participant did not finish:\n%s\n%s", hostBody, guestBody)
	}
	return hostBody, guestBody, nil
}

// natRelayFor is the relay's address as one namespace sees it.
func natRelayFor(bridge string, r natRelayInfo) string {
	return p2pAddr(bridge, r.port, r.id)
}

// TestNATParticipant is the helper TestARoomAcrossNAT runs inside a namespace,
// once per side. It is a test like any other and fails the way one fails: by
// not reaching the end.
func TestNATParticipant(t *testing.T) {
	role := os.Getenv(natEnvRole)
	if role == "" {
		t.Skip("not a participant; set " + natEnvRole)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	parts := strings.Split(os.Getenv(natEnvInvite), ":")
	require.Len(t, parts, 3, "the invite must be community:password:key")
	inv := rtc.Invite{Community: parts[0], Password: parts[1], Key: parts[2]}

	mesh := rtc.NewMesh(rtc.MeshOptions{
		Bootstrap: os.Getenv(natEnvBoot),
		Relays:    os.Getenv(natEnvRelay),
		Community: inv.Community,
		Password:  inv.Password,
		Key:       inv.Key,
		UserID:    role,
	})
	defer func() { _ = mesh.Close() }()
	require.NoError(t, mesh.Join(ctx, role == "host"), "the participant could not join over NAT")

	var gotMagnet chan rtc.Magnet
	if role == "guest" {
		gotMagnet = make(chan rtc.Magnet, 1)
	}
	room := rtc.NewRoom(mesh, mesh.SelfID(), role, role == "host", &rtc.Callbacks{
		OnMagnet: func(m rtc.Magnet) {
			select {
			case gotMagnet <- m:
			default:
			}
		},
	})
	go room.Run(ctx)

	// The room has to meet with no direct path open, and the window is sized
	// from what the rendezvous actually costs rather than from what looks
	// reasonable: against the public DHT a room takes about two minutes to
	// converge, because the wide publish that makes it findable is itself
	// measured in minutes. A test that gives up sooner would be measuring its
	// own impatience — and the product's, which is the thing under test.
	if !waitForMeshWithin(t, mesh, 1, 3*time.Minute) {
		t.Fatalf("the room never met with no direct path open (status %+v)", mesh.ConnStatus())
	}

	st := mesh.ConnStatus()
	require.Len(t, st.Peers, 1, "the participant must see exactly one peer")
	if os.Getenv(natEnvExpectRelay) == "1" {
		assert.True(t, st.Peers[0].Relayed,
			"the peer connected directly: the namespaces are not actually separated, so this proves nothing")
	}

	if role == "host" {
		require.NoError(t, room.AnnounceMagnet(ctx, rtc.Magnet{
			MagnetURI:    "magnet:?xt=urn:btih:nat-scenario",
			InfoHash:     "nat-scenario",
			SelectedFile: 0,
			Name:         "behind-nat.mkv",
		}), "the host could not announce its film across the relay")
	}

	if role == "guest" {
		select {
		case m := <-gotMagnet:
			assert.Equal(t, "nat-scenario", m.InfoHash, "the guest learned the wrong film")
		case <-ctx.Done():
			t.Fatal("the room's magnet never crossed the relayed path")
		}
	}

	fmt.Printf("participant %s: OK\n", role)
}

// natParticipant is one side's parameters, passed to the helper process.
type natParticipant struct {
	role        string
	invite      rtc.Invite
	bootstrap   string
	relay       string
	expectRelay string
	// namespace is where this participant runs: a network namespace name, or
	// empty for this machine.
	namespace string
	// bridge is the address the host side of that namespace is reachable at.
	bridge string
}

// startParticipant launches one participant and hands back its outcome.
// Re-executing the binary is what puts a real second participant in a real
// second network namespace: there is no in-process way to be somewhere else.
func startParticipant(ctx context.Context, namespace string, p natParticipant) (<-chan string, <-chan error) {
	out := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		body, err := runParticipant(ctx, namespace, p)
		out <- body
		done <- err
	}()
	return out, done
}

// waitForParticipant reads one participant's outcome and fails on it, naming
// what went wrong rather than only that something did.
func waitForParticipant(t *testing.T, out <-chan string, done <-chan error, what string) {
	t.Helper()
	body := <-out
	if err := <-done; err != nil {
		t.Fatalf("%s failed:\n%s", what, body)
	}
	if !strings.Contains(body, "OK") {
		t.Fatalf("%s did not report success:\n%s", what, body)
	}
}

// runParticipant runs this test binary inside a namespace and returns its
// output.
func runParticipant(ctx context.Context, namespace string, p natParticipant) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("the test binary must be executable: %w", err)
	}

	// "self" means no namespace at all: the coordinator names one, and a local
	// smoke run of the helper itself names this.
	args := []string{"-test.run", "^TestNATParticipant$", "-test.v", "-test.timeout", "7m"}
	var cmd *exec.Cmd
	if namespace != "" && namespace != "self" {
		cmd = exec.CommandContext(ctx, "ip", append([]string{"netns", "exec", namespace, exe}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, exe, args...)
	}
	cmd.Env = append(os.Environ(),
		natEnvRole+"="+p.role,
		natEnvInvite+"="+strings.Join([]string{p.invite.Community, p.invite.Password, p.invite.Key}, ":"),
		natEnvBoot+"="+p.bootstrap,
		natEnvRelay+"="+p.relay,
		natEnvExpectRelay+"="+p.expectRelay,
		// Fyne would otherwise look for a display in a namespace that has
		// none; the participant draws nothing, but the toolkit is linked in.
		"FYNE_DRIVER=offscreen",
	)

	out, err := cmd.CombinedOutput()
	return string(out), err
}

// natRelay is a relay the participants can reach, and the port it listens on
// in each namespace's own subnet.
type natRelayInfo struct {
	id   peer.ID
	port int
}

// startNATRelay runs a circuit relay in the host namespace, listening on both
// bridges so either namespace can reach it. It forwards bytes and knows
// nothing about rooms, which is the whole of its job.
func startNATRelay(t *testing.T) natRelayInfo {
	t.Helper()

	h, err := libp2p.New(
		libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip4/%s/tcp/0", natHostBridge),
			fmt.Sprintf("/ip4/%s/tcp/0", natGuestBridge),
		),
	)
	require.NoError(t, err, "the relay must start")
	t.Cleanup(func() { _ = h.Close() })
	_, err = relayv2.New(h)
	require.NoError(t, err, "the relay service must attach")

	port, ok := tcpPortOf(h.Addrs())
	require.True(t, ok, "the relay must listen on TCP: %v", h.Addrs())
	return natRelayInfo{id: h.ID(), port: port}
}

// startNATTestbed runs the DHT the participants rendezvous through, and
// reports the port it listens on. The address handed to each participant is
// the bridge inside its own namespace's subnet — the node's own view of its
// addresses would name interfaces the namespace cannot see.
func startNATTestbed(t *testing.T, inv rtc.Invite) (peer.ID, int) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	bed, err := rtc.NewNode(rtc.NodeOptions{Invite: inv})
	require.NoError(t, err, "the testbed node could not start")
	t.Cleanup(func() { _ = bed.Close() })
	require.NoError(t, bed.Bootstrap(ctx, nil), "the testbed DHT could not start")

	port, ok := tcpPortOf(bed.AddrInfo().Addrs)
	require.True(t, ok, "the testbed must listen on TCP: %v", bed.AddrInfo().Addrs)
	return bed.ID(), port
}

// p2pAddr is the one address a participant needs: an IP the testbed or relay
// is reachable at, its port, and the peer it belongs to.
func p2pAddr(ip string, port int, id peer.ID) string {
	return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", ip, port, id)
}

func tcpPortOf(addrs []multiaddr.Multiaddr) (int, bool) {
	for _, a := range addrs {
		value, err := a.ValueForProtocol(multiaddr.P_TCP)
		if err != nil || value == "" {
			continue
		}
		var port int
		if _, err := fmt.Sscanf(value, "%d", &port); err == nil {
			return port, true
		}
	}
	return 0, false
}

// waitForMeshWithin is waitForMesh without the fatal, so a participant can say
// what it knew at the moment it gave up.
func waitForMeshWithin(t *testing.T, m interface{ Peers() []string }, want int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(m.Peers()) >= want {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
