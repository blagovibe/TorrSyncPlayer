// Package contract provides Pact provider verification tests.

//go:build contract
// +build contract

package contract

import (
	"context"
	"encoding/json"
	"fmt"
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

// The client id the recorded interactions present, and therefore the user the
// room provider states have to set up. There are no accounts by design, so the
// caller's identity is this header.
const pactUserID = "pact-frontend"

// pactTorrentID is a syntactically valid infohash for a torrent nobody has.
//
// A room is bound to a torrent and the binding is required, but p2p does not
// look the torrent up — the service that owns the torrent is a different one —
// and this contract is about the shape of the answer, not about downloading
// anything.
const pactTorrentID = "0000000000000000000000000000000000000001"

// noopStateHandler does nothing: the backend manages its own per-request state,
// so no pact provider state needs setup. Signature per pact-go v2
// models.StateHandler.
func noopStateHandler(bool, models.ProviderState) (models.ProviderStateResponse, error) {
	return nil, nil
}

// inARoom establishes the precondition its name promises.
//
// It used to be a no-op, and that was true when it was written: the sync
// handlers did not ask who was calling. They do now — every sync endpoint
// resolves the caller's room first and answers 400 without one — so a no-op
// turned "user is in a room" into a promise the provider never kept, and the
// verification failed on the very state it claimed to be in.
func inARoom(p2pSvc *p2p.Service) func(bool, models.ProviderState) (models.ProviderStateResponse, error) {
	return func(bool, models.ProviderState) (models.ProviderStateResponse, error) {
		ctx := context.Background()

		// The verifier replays every interaction back to back, and the
		// room-creation interaction already puts this client in a room.
		// Creating another would leave the later sync interactions checking a
		// room the caller is not in.
		if _, err := p2pSvc.GetRoomInfo(ctx, pactUserID); err == nil {
			return nil, nil
		}

		// CreateRoom makes the caller the host and puts them in the roster,
		// which is what both room states describe.
		if _, err := p2pSvc.CreateRoom(ctx, pactUserID, "Pact Room", "", pactTorrentID); err != nil {
			return nil, fmt.Errorf("provider state: put %q in a room: %w", pactUserID, err)
		}
		return nil, nil
	}
}

// stateHandlers maps every provider state named in the pact file.
func stateHandlers(p2pSvc *p2p.Service) models.StateHandlers {
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
	handlers["user is in a room"] = inARoom(p2pSvc)
	handlers["user is in a room and is host"] = inARoom(p2pSvc)
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

	// The p2p service is the host checker the sync service asks: only the host
	// may drive playback. Production wires it the same way (cmd/server/main.go).
	//
	// This file is behind the `contract` build tag, which is why a changed
	// constructor here stayed invisible: plain `go test ./...` and `go vet`
	// never compile it, and only the pact job did — which means that job has
	// been red, not green, since the checker was introduced (e2fb119).
	syncSvc := sync.NewService(p2pSvc)
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
		StateHandlers:              stateHandlers(p2pSvc),
		RequestTimeout:             60 * time.Second,
	}

	if err := provider.NewVerifier().VerifyProvider(t, request); err != nil {
		t.Fatalf("Pact verification failed: %v", err)
	}
}
