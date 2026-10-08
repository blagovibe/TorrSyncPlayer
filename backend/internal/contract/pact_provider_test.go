// Package contract provides Pact provider verification tests.

//go:build contract
// +build contract

package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/models"
	"github.com/pact-foundation/pact-go/v2/provider"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/p2p"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/sync"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/torrent"
)

// Token placeholder used in pacts/frontend-backend.json. The contract pins the
// *shape* of an authenticated call, not a specific token value: a literal
// "Bearer test-token" can never satisfy a JWT-validating router (every
// interaction came back 401). The verifier therefore mints a real token and
// substitutes it for this marker before running.
const pactTokenPlaceholder = "${PACT_TOKEN}"

// Credentials that match the login interaction in the pact file.
const ()

// materialisePact loads the pact, replaces every token placeholder with the
// supplied token and writes the result to a temp file for verification.
func materialisePact(t *testing.T, token string) string {
	t.Helper()

	src := filepath.Join("..", "..", "..", "pacts", "frontend-backend.json")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("pact file not found at %s: %v", src, err)
	}

	resolved := strings.ReplaceAll(string(raw), pactTokenPlaceholder, token)
	if strings.Contains(resolved, pactTokenPlaceholder) {
		t.Fatal("pact still contains an unsubstituted token placeholder")
	}
	// Sanity check: the rewritten file must stay valid JSON, otherwise the
	// verifier would fail with a parse error that looks like a contract mismatch.
	var probe any
	if err := json.Unmarshal([]byte(resolved), &probe); err != nil {
		t.Fatalf("rewritten pact is not valid JSON: %v", err)
	}

	out := filepath.Join(t.TempDir(), "frontend-backend.json")
	if err := os.WriteFile(out, []byte(resolved), 0o600); err != nil {
		t.Fatalf("failed to write rewritten pact: %v", err)
	}
	return out
}

// noopStateHandler does nothing: the backend manages its own per-request state,
// so no pact provider state needs setup. Signature per pact-go v2
// models.StateHandler.
func noopStateHandler(bool, models.ProviderState) (models.ProviderStateResponse, error) {
	return nil, nil
}

// stateHandlers maps every provider state named in the pact file.
func stateHandlers() models.StateHandlers {
	states := []string{
		"server is running",
		"torrent exists",
		"no torrents exist",
		"user is in a room",
		"user is in a room and is host",
		"user exists",
		"user does not exist",
	}
	handlers := make(models.StateHandlers, len(states))
	for _, state := range states {
		handlers[state] = noopStateHandler
	}
	return handlers
}

// TestPactProvider verifies that the REAL backend API satisfies
// the contract defined in the pact file. Unlike earlier versions that
// served canned mock responses, this stands up the actual router with
// real services so verification reflects production behaviour.
func TestPactProvider(t *testing.T) {
	// Build the real router with real (in-memory) services.
	bufferSvc := buffer.NewService(64 * 1024 * 1024)
	torrentSvc, err := torrent.NewServiceWithOptions(bufferSvc, torrent.ServiceOptions{
		NoDHT:      true,
		DisableUTP: true,
		DisableTCP: true,
		ListenPort: 0,
	})
	if err != nil {
		t.Fatalf("failed to create torrent service: %v", err)
	}
	defer torrentSvc.Close()

	authService, err := auth.NewAuthService()
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// The token is generated at startup, so the pact can be materialised with a
	// real value straight away — no fixture account has to exist first.
	pactToken := authService.AccessToken()

	p2pSvc, err := p2p.NewService(authService)
	if err != nil {
		t.Fatalf("failed to create p2p service: %v", err)
	}
	defer p2pSvc.Close()

	syncSvc := sync.NewService()
	defer syncSvc.Close()

	router := api.NewRouter(api.RouterConfig{
		TorrentSvc:  torrentSvc,
		P2pSvc:      p2pSvc,
		SyncSvc:     syncSvc,
		AuthService: authService,
	})

	// The verifier replays every recorded interaction back-to-back from
	// 127.0.0.1, so the production per-IP limiter would throttle the run
	// part-way through and report 429s that have nothing to do with the
	// contract. Clearing the bucket before each request keeps the replay
	// faithful; the limiter is a defence against real clients, not against
	// a test harness reading a file.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.ResetClientRateLimiter()
		router.ServeHTTP(w, r)
	}))
	defer server.Close()

	pactPath := materialisePact(t, pactToken)

	// Configure Pact provider verification. The real backend manages its own
	// state per request, so the provider state handlers are intentionally
	// no-ops (kept for compatibility with the pact's providerStates setup).
	// pact-go v2 exposes provider.NewVerifier() with a provider.VerifyRequest.
	// The older provider.VerifierConfig / provider.VerifyProvider(ctx, cfg) pair
	// no longer exists and made this test fail to compile.
	request := provider.VerifyRequest{
		ProviderBaseURL: server.URL,
		PactFiles:       []string{pactPath},
		Provider:        "TorrSyncPlayer-Backend",
		// The consumer name lives in the pact file itself; FilterConsumers
		// restricts verification to that consumer's interactions.
		FilterConsumers:            []string{"TorrSyncPlayer-Frontend"},
		PublishVerificationResults: false, // set to true to publish to a Pact Broker
		BrokerURL:                  os.Getenv("PACT_BROKER_URL"),
		BrokerToken:                os.Getenv("PACT_BROKER_TOKEN"),
		StateHandlers:              stateHandlers(),
		RequestTimeout:             60 * time.Second,
	}

	if err := provider.NewVerifier().VerifyProvider(t, request); err != nil {
		t.Fatalf("Pact verification failed: %v", err)
	}
}
