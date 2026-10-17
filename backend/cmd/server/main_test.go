// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package main

import (
	"flag"
	"io"
	"strings"
	"testing"
)

// newTestFlagSet builds a flag set that swallows its own output and usage text,
// so a failing parse cannot spam the test log or call os.Exit.
func newTestFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("torrsyncplayer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// TestParseFlagsAddrOverridesPort pins the contract from DEV.md, "Флаги":
// --addr replaces the derived ":<port>" address outright. Without it the
// address stays ":<port>", which is how the server has always listened.
func TestParseFlagsAddrOverridesPort(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "addr wins over port",
			args: []string{"--addr", "127.0.0.1:9999", "--port", "8889"},
			want: "127.0.0.1:9999",
		},
		{
			name: "addr may carry only a port",
			args: []string{"--addr", ":9999"},
			want: ":9999",
		},
		{
			name: "no addr keeps the port-derived address",
			args: []string{"--port", "8889"},
			want: ":8889",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseFlags(newTestFlagSet(), tt.args)
			if err != nil {
				t.Fatalf("parseFlags(%v) returned error: %v", tt.args, err)
			}
			if cfg.ListenAddr != tt.want {
				t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, tt.want)
			}
		})
	}
}

// TestParseFlagsToken pins the other half of the same DEV.md contract: --token
// fixes the access token instead of taking the random one generated at startup.
func TestParseFlagsToken(t *testing.T) {
	cfg, err := parseFlags(newTestFlagSet(), []string{"--token", "fixed-token-value"})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if cfg.AccessToken != "fixed-token-value" {
		t.Errorf("AccessToken = %q, want %q", cfg.AccessToken, "fixed-token-value")
	}
}

// TestParseFlagsRejectsUnknownFlag keeps a typo from silently starting a server
// with defaults nobody asked for.
func TestParseFlagsRejectsUnknownFlag(t *testing.T) {
	//nolint:misspell // the misspelling is the point of the test
	if _, err := parseFlags(newTestFlagSet(), []string{"--addres", "127.0.0.1:1"}); err == nil {
		t.Fatal("parseFlags accepted an unknown flag, want error")
	}
}

// TestNewAuthServiceUsesFixedToken is the other end of the --token contract: the
// token that reaches the auth service is the one the operator passed, not a
// freshly generated one nobody downstream knows about.
func TestNewAuthServiceUsesFixedToken(t *testing.T) {
	svc, err := newAuthService("operator-chosen-token")
	if err != nil {
		t.Fatalf("newAuthService returned error: %v", err)
	}
	if !svc.ValidateAccessToken("operator-chosen-token") {
		t.Error("fixed token was rejected; --token does not reach the auth service")
	}
}

// TestNewAuthServiceGeneratesWhenEmpty guards the default that the goal
// promises: no --token means a random token, and two services never share it.
func TestNewAuthServiceGeneratesWhenEmpty(t *testing.T) {
	first, err := newAuthService("")
	if err != nil {
		t.Fatalf("newAuthService returned error: %v", err)
	}
	second, err := newAuthService("")
	if err != nil {
		t.Fatalf("newAuthService returned error: %v", err)
	}
	if first.AccessToken() == second.AccessToken() {
		t.Error("two services share an access token; the default is not random")
	}
}

// ── gateway (remoting) flags, idea-0023 ───────────────────────────────────

// A gateway URL with no credentials would silently fall back to an unauthenticated
// request that the gateway rejects only later, at play time.
func TestValidateConfigGatewayRequiresCredentials(t *testing.T) {
	base := Config{
		Port:       "8889",
		ListenAddr: ":8889",
		GatewayURL: "https://gateway.example.com",
	}

	if err := validateConfig(base); err == nil {
		t.Fatal("gateway URL without credentials was accepted")
	}

	withUser := base
	withUser.GatewayUser = "user"
	if err := validateConfig(withUser); err == nil {
		t.Fatal("gateway URL with only a username was accepted")
	}

	complete := withUser
	complete.GatewayPass = "pass"
	if err := validateConfig(complete); err != nil {
		t.Fatalf("complete gateway config rejected: %v", err)
	}
}

// A pinned CA that is not there must fail at startup, not on the first stream.
func TestValidateConfigGatewayRejectsMissingCACert(t *testing.T) {
	cfg := Config{
		Port:          "8889",
		ListenAddr:    ":8889",
		GatewayURL:    "https://gateway.example.com",
		GatewayUser:   "user",
		GatewayPass:   "pass",
		GatewayCACert: "/nonexistent/path/to/ca.pem",
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("missing gateway CA cert was accepted")
	}
}

// Without --gateway-url the server must stay on the in-process client, so an
// unset gateway is the default and not an error.
func TestParseFlagsGatewayDefaultsToLocal(t *testing.T) {
	cfg, err := parseFlags(newTestFlagSet(), nil)
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if cfg.GatewayURL != "" {
		t.Errorf("GatewayURL = %q, want empty (local mode by default)", cfg.GatewayURL)
	}
}

func TestParseFlagsGateway(t *testing.T) {
	cfg, err := parseFlags(newTestFlagSet(), []string{
		"--gateway-url", "https://gateway.example.com",
		"--gateway-user", "user",
		"--gateway-pass", "pass",
		"--gateway-ca-cert", "/etc/ssl/gateway.pem",
	})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if cfg.GatewayURL != "https://gateway.example.com" {
		t.Errorf("GatewayURL = %q", cfg.GatewayURL)
	}
	if cfg.GatewayUser != "user" || cfg.GatewayPass != "pass" {
		t.Errorf("credentials = %q/%q", cfg.GatewayUser, cfg.GatewayPass)
	}
	if cfg.GatewayCACert != "/etc/ssl/gateway.pem" {
		t.Errorf("GatewayCACert = %q", cfg.GatewayCACert)
	}
}

// ── --token-stdin, deviation-0061 ─────────────────────────────────────────

// TestParseFlagsTokenStdin pins the flag itself: it asks for the token to be
// read from standard input rather than from the command line, so the secret
// never appears in the process list.
func TestParseFlagsTokenStdin(t *testing.T) {
	cfg, err := parseFlags(newTestFlagSet(), []string{"--token-stdin"})
	if err != nil {
		t.Fatalf("parseFlags returned error: %v", err)
	}
	if !cfg.AccessTokenStdin {
		t.Error("--token-stdin did not reach the config")
	}
	if cfg.AccessToken != "" {
		t.Errorf("AccessToken = %q with no token given, want empty", cfg.AccessToken)
	}
}

// TestValidateConfigRejectsBothTokenSources covers the ambiguous invocation:
// two token sources at once. Honouring one silently would start the server
// with a secret the operator did not choose, which is exactly the kind of
// confusion the startup check exists for.
func TestValidateConfigRejectsBothTokenSources(t *testing.T) {
	cfg := Config{
		Port:             "8889",
		ListenAddr:       ":8889",
		AccessToken:      "fixed",
		AccessTokenStdin: true,
	}
	if err := validateConfig(cfg); err == nil {
		t.Fatal("--token together with --token-stdin was accepted")
	}
}

// TestReadStdinTokenTakesTheFirstLine is the transport: the parent writes the
// secret and a newline; whatever follows is not the token.
func TestReadStdinTokenTakesTheFirstLine(t *testing.T) {
	token, err := readStdinToken(strings.NewReader("secret-value\nleftover bytes\n"))
	if err != nil {
		t.Fatalf("readStdinToken returned error: %v", err)
	}
	if token != "secret-value" {
		t.Errorf("token = %q, want %q", token, "secret-value")
	}
}

// TestReadStdinTokenTrimsWhitespace keeps a stray \r from a Windows parent
// from becoming part of the secret: it would compare unequal on every request
// and look like a wrong token rather than a transport detail.
func TestReadStdinTokenTrimsWhitespace(t *testing.T) {
	token, err := readStdinToken(strings.NewReader("  secret-value  \r\n"))
	if err != nil {
		t.Fatalf("readStdinToken returned error: %v", err)
	}
	if token != "secret-value" {
		t.Errorf("token = %q, want %q", token, "secret-value")
	}
}

// TestReadStdinTokenRefusesAnEmptySecret: an empty token would pin the server
// to an empty secret, which is the failure the whole token-pinning change was
// about. Refusing at startup says so; accepting it would not.
func TestReadStdinTokenRefusesAnEmptySecret(t *testing.T) {
	for _, input := range []string{"", "\n", "   \n", "\r\n"} {
		if _, err := readStdinToken(strings.NewReader(input)); err == nil {
			t.Errorf("input %q was accepted as a token", input)
		}
	}
}

// The first run of the desktop application greets the owner with this log
// line, so it must not sound like a verdict on what they are doing: the
// backend listens on loopback by default and every byte on it is their own.
// The warning belongs to someone who put the server on a network.
func TestTheTLSWarningOnlyFiresOffLoopback(t *testing.T) {
	for _, tc := range []struct {
		addr string
		warn bool
	}{
		{"127.0.0.1:8889", false},
		{"localhost:8889", false},
		{"[::1]:8889", false},
		{":8889", true},             // every interface
		{"0.0.0.0:8889", true},      // every interface, named
		{"192.168.1.10:8889", true}, // a network
	} {
		if got := isLoopbackAddr(tc.addr); got == tc.warn {
			t.Errorf("%q: loopback = %v, want %v", tc.addr, got, !tc.warn)
		}
	}
}
