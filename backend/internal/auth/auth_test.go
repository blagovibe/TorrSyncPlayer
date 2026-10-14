// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package auth

import (
	"strings"
	"testing"
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
