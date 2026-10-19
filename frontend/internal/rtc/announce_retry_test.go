// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package rtc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The wide publish is the only thing that makes a room findable from another
// part of the DHT, and against the public DHT it is measured in minutes —
// sometimes it completed in under two, and twice in five runs it had not
// landed at all inside five and a half. It used to be attempted exactly once
// at announce and then left alone for half an hour, so a publish that lost
// that race cost the room half an hour of being invisible (bug-0128).
//
// The fix is a policy, and the policy is what these tests watch: try again
// soon until the room is actually out there, and only then settle into the
// long refresh that keeps it there.

func TestTheWidePublishIsRetriedUntilTheRoomIsActuallyFound(t *testing.T) {
	assert.Less(t, widePublishDelay(false), widePublishDelay(true),
		"a room that has not been published yet must be published again long before the refresh")

	assert.LessOrEqual(t, widePublishDelay(false), 2*time.Minute,
		"retrying every five minutes is the failure this fixes: the room was invisible that long")
	assert.GreaterOrEqual(t, widePublishDelay(false), 30*time.Second,
		"retrying faster than this turns the DHT into a firehose for no gain")
}

func TestAPublishedRoomSettlesIntoTheLongRefresh(t *testing.T) {
	assert.Equal(t, announceWideningRepublishes, widePublishDelay(true),
		"once the room is out there, it is a matter of keeping it there")
}
