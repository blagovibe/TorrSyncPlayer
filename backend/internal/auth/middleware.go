// Package auth provides access control middleware.
package auth

import (
	"context"
	"fmt"
	"net/http"
)

// HeaderAccessToken carries the per-process access token on every API call.
const HeaderAccessToken = "X-Access-Token"

// HeaderClientID identifies the calling client for the lifetime of its run.
// It is not an account: nothing is stored against it, and it only ever
// distinguishes one connected player from another within a room.
const HeaderClientID = "X-Client-ID"

type contextKey string

const clientIDKey contextKey = "torrsyncplayer.clientID"

// TokenMiddleware rejects requests that do not present this process's access
// token, and records the caller's client id for the P2P session.
//
// The token is compared in constant time. A missing or wrong token is answered
// with 401 and no hint about which part was wrong.
//
// No CSRF middleware is needed alongside this: a cross-origin page cannot read
// the token, and cannot set a custom header on the request without a CORS
// preflight that the server does not approve. That was already true of the
// previous JWT scheme and is why CSRF was only ever applied to cookie requests.
func (s *AuthService) TokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented := r.Header.Get(HeaderAccessToken)
		if presented == "" || !s.ValidateAccessToken(presented) {
			w.Header().Set("WWW-Authenticate", HeaderAccessToken)
			writeAuthError(w, http.StatusUnauthorized, "invalid or missing access token")
			return
		}

		ctx := context.WithValue(r.Context(), clientIDKey, r.Header.Get(HeaderClientID))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetClientID returns the client id recorded by TokenMiddleware, or an empty
// string when the request did not pass through it.
func GetClientID(r *http.Request) string {
	id, _ := r.Context().Value(clientIDKey).(string)
	return id
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, message)
}
