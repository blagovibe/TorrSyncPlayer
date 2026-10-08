// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package auth

import (
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
		payload := streamTicketPayload(expiry, clientID, torrentID)
		sig := svc.signStreamTicket(payload)
		old := strconv.FormatInt(expiry, 10) + "." + clientID + "." + torrentID + "." + sig
		if _, ok := svc.ValidateStreamTicket(old, torrentID); ok {
			t.Error("expired ticket was accepted")
		}
	})

	t.Run("non-positive expiry", func(t *testing.T) {
		for _, expiry := range []int64{0, -1, -1 << 40} {
			payload := streamTicketPayload(expiry, clientID, torrentID)
			sig := svc.signStreamTicket(payload)
			tk := strconv.FormatInt(expiry, 10) + "." + clientID + "." + torrentID + "." + sig
			if _, ok := svc.ValidateStreamTicket(tk, torrentID); ok {
				t.Errorf("ticket with expiry %d was accepted", expiry)
			}
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

// TestStreamTicketResistsForgery is the regression test for the vulnerability
// that made the ticket worthless.
//
// The original signature was hex(payload || HMAC(key, "")). Because the MAC was
// computed over the empty string, the digest was a constant for the lifetime of
// the process and the payload travelled next to it in the clear. Anyone holding
// one legitimate ticket could therefore re-cut the payload for a different
// torrent, extend the expiry arbitrarily, re-hex it themselves and splice the
// constant MAC back on — producing a ticket the server accepted.
//
// forgeTicketBelow reproduces exactly that construction. Under the old code
// every case here validated; they must all be rejected now.
func TestStreamTicketResistsForgery(t *testing.T) {
	svc, err := NewAuthService()
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}

	const clientID = "client-abc"
	const ownedTorrent = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const victimTorrent = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	legit, err := svc.GenerateStreamTicket(clientID, ownedTorrent)
	if err != nil {
		t.Fatalf("GenerateStreamTicket: %v", err)
	}
	if _, ok := svc.ValidateStreamTicket(legit, ownedTorrent); !ok {
		t.Fatal("a legitimately issued ticket must validate")
	}
	parts := strings.Split(legit, ".")
	if len(parts) != 4 {
		t.Fatalf("unexpected ticket format %q", legit)
	}

	// Extract the constant digest tail exactly as an attacker would: the last 64
	// hex chars of the signature. Under a correct implementation the signature is
	// nothing but the MAC, so this "tail" is the whole signature and re-splicing
	// it cannot sign anything new — which is precisely what must hold.
	const macHexLen = 2 * sha256.Size
	digestTail := ""
	if len(parts[3]) >= macHexLen {
		digestTail = parts[3][len(parts[3])-macHexLen:]
	}

	forge := func(expiry int64, user, torrent string) string {
		// Recompute the payload prefix the attacker controls, then append the
		// constant digest they lifted from a ticket they legitimately hold.
		payload := streamTicketPayload(expiry, user, torrent)
		return strconv.FormatInt(expiry, 10) + "." + user + "." + torrent + "." +
			hex.EncodeToString([]byte(payload)) + digestTail
	}

	soon := time.Now().Add(constants.StreamTicketTTL).Unix()

	t.Run("re-cut for another torrent", func(t *testing.T) {
		forged := forge(soon, clientID, victimTorrent)
		if _, ok := svc.ValidateStreamTicket(forged, victimTorrent); ok {
			t.Fatal("a ticket re-cut for a torrent never granted was accepted")
		}
	})

	t.Run("re-cut with extended expiry", func(t *testing.T) {
		far := time.Now().Add(365 * 24 * time.Hour).Unix()
		forged := forge(far, clientID, ownedTorrent)
		if _, ok := svc.ValidateStreamTicket(forged, ownedTorrent); ok {
			t.Fatal("a ticket with a forged far-future expiry was accepted")
		}
	})

	t.Run("re-cut for another client id", func(t *testing.T) {
		forged := forge(soon, "someone-else", ownedTorrent)
		if _, ok := svc.ValidateStreamTicket(forged, ownedTorrent); ok {
			t.Fatal("a ticket with a forged client id was accepted")
		}
	})

	t.Run("re-cut for another torrent with extended expiry", func(t *testing.T) {
		far := time.Now().Add(365 * 24 * time.Hour).Unix()
		forged := forge(far, "someone-else", victimTorrent)
		if _, ok := svc.ValidateStreamTicket(forged, victimTorrent); ok {
			t.Fatal("a fully re-cut ticket was accepted")
		}
	})

	// Simpler attacks must fail too: a field swapped while the original
	// signature is carried over verbatim.
	t.Run("field swapped, original signature kept", func(t *testing.T) {
		for _, tc := range []struct{ name, ticket, want string }{
			{"torrent", parts[0] + "." + parts[1] + "." + victimTorrent + "." + parts[3], victimTorrent},
			{"expiry", strconv.FormatInt(time.Now().Add(365*24*time.Hour).Unix(), 10) + "." + parts[1] + "." + parts[2] + "." + parts[3], ownedTorrent},
			{"client", parts[0] + "." + "someone-else" + "." + parts[2] + "." + parts[3], ownedTorrent},
		} {
			if _, ok := svc.ValidateStreamTicket(tc.ticket, tc.want); ok {
				t.Errorf("%s: tampered ticket was accepted", tc.name)
			}
		}
	})

	// The signature must be a fixed-length MAC and nothing else. Under the old
	// code it also carried the hex payload, which is what exposed the constant
	// digest an attacker needed in order to re-sign from scratch.
	t.Run("signature is exactly one MAC", func(t *testing.T) {
		if got := len(parts[3]); got != macHexLen {
			t.Fatalf("signature is %d hex chars, want %d: it must carry the MAC only, never the payload", got, macHexLen)
		}
	})

	// Two tickets for different torrents must never share a signature.
	t.Run("signatures differ per torrent", func(t *testing.T) {
		other, err := svc.GenerateStreamTicket(clientID, victimTorrent)
		if err != nil {
			t.Fatalf("GenerateStreamTicket: %v", err)
		}
		if strings.Split(other, ".")[3] == parts[3] {
			t.Fatal("two tickets for different torrents share a signature")
		}
	})
}

func TestGenerateStreamTicketRejectsDots(t *testing.T) {
	svc, err := NewAuthService()
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	// A dot would make the ticket unparseable, so it must be refused up front
	// rather than minted into something that can never validate.
	for _, tc := range []struct{ user, torrent string }{
		{"client.evil", "abc"},
		{"client", "tor.rent"},
		{"cl.ient", "tor.rent"},
	} {
		if _, err := svc.GenerateStreamTicket(tc.user, tc.torrent); err == nil {
			t.Errorf("GenerateStreamTicket(%q, %q) succeeded, want error", tc.user, tc.torrent)
		}
	}
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
