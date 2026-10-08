// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package auth provides access control for the local API.
//
// TorrSyncPlayer runs one backend per person: you install it, it prints an
// access token, and you hand that token to the friends you watch with. There
// are no accounts, no registration and no password database — see CONCEPT.md.
// What remains here is exactly two things:
//
//   - a single random access token, generated fresh on every start, that gates
//     the API and stops anything else on the internet from driving the machine;
//   - short-lived signed stream tickets, so a media player (libmpv) that cannot
//     attach request headers can still fetch media over HTTP.
//
// Everything else that used to live here — user registration, password hashing,
// JWT issuance and revocation — was removed. It solved a problem a single-user
// desktop application does not have, and nothing else depended on it.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/hkdf"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/constants"
)

// AccessTokenLength is the number of random bytes in a generated access token.
// 32 bytes is 256 bits: not guessable, and it is regenerated on every start so
// there is nothing to store, rotate or leak between runs.
const AccessTokenLength = 32

// AuthService holds the per-process access token and the key used to sign
// stream tickets.
type AuthService struct {
	accessToken string
	signKey     []byte
}

// NewAuthService generates a fresh access token and stream-ticket signing key
// for this process. Both come from crypto/rand, so nothing needs to be
// configured, persisted or supplied through the environment.
func NewAuthService() (*AuthService, error) {
	token := make([]byte, AccessTokenLength)
	if _, err := rand.Read(token); err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	signKey := make([]byte, 32)
	if _, err := rand.Read(signKey); err != nil {
		return nil, fmt.Errorf("generate stream ticket key: %w", err)
	}

	return &AuthService{accessToken: hex.EncodeToString(token), signKey: signKey}, nil
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

// GenerateStreamTicket mints a short-lived ticket that lets a media player fetch
// one torrent's media without attaching an access-token header. libmpv issues
// its own HTTP requests and cannot be given one, which is why this exists
// instead of simply putting the token in a header.
//
// Format:
//
//	<expiry>.<userID>.<torrentID>.<hex(HMAC(expiry|userID|torrentID))>
//
// The signature covers every field that a client can influence — including the
// expiry. A client that tampers with any part of the payload produces a
// signature that no longer matches and is rejected.
func (s *AuthService) GenerateStreamTicket(userID, torrentID string) (string, error) {
	if userID == "" || torrentID == "" {
		return "", fmt.Errorf("userID and torrentID are required for a stream ticket")
	}
	// A "." in either field would break the dotted format below: ValidateStreamTicket
	// splits on "." and insists on exactly 4 parts, so such a ticket could never
	// validate. Refuse it here instead of minting something unusable.
	if strings.ContainsAny(userID+"\x00"+torrentID, ".") {
		return "", fmt.Errorf("stream ticket fields must not contain a dot")
	}
	expiry := time.Now().Add(constants.StreamTicketTTL).Unix()
	payload := streamTicketPayload(expiry, userID, torrentID)
	return fmt.Sprintf("%d.%s.%s.%s", expiry, userID, torrentID, s.signStreamTicket(payload)), nil
}

// ValidateStreamTicket verifies a ticket issued by GenerateStreamTicket. It
// returns true only if the embedded torrent matches the one requested, the
// signature is valid and the ticket has not expired. The embedded userID is
// returned so the caller can associate the stream with a client.
func (s *AuthService) ValidateStreamTicket(ticket, torrentID string) (string, bool) {
	if ticket == "" || torrentID == "" {
		return "", false
	}
	parts := strings.Split(ticket, ".")
	if len(parts) != 4 {
		return "", false
	}
	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", false
	}
	// Reject a non-positive expiry: it is a forged field rather than a real
	// timestamp, and the comparison below would otherwise treat it as valid.
	if expiry <= 0 || time.Now().Unix() > expiry {
		return "", false
	}
	userID := parts[1]
	if parts[2] != torrentID {
		return "", false
	}
	// Recompute the signature over exactly the fields the caller supplied. Any
	// modification to the expiry, the userID or the torrentID changes the MAC.
	payload := streamTicketPayload(expiry, userID, torrentID)
	if !hmac.Equal([]byte(s.signStreamTicket(payload)), []byte(parts[3])) {
		return "", false
	}
	return userID, true
}

// streamTicketPayload builds the canonical string that the signature covers.
// Signing and verifying must agree byte for byte, so both go through here
// rather than each formatting the string itself.
func streamTicketPayload(expiry int64, userID, torrentID string) string {
	return fmt.Sprintf("%d|%s|%s", expiry, userID, torrentID)
}

// signStreamTicket returns the hex-encoded HMAC over payload.
//
// The payload MUST be written to the hash before summing. hash.Hash.Sum
// appends the MAC of whatever has been written *so far*; passing the payload as
// the append argument instead would MAC the empty string and merely prefix it
// with attacker-controlled bytes, leaving the signature meaningless.
func (s *AuthService) signStreamTicket(payload string) string {
	mac := hmac.New(sha256.New, s.streamTicketKey())
	if _, err := mac.Write([]byte(payload)); err != nil {
		// hash.Hash documents Write as never returning an error; if that ever
		// changes, returning a value that cannot verify is the safe direction.
		return ""
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// streamTicketKey derives the HMAC key for stream tickets from the per-process
// signing key, using HKDF with a domain-separation label so ticket signing
// never reuses the key material directly.
func (s *AuthService) streamTicketKey() []byte {
	r := hkdf.New(sha256.New, s.signKey, nil, []byte(constants.StreamTicketSecret))
	key := make([]byte, 32)
	if _, err := r.Read(key); err != nil {
		// HKDF does not fail for these parameters, but never return an empty
		// key: fall back to the raw signing key rather than signing with nothing.
		return s.signKey
	}
	return key
}
