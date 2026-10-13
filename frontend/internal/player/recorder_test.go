// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package player

import (
	"net/http"
	"net/http/httptest"
	"sync"
)

// headerRecorder answers 401 to every request and remembers what it saw.
//
// 401 rather than media, so the test does not need a decodable stream: what is
// being checked is that the request left mpv carrying the header, not that
// anything played.
type headerRecorder struct {
	*httptest.Server
	mu   sync.Mutex
	seen bool
}

func newHeaderRecorder(t interface{ Cleanup(func()) }) *headerRecorder {
	r := &headerRecorder{}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Access-Token") != "" {
			r.mu.Lock()
			r.seen = true
			r.mu.Unlock()
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *headerRecorder) Seen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen
}
