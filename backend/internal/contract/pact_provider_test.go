// Package contract provides Pact provider verification tests.

//go:build contract
// +build contract

package contract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/provider"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/p2p"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/sync"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/torrent"
)

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

	// Get the path to the pact file
	pactPath := filepath.Join("..", "..", "..", "..", "pacts", "frontend-backend.json")

	// Configure Pact provider verification. The real backend manages its own
	// state per request, so the provider state handlers are intentionally
	// no-ops (kept for compatibility with the pact's providerStates setup).
	config := provider.VerifierConfig{
		ProviderBaseURL: server.URL,
		PactURLs:        []string{pactPath},
		Provider:        "TorrSyncPlayer-Backend",
		Consumer:        "TorrSyncPlayer-Frontend",
		PublishResults:  false, // Set to true to publish to Pact Broker
		BrokerURL:       os.Getenv("PACT_BROKER_URL"),
		BrokerToken:     os.Getenv("PACT_BROKER_TOKEN"),
		StateHandlers: map[string]func() error{
			"torrent exists":                func() error { return nil },
			"no torrents exist":             func() error { return nil },
			"user is in a room":             func() error { return nil },
			"user is in a room and is host": func() error { return nil },
			"user exists":                   func() error { return nil },
			"user does not exist":           func() error { return nil },
		},
		RequestFilter: func(req *http.Request) error {
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := provider.VerifyProvider(ctx, config); err != nil {
		t.Fatalf("Pact verification failed: %v", err)
	}
}
