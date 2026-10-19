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
// The relayed assertion is what makes this impossible to pass vacuously: if
// `ip netns exec` were a no-op, both participants would sit on the host,
// connect directly, and the room would be green — because a room on one host
// is exactly what every other scenario here already proves. The test can only
// go green with the two sides genuinely separated and genuinely talking
// through a relay.
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
	natEnvSetup    = "TORRSYNC_NETNS"
	natHostNS      = "ts-host"
	natGuestNS     = "ts-guest"
	natHostBridge  = "10.77.1.1"
	natGuestBridge = "10.77.2.1"
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
			natParticipant{
				role: "host", namespace: natHostNS, bridge: natHostBridge,
				relay: natRelayFor(natHostBridge, relayInfo), expectRelay: "1",
			},
			natParticipant{
				role: "guest", namespace: natGuestNS, bridge: natGuestBridge,
				relay: natRelayFor(natGuestBridge, relayInfo), expectRelay: "1",
			},
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
	if host.public {
		// No testbed: the production rendezvous, the project's own seed list,
		// exactly as a person gets it with an empty --bootstrap.
		host.bootstrap, guest.bootstrap = "", ""
		host.invite, guest.invite = inv, inv
		return runParticipantPair(ctx, t, host, guest)
	}

	bedID, bedPort := startNATTestbed(t, inv)
	host.bootstrap = p2pAddr(host.bridge, bedPort, bedID)
	guest.bootstrap = p2pAddr(guest.bridge, bedPort, bedID)
	host.invite, guest.invite = inv, inv

	// Both at once. Staggering them looks like a closer imitation of how a room
	// forms — someone opens it, someone else walks in — and it is exactly
	// wrong here: the host meets the guest, announces its film and finishes,
	// while the guest is still starting up and waits three minutes for someone
	// who has already gone. Tried, measured, reverted.
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

// runParticipantPair starts both sides and waits for both to finish. A public
// meeting can take minutes, so the participants' own wait is the window; the
// coordinator does not add one of its own on top.
func runParticipantPair(ctx context.Context, t *testing.T, host, guest natParticipant) (string, string, error) {
	t.Helper()

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
	role := natFlag("-nat-role")
	if role == "" {
		t.Skip("not a participant: run me through the coordinator, not directly")
	}

	wait, err := time.ParseDuration(natFlag("-nat-wait"))
	if err != nil || wait <= 0 {
		wait = participantWaitDefault
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait+time.Minute)
	defer cancel()

	parts := strings.Split(natFlag("-nat-invite"), ":")
	require.Len(t, parts, 3, "the invite must be community:password:key")
	inv := rtc.Invite{Community: parts[0], Password: parts[1], Key: parts[2]}

	mesh := rtc.NewMesh(rtc.MeshOptions{
		Bootstrap: natFlag("-nat-bootstrap"),
		Relays:    natFlag("-nat-relay"),
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
	// The roster is the room's truth: it is who said hello and has not said
	// bye. The mesh's peer list is a transport fact that comes and goes with
	// the connections under it, and a participant that waits for it waits for
	// a coincidence of timings.
	roster := make(chan []rtc.Peer, 8)
	room := rtc.NewRoom(mesh, mesh.SelfID(), role, role == "host", &rtc.Callbacks{
		OnMagnet: func(m rtc.Magnet) {
			select {
			case gotMagnet <- m:
			default:
			}
		},
		OnRoster: func(peers []rtc.Peer) {
			select {
			case roster <- peers:
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
	// own impatience — and the product's, which is the thing under test. The
	// coordinator may ask for less, which is how the same machinery gets three
	// real attempts out of one budget instead of one and two that ran out of it.
	if !waitForRoster(t, roster, 1, wait) {
		t.Fatalf("the room never met with no direct path open (status %+v)", mesh.ConnStatus())
	}

	st := mesh.ConnStatus()
	require.Len(t, st.Peers, 1, "the participant must see exactly one peer")
	if natFlag("-nat-expect-relay") == "1" {
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

		// The host stays until the guest has left. A host that vanishes the
		// moment it has a peer takes the room with it: the guest is left
		// holding a half-open stream, and the scenario measures a race
		// between two processes rather than the room.
		waitForRoster(t, roster, 0, 30*time.Second)
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
	// public means: no testbed, no relay, the project's own seed list.
	public bool
	// wait is how long this participant waits for the room to meet. Zero means
	// the default, which is sized for a real rendezvous.
	wait time.Duration
}

// participantWaitDefault is the window when nobody asks for one: the two
// minutes it takes a room to meet over the public DHT, plus room to spare.
const participantWaitDefault = 3 * time.Minute

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
	// A participant with no role is a process that starts, skips itself and
	// reports success — which is how this scenario spent two runs on a real
	// runner saying "did not meet" for a reason nobody could see. The mistake
	// was a coordinator literal that simply omitted the role, and the child
	// had no way to object. It objects here instead.
	if strings.TrimSpace(p.role) == "" {
		return "", fmt.Errorf("a participant needs a role; %q was given none", p.namespace)
	}
	if strings.TrimSpace(p.invite.Community) == "" {
		return "", fmt.Errorf("a participant needs an invite; %q was given none", p.role)
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("the test binary must be executable: %w", err)
	}

	// Everything the participant needs travels as arguments rather than as
	// environment variables. The first run of this scenario on a real runner
	// found that out the hard way: the children started, and reported that
	// they were not participants — the role had not reached them. Nothing
	// between here and there should be able to drop a value quietly, and a
	// command line is the one channel that survives a privilege wrapper.
	//
	// "self" means no namespace at all: the coordinator names one, and a local
	// smoke run of the helper itself names this.
	args := []string{
		"-test.run", "^TestNATParticipant$",
		"-test.v",
		"-test.timeout", "7m",
		"-nat-role", p.role,
		"-nat-invite", strings.Join([]string{p.invite.Community, p.invite.Password, p.invite.Key}, ":"),
		"-nat-bootstrap", p.bootstrap,
		"-nat-relay", p.relay,
		"-nat-expect-relay", p.expectRelay,
		"-nat-wait", p.wait.String(),
	}
	var cmd *exec.Cmd
	if namespace != "" && namespace != "self" {
		cmd = exec.CommandContext(ctx, "ip", append([]string{"netns", "exec", namespace, exe}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, exe, args...)
	}
	// The environment is only the toolchain's: a display for Fyne that this
	// namespace certainly has not got, and nothing else worth carrying.
	cmd.Env = append(os.Environ(), "FYNE_DRIVER=offscreen")

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

// waitForRoster waits until the room's roster holds exactly the wanted
// number of people. The roster is who said hello and has not said bye: it is
// the room's own account of who is watching, and unlike the transport's peer
// list it does not empty itself when a connection blinks.
//
// What is already queued is dropped first: the roster fires on every change,
// and a stale entry is not the answer.
func waitForRoster(t *testing.T, roster <-chan []rtc.Peer, want int, timeout time.Duration) bool {
	t.Helper()
	for {
		select {
		case <-roster:
			continue
		default:
		}
		break
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case peers := <-roster:
			if len(peers) == want {
				return true
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	return false
}
