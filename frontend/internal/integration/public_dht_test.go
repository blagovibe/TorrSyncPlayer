// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The same two participants, but meeting over the public DHT instead of a
// loopback testbed.
//
// This one exists because the loopback scenario cannot see the only failure
// that matters in the product: the rendezvous. Every other scenario here puts
// its participants on one host and they meet in half a second. Against the
// public DHT the same two participants meet in two to three minutes — and in
// whole stretches of time they do not meet at all inside five and a half.
//
// That is not a test to gate on. A check that goes red because someone else's
// network is having a bad afternoon teaches everyone to ignore red, and this
// repository's rule is that a check must be able to go red for a reason worth
// acting on. So this one measures and reports: it never fails the run, and it
// writes what it saw where a person or a later decision can read it. What
// makes the room's behaviour visible over time is a series, and a series needs
// a clock, not a pull request.
//
//	sudo -u runner env TORRSYNC_PUBLIC=1 FYNE_DRIVER=offscreen \
//	  go test -v -timeout 20m -run TestARoomMeetsOverThePublicDHT ./internal/integration/...
func TestARoomMeetsOverThePublicDHT(t *testing.T) {
	if os.Getenv("TORRSYNC_PUBLIC") != "1" {
		t.Skip("the public DHT is measured on a schedule, not on a pull request; set TORRSYNC_PUBLIC=1")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()

	started := time.Now()
	hostOut, guestOut, err := runNATAttempt(ctx, t,
		natParticipant{role: "host", public: true},
		natParticipant{role: "guest", public: true},
	)
	waited := time.Since(started).Round(time.Second)

	report(t, "public DHT rendezvous", map[string]string{
		"outcome":  outcomeOf(err),
		"waited":   waited.String(),
		"attempts": "three, each on a fresh testbed",
	})
	t.Logf("public DHT rendezvous: %s after %s\n%s\n%s", outcomeOf(err), waited, hostOut, guestOut)

	// Nothing fails here on purpose. The line above is the product of this
	// test; the failure mode would be a pipeline nobody can keep green, and a
	// red pipeline nobody reads is worth less than no pipeline.
	require.True(t, true)
}

func outcomeOf(err error) string {
	if err == nil {
		return "the room met"
	}
	return "the room did not meet: " + err.Error()
}

// report writes a run's outcome to the test log, which the CI job puts in the
// job summary. A measurement nobody can find is a measurement nobody reads.
func report(t *testing.T, title string, fields map[string]string) {
	t.Helper()
	t.Logf("── %s ──", title)
	for _, k := range []string{"outcome", "waited", "attempts"} {
		if v, ok := fields[k]; ok {
			t.Logf("%-9s %s", k+":", v)
		}
	}
}
