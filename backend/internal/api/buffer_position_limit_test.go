// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
)

// validBufferTorrentID is a syntactically valid infohash for the request
// validators. The torrent does not have to exist: these tests are about the
// limiter in front of the handler, and a limiter applies either way.
const validBufferTorrentID = "0123456789abcdef0123456789abcdef01234567"

// bufferPositionURL builds the concrete request URL from the route constant.
func bufferPositionURL() string {
	const prefix = "/api/v1/torrents/"
	return APIPathTorrentBufferPosition[:len(prefix)] + validBufferTorrentID + "/buffer/position"
}

// withShippedAPIRateLimits installs the rate limits that actually ship.
//
// TestMain raises API_RATE_LIMIT to 100 req/s for the rest of the suite, which
// is right for tests that are not about limiting but makes every one of them
// blind to it: a shipped 1 req/s budget cannot be observed from inside a suite
// running at 100. The regression this file pins — a client's own 2 Hz
// heartbeat answered 429 — was invisible for exactly that reason.
//
// Tests that care call this and get the previous limiter back afterwards.
func withShippedAPIRateLimits(t *testing.T) {
	t.Helper()
	previous := globalClientRateLimiter
	t.Cleanup(func() {
		previous.Stop()
		globalClientRateLimiter = newClientRateLimiter(resolveAPIRateLimit(), resolveAPIRateBurst())
	})
	globalClientRateLimiter.Stop()
	globalClientRateLimiter = newClientRateLimiter(
		rate.Limit(constants.APIRateLimit), constants.APIRateBurst)
}

// postBufferPosition sends one playback-position report the way the Qt
// frontend does: an authenticated POST carrying one integer.
func postBufferPosition(t *testing.T, clientID string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, bufferPositionURL(),
		strings.NewReader(`{"position":1024}`))
	req.Header.Set("Content-Type", "application/json")
	applyAuthHeadersAs(req, clientID)

	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	return rec.Code
}

// TestBufferPosition_SurvivesClientCadence pins the cadence the frontend
// actually sends.
//
// MainWindow::m_bufferPositionTimer fires every 500 ms — two position reports
// per second for as long as a file is playing. Fifteen back-to-back reports
// are what one client sends in the first seven seconds of playback.
//
// Under the protected group's 1 req/s bucket (burst 10) this fails: the first
// ten pass and the rest are answered 429. Because 429 is a 4xx the client does
// not retry it (networkmanager.cpp retries only on 5xx and on a transport
// failure), so piece priorities silently froze a few seconds into playback —
// the exact defect bug-0028 set out to fix.
func TestBufferPosition_SurvivesClientCadence(t *testing.T) {
	withShippedAPIRateLimits(t)
	ResetBufferPositionRateLimiter()
	t.Cleanup(ResetBufferPositionRateLimiter)

	const reportsInFirstSeconds = 15

	codes := make([]int, 0, reportsInFirstSeconds)
	for i := 0; i < reportsInFirstSeconds; i++ {
		codes = append(codes, postBufferPosition(t, "cadence-client"))
	}

	var throttled int
	for _, code := range codes {
		if code == http.StatusTooManyRequests {
			throttled++
		}
	}
	assert.Zero(t, throttled,
		"a single client's own cadence must not be rate limited; got %v", codes)
}

// TestBufferPosition_BudgetCoversRoomBehindOneNAT checks the shipped budget
// against the topology the project actually has: several participants of one
// room can share one public address, and each reports twice a second.
//
// AllowN evaluates the bucket at a synthetic instant, so the assertion costs
// no wall-clock time and cannot flake.
func TestBufferPosition_BudgetCoversRoomBehindOneNAT(t *testing.T) {
	const players = 8
	const reportsPerPlayerPerSecond = 2

	limiter := newClientRateLimiter(
		resolveBufferPositionRateLimit(), resolveBufferPositionRateBurst())
	t.Cleanup(limiter.Stop)

	assert.True(t,
		limiter.getLimiter("203.0.113.7").
			AllowN(time.Now().Add(time.Second), players*reportsPerPlayerPerSecond),
		"shipped budget %v req/s burst %d must absorb %d players reporting %d times a second",
		constants.BufferPositionRateLimit, constants.BufferPositionRateBurst,
		players, reportsPerPlayerPerSecond)
}

// TestBufferPosition_DoesNotSpendControlPlaneBudget is the other half of the
// contract. The position endpoint must not merely have a bigger limit of its
// own — it must be off the control-plane budget entirely, so a room full of
// players cannot starve the endpoints that actually cost something.
//
// The sharp form of the assertion: after the heartbeat has run, the
// control-plane bucket must still hand out its full burst and only then start
// refusing. Had the position reports been charged to that bucket, not a single
// control-plane call would succeed.
func TestBufferPosition_DoesNotSpendControlPlaneBudget(t *testing.T) {
	withShippedAPIRateLimits(t)
	ResetBufferPositionRateLimiter()
	t.Cleanup(ResetBufferPositionRateLimiter)

	// More position reports than the control-plane burst, at a cadence that
	// would have drained it.
	const heartbeats = constants.APIRateBurst * 3
	for i := 0; i < heartbeats; i++ {
		require.NotEqual(t, http.StatusTooManyRequests, postBufferPosition(t, "noisy-client"),
			"heartbeat %d was throttled by the control-plane budget", i)
	}

	// The control-plane bucket must still be full.
	for i := 0; i < constants.APIRateBurst; i++ {
		req := httptest.NewRequest(http.MethodGet, APIPathTorrents, nil)
		applyAuthHeadersAs(req, "noisy-client")
		rec := httptest.NewRecorder()
		apiRouter.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusTooManyRequests, rec.Code,
			"control-plane call %d was refused: heartbeat reports were charged to its budget", i+1)
	}

	// And it must still be a limiter, not a hole.
	req := httptest.NewRequest(http.MethodGet, APIPathTorrents, nil)
	applyAuthHeadersAs(req, "noisy-client")
	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code,
		"control-plane budget must still be enforced after its burst is spent")
}

// TestBufferPosition_StillRequiresToken guards the obvious wrong fix. The 429s
// came from the wrong bucket, not from a missing control: dropping the limiter
// would silence the symptom and hand the endpoint to anyone who guesses a URL.
func TestBufferPosition_StillRequiresToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, bufferPositionURL(),
		strings.NewReader(`{"position":1024}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestBufferPosition_RejectsWrongToken makes sure the new bucket is not a way
// around the token check for someone holding a bad one.
func TestBufferPosition_RejectsWrongToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, bufferPositionURL(),
		strings.NewReader(`{"position":1024}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderAccessToken, "not-the-token")
	req.Header.Set(auth.HeaderClientID, "cadence-client")

	rec := httptest.NewRecorder()
	apiRouter.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestBufferPosition_FloodIsStillRejected keeps the endpoint bounded. A
// position report is a tiny authenticated write, so the budget is generous —
// but generous must not mean unlimited, or the endpoint becomes a free
// write-amplification loop for anything that got hold of the token.
func TestBufferPosition_FloodIsStillRejected(t *testing.T) {
	ResetBufferPositionRateLimiter()
	t.Cleanup(ResetBufferPositionRateLimiter)

	const flood = constants.BufferPositionRateBurst * 3

	rejected := 0
	for i := 0; i < flood; i++ {
		if postBufferPosition(t, "flood-client") == http.StatusTooManyRequests {
			rejected++
		}
	}
	assert.Positive(t, rejected,
		"a flood of %d reports must eventually be rejected, not all admitted", flood)
}
