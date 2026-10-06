// Package contract provides Pact provider verification tests.

//go:build contract
// +build contract

package contract

import (
	"net/http/httptest"
	"os"
	"path/filepath"
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

// noopStateHandler does nothing: the real backend manages its own state per
// request, so the pact's provider states need no setup here. Signature per
// pact-go v2 models.StateHandler.
func noopStateHandler(bool, models.ProviderState) (models.ProviderStateResponse, error) {
	return nil, nil
}

// stateHandlers maps every provider state named in the pact file.
func stateHandlers() models.StateHandlers {
	states := []string{
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

	authService, err := auth.NewAuthService([]byte("pact-test-secret-key-for-verification-32b!"))
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}
	authStore := auth.NewUserStore()
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
		AuthStore:   authStore,
		AuthService: authService,
	})

	server := httptest.NewServer(router)
	defer server.Close()

	// Get the path to the pact file. PactFiles takes local paths; PactURLs is
	// for URLs, and passing a filesystem path there fails with a builder error.
	pactPath := filepath.Join("..", "..", "..", "pacts", "frontend-backend.json")
	if _, err := os.Stat(pactPath); err != nil {
		t.Fatalf("pact file not found at %s: %v", pactPath, err)
	}

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
