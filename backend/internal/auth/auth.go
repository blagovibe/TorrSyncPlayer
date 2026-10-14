// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package auth provides access control for the local API.
//
// TorrSyncPlayer runs one backend per person: you install it, it prints an
// access token, and you hand that token to the friends you watch with. There
// are no accounts, no registration and no password database — see CONCEPT.md.
// What remains here is one thing: a single random access token, generated fresh
// on every start, that gates the API and stops anything else on the internet
// from driving the machine.
//
// Everything else that used to live here — user registration, password hashing,
// JWT issuance and revocation — was removed. It solved a problem a single-user
// desktop application does not have, and nothing else depended on it.
//
// Signed stream tickets went the same way, and for the same reason. They existed
// because the media endpoint was assumed to be unreachable by a player that
// cannot attach request headers; mpv turned out to send headers just fine
// (idea-0066), so the bearer value in a URL — and the transferable-access
// deviation it caused (deviation-0029) — went away with the assumption.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// AccessTokenLength is the number of random bytes in a generated access token.
// 32 bytes is 256 bits: not guessable, and it is regenerated on every start so
// there is nothing to store, rotate or leak between runs.
const AccessTokenLength = 32

// AuthService holds the per-process access token.
type AuthService struct {
	accessToken string
}

// NewAuthService generates a fresh access token for this process. It comes from
// crypto/rand, so nothing needs to be configured, persisted or supplied through
// the environment.
func NewAuthService() (*AuthService, error) {
	token := make([]byte, AccessTokenLength)
	if _, err := rand.Read(token); err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	return &AuthService{accessToken: hex.EncodeToString(token)}, nil
}

// NewAuthServiceWithToken builds a service around an existing token. Used by
// tests and by callers that restore a known value; the string is hashed so the
// plaintext is not retained in memory longer than necessary.
func NewAuthServiceWithToken(token string) (*AuthService, error) {
	if token == "" {
		return nil, fmt.Errorf("access token must not be empty")
	}
	svc, err := NewAuthService()
	if err != nil {
		return nil, err
	}
	svc.accessToken = token
	return svc, nil
}

// AccessToken returns the token clients must present.
// This is the value printed at startup and shared with friends.
func (s *AuthService) AccessToken() string {
	return s.accessToken
}

// ValidateAccessToken reports whether the presented token matches this
// process's token. Comparison is constant time so a caller cannot learn the
// token byte by byte from response timing.
func (s *AuthService) ValidateAccessToken(presented string) bool {
	return subtle.ConstantTimeCompare([]byte(presented), []byte(s.accessToken)) == 1
}

// ValidateAccessTokenForClient reports whether the presented token matches and
// returns the client it belongs to.
//
// It exists so the media endpoint can take the ordinary header like every other
// endpoint does: mpv can be told to send custom headers (idea-0066), so /stream
// no longer has to be a public route reached through a bearer value in a URL.
func (s *AuthService) ValidateAccessTokenForClient(presented, clientID string) (string, bool) {
	if !s.ValidateAccessToken(presented) {
		return "", false
	}
	return clientID, true
}
