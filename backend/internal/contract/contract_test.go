// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

// Package contract holds the thin provider check: the fixture in
// `pacts/frontend-backend.json` is replayed against the real backend router,
// and every interaction must produce the status, the Content-Type and the
// shape of the body it records.
//
// This package used to stand on pact-go and its cgo-FFI verifier, which
// go1.27.4 broke at link time (the library could not be linked at all,
// deviation-0169). The fixture is the contract, not the toolchain, so the
// check now reads it directly: no build tag, no cgo, no native library, and
// it runs as an ordinary test on every push. The questions it answers and the
// quality of its answers are unchanged — the client's conformance to the
// fixture is still covered by the scenario tests in the frontend, which start
// the real backend binary and drive it.
//
// What a rule means here. An interaction whose body carries `matchingRules`
// `$.body` with `match: "type"` pins the *kind* of the body (object, array,
// string, number, bool, null) — not its contents. `$.body.field` does the
// same one level down, and `[*]` applies it to every element of an array
// there. Bodies without matching rules are compared exactly. Any rule this
// verifier does not understand fails the test instead of passing silently:
// a new rule in the fixture is a decision, not a detail.
package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/blagovibe/TorrSyncPlayer/backend/internal/api"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/auth"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/buffer"
	"github.com/blagovibe/TorrSyncPlayer/backend/internal/torrent"
)

// tokenPlaceholder is what the fixture carries instead of a real access
// token. The contract pins the *shape* of an authenticated call, not a
// specific token value: the verifier mints a real token per run and
// substitutes it into the request headers before answering.
const tokenPlaceholder = "${PACT_TOKEN}"

type matchingRule struct {
	Match string `json:"match"`
}

type interaction struct {
	Description   string `json:"description"`
	ProviderState string `json:"providerState"`
	Request       struct {
		Method  string            `json:"method"`
		Path    string            `json:"path"`
		Headers map[string]string `json:"headers"`
		Body    json.RawMessage   `json:"body"`
	} `json:"request"`
	Response struct {
		Status        int                     `json:"status"`
		Headers       map[string]string       `json:"headers"`
		Body          json.RawMessage         `json:"body"`
		MatchingRules map[string]matchingRule `json:"matchingRules"`
	} `json:"response"`
}

type fixture struct {
	Interactions []interaction `json:"interactions"`
}

func loadFixture(t *testing.T) fixture {
	t.Helper()

	src := filepath.Join("..", "..", "..", "pacts", "frontend-backend.json")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("fixture not found at %s: %v", src, err)
	}
	var out fixture
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("fixture is not valid JSON and would fail every assertion disguised as a mismatch: %v", err)
	}
	if len(out.Interactions) == 0 {
		t.Fatalf("fixture has no interactions — the check would pass without checking anything")
	}
	for _, it := range out.Interactions {
		if strings.Contains(it.Request.Path, tokenPlaceholder) {
			t.Fatalf("interaction %q carries a token placeholder in the path — substitution happens in headers only", it.Description)
		}
	}
	return out
}

// jsonKind names the JSON type of v, for error messages and for the
// type-matching rules.
func jsonKind(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "number"
	}
}

func decodeJSON(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("response is not valid JSON: %v\n%s", err, raw)
	}
	return v
}

// walkSegments descends obj path segments into v and returns the value found.
// Segments are dot-separated object keys; there is no index syntax in the
// fixture and inventing one would be guessing.
func walkSegments(t *testing.T, path string, v any) any {
	t.Helper()

	if !strings.HasPrefix(path, "$.body") {
		t.Fatalf("unsupported matchingRule path %q — extend the verifier instead of guessing", path)
	}
	rest := strings.TrimPrefix(path, "$.body")
	rest = strings.TrimPrefix(rest, ".")
	if rest == "" {
		return v
	}
	for _, segment := range strings.Split(rest, ".") {
		if segment == "" {
			t.Fatalf("unsupported matchingRule path %q — empty segment", path)
		}
		obj, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("matchingRule path %q descends into a %s, not an object", path, jsonKind(v))
		}
		v = obj[segment]
	}
	return v
}

func resolveToArray(t *testing.T, path string, v any) []any {
	t.Helper()
	v = walkSegments(t, path, v)
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("matchingRule path %q expects an array, got %s", path, jsonKind(v))
	}
	return arr
}

func checkTypeRules(t *testing.T, it interaction, expected, actual json.RawMessage) {
	t.Helper()

	exp := decodeJSON(t, expected)
	act := decodeJSON(t, actual)

	for path, rule := range it.Response.MatchingRules {
		if rule.Match != "type" {
			t.Fatalf("interaction %q: matchingRule %q uses match %q — this verifier understands only \"type\"", it.Description, path, rule.Match)
		}

		switch {
		case path == "$.body":
			if jsonKind(exp) != jsonKind(act) {
				t.Fatalf("interaction %q: body kind %q, expected %q\nexpected: %s\nactual:   %s",
					it.Description, jsonKind(act), jsonKind(exp), expected, actual)
			}
		case strings.HasSuffix(path, "[*]"):
			base := strings.TrimSuffix(path, "[*]")
			expElems := resolveToArray(t, base, exp)
			actElems := resolveToArray(t, base, act)
			for i := range expElems {
				if i >= len(actElems) {
					t.Fatalf("interaction %q: actual array at %q has %d elements, expected at least %d", it.Description, base, len(actElems), len(expElems))
				}
				if jsonKind(expElems[i]) != jsonKind(actElems[i]) {
					t.Fatalf("interaction %q: %s[%d] is %s, expected %s", it.Description, path, i, jsonKind(actElems[i]), jsonKind(expElems[i]))
				}
			}
		default:
			expV := walkSegments(t, path, exp)
			actV := walkSegments(t, path, act)
			if jsonKind(expV) != jsonKind(actV) {
				t.Fatalf("interaction %q: %s is %s, expected %s", it.Description, path, jsonKind(actV), jsonKind(expV))
			}
		}
	}
}

func TestProviderContract(t *testing.T) {
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
	pactToken := authService.AccessToken()

	router := api.NewRouter(api.RouterConfig{
		TorrentSvc:  torrentSvc,
		AuthService: authService,
	})

	// The replay runs back-to-back from 127.0.0.1; the production per-IP
	// limiter would throttle it halfway through and report 429s that say
	// nothing about the contract. Clearing the bucket before each request
	// keeps the replay faithful.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.ResetClientRateLimiter()
		router.ServeHTTP(w, r)
	}))
	defer server.Close()

	contract := loadFixture(t)

	for _, interaction := range contract.Interactions {
		it := interaction
		t.Run(it.Description, func(t *testing.T) {
			hdrs := make(http.Header, len(it.Request.Headers))
			for k, v := range it.Request.Headers {
				sub := strings.ReplaceAll(v, tokenPlaceholder, pactToken)
				if strings.Contains(v, tokenPlaceholder) && sub == v {
					t.Fatalf("interaction %q: token placeholder survived substitution in header %q", it.Description, k)
				}
				hdrs.Set(k, sub)
			}

			req, err := http.NewRequest(it.Request.Method, server.URL+it.Request.Path, bytes.NewReader(it.Request.Body))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			for k, values := range hdrs {
				req.Header[k] = values
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("replay request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != it.Response.Status {
				t.Fatalf("status %d, expected %d", resp.StatusCode, it.Response.Status)
			}
			if want, ok := it.Response.Headers["Content-Type"]; ok {
				got := resp.Header.Get("Content-Type")
				if !strings.Contains(got, want) {
					t.Fatalf("Content-Type %q, expected it to contain %q", got, want)
				}
			}

			raw := readBody(t, resp)
			if len(it.Response.MatchingRules) == 0 {
				exp := decodeJSON(t, it.Response.Body)
				gotJSON := decodeJSON(t, raw)
				if !reflect.DeepEqual(exp, gotJSON) {
					t.Fatalf("body does not match the fixture exactly (no matchingRules for this interaction)\nexpected: %s\nactual:   %s", it.Response.Body, raw)
				}
				return
			}
			checkTypeRules(t, it, it.Response.Body, raw)
		})
	}
}

func readBody(t *testing.T, resp *http.Response) json.RawMessage {
	t.Helper()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return buf.Bytes()
}
