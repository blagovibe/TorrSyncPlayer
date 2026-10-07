// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
)

func TestNewAuthServiceGeneratesUsableToken(t *testing.T) {
	svc, err := NewAuthService()
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}

	token := svc.AccessToken()
	if len(token) != AccessTokenLength*2 {
		t.Fatalf("token length = %d chars, want %d", len(token), AccessTokenLength*2)
	}
	if !svc.ValidateAccessToken(token) {
		t.Fatal("generated token must validate against its own service")
	}
}

func TestTokensAreNotSharedBetweenProcesses(t *testing.T) {
	// Each start generates a fresh token, so a token from a previous run — or a
	// token minted for someone else's server — is never accepted.
	a, err := NewAuthService()
	if err != nil {
		t.Fatalf("NewAuthService a: %v", err)
	}
	b, err := NewAuthService()
	if err != nil {
		t.Fatalf("NewAuthService b: %v", err)
	}

	if a.AccessToken() == b.AccessToken() {
		t.Fatal("two services produced the same token")
	}
	if b.ValidateAccessToken(a.AccessToken()) {
		t.Fatal("service b accepted service a's token")
	}
}

func TestValidateAccessTokenRejects(t *testing.T) {
	svc, err := NewAuthService()
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}

	cases := map[string]string{
		"empty":     "",
		"not hex":   "zzzz",
		"all zeros": strings.Repeat("00", AccessTokenLength),
		"truncated": svc.AccessToken()[:len(svc.AccessToken())-2],
	}
	for name, presented := range cases {
		if svc.ValidateAccessToken(presented) {
			t.Errorf("%s: token was accepted but must be rejected", name)
		}
	}
}

func TestNewAuthServiceWithToken(t *testing.T) {
	svc, err := NewAuthServiceWithToken("known-token-for-tests")
	if err != nil {
		t.Fatalf("NewAuthServiceWithToken: %v", err)
	}
	if !svc.ValidateAccessToken("known-token-for-tests") {
		t.Fatal("restored token must validate")
	}

	if _, err := NewAuthServiceWithToken(""); err == nil {
		t.Fatal("empty token should be rejected")
	}
}

func TestStreamTicketRoundTrip(t *testing.T) {
	svc, err := NewAuthService()
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}

	const clientID = "client-abc"
	const torrentID = "0123456789abcdef0123456789abcdef01234567"

	ticket, err := svc.GenerateStreamTicket(clientID, torrentID)
	if err != nil {
		t.Fatalf("GenerateStreamTicket: %v", err)
	}

	gotID, ok := svc.ValidateStreamTicket(ticket, torrentID)
	if !ok {
		t.Fatal("freshly issued ticket did not validate")
	}
	if gotID != clientID {
		t.Fatalf("client id = %q, want %q", gotID, clientID)
	}
}

func TestStreamTicketRejects(t *testing.T) {
	svc, err := NewAuthService()
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}

	const clientID = "client-abc"
	const torrentID = "0123456789abcdef0123456789abcdef01234567"

	ticket, err := svc.GenerateStreamTicket(clientID, torrentID)
	if err != nil {
		t.Fatalf("GenerateStreamTicket: %v", err)
	}

	t.Run("wrong torrent", func(t *testing.T) {
		if _, ok := svc.ValidateStreamTicket(ticket, "ffffffffffffffffffffffffffffffffffffffff"); ok {
			t.Error("ticket validated for a torrent it was not issued for")
		}
	})

	t.Run("tampered signature", func(t *testing.T) {
		parts := strings.Split(ticket, ".")
		parts[3] = strings.Repeat("0", len(parts[3]))
		if _, ok := svc.ValidateStreamTicket(strings.Join(parts, "."), torrentID); ok {
			t.Error("ticket with a forged signature was accepted")
		}
	})

	t.Run("issued by another process", func(t *testing.T) {
		other, err := NewAuthService()
		if err != nil {
			t.Fatalf("NewAuthService: %v", err)
		}
		theirs, err := other.GenerateStreamTicket(clientID, torrentID)
		if err != nil {
			t.Fatalf("GenerateStreamTicket: %v", err)
		}
		if _, ok := svc.ValidateStreamTicket(theirs, torrentID); ok {
			t.Error("ticket signed by a different process was accepted")
		}
	})

	t.Run("expired", func(t *testing.T) {
		// Backdate the expiry inside an otherwise correctly signed ticket.
		expiry := time.Now().Add(-time.Minute).Unix()
		payload := strconv.FormatInt(expiry, 10) + "|" + clientID + "|" + torrentID
		mac := hmac.New(sha256.New, svc.streamTicketKey())
		sig := hex.EncodeToString(mac.Sum([]byte(payload)))
		old := payload[:strings.Index(payload, "|")] + "." + clientID + "." + torrentID + "." + sig
		if _, ok := svc.ValidateStreamTicket(old, torrentID); ok {
			t.Error("expired ticket was accepted")
		}
	})

	t.Run("malformed", func(t *testing.T) {
		for _, bad := range []string{"", "abc", "1.2.3", "x.y.z.w"} {
			if _, ok := svc.ValidateStreamTicket(bad, torrentID); ok {
				t.Errorf("malformed ticket %q was accepted", bad)
			}
		}
	})
}

func TestStreamTicketRequiresBothIDs(t *testing.T) {
	svc, err := NewAuthService()
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	if _, err := svc.GenerateStreamTicket("", "abc"); err == nil {
		t.Error("empty client id should be rejected")
	}
	if _, err := svc.GenerateStreamTicket("abc", ""); err == nil {
		t.Error("empty torrent id should be rejected")
	}
}

func TestStreamTicketTTLIsShort(t *testing.T) {
	// The ticket is handed to a media player, so it must not outlive its
	// usefulness by much.
	if constants.StreamTicketTTL > 10*time.Minute {
		t.Fatalf("stream ticket TTL is %v, expected a short lifetime", constants.StreamTicketTTL)
	}
}
