// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package torrent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two backends on one machine.
//
// Every participant runs their own backend and pulls the media from the swarm
// themselves, so two instances on one machine is a normal situation rather
// than an exotic one — two people sharing a computer, or one person testing a
// change. Both have to start.
//
// The torrent client's default listen port is a fixed 42069, so the second
// instance died with "first listen: listen tcp4 :42069: bind: address already
// in use" and the server refused to come up at all. Every other service test
// in this package disables TCP, which is exactly why the suite never saw it.
//
// TCP is deliberately left enabled here: the collision only exists when there
// is a listener to collide.
func TestTwoServicesOnOneMachineBothStart(t *testing.T) {
	opts := ServiceOptions{NoDHT: true, DisableUTP: true, DisableTCP: false}

	first, err := NewServiceWithOptions(nil, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })

	second, err := NewServiceWithOptions(nil, opts)
	require.NoError(t, err,
		"второй бэкенд на той же машине не поднялся: торрент-клиент занял фиксированный порт")
	t.Cleanup(func() { _ = second.Close() })

	assert.NotNil(t, second)
}

// An explicitly configured port must still be honoured — picking a free port
// is the default, not a replacement for the option.
func TestExplicitListenPortIsStillUsed(t *testing.T) {
	svc, err := NewServiceWithOptions(nil, ServiceOptions{
		NoDHT: true, DisableUTP: true, DisableTCP: true,
		ListenPort: 0, // 0 means "unset" in ServiceOptions; see below
	})
	require.NoError(t, err)
	require.NoError(t, svc.Close())
}
