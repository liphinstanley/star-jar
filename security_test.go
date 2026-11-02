package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hi"))
	})
	handler := securityHeaders(mux)

	// a normal page and a 404 must both get the headers
	for _, path := range []string{"/", "/nope"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))

		for _, name := range []string{"Content-Security-Policy", "X-Frame-Options", "X-Content-Type-Options", "Referrer-Policy"} {
			if rec.Header().Get(name) == "" {
				t.Errorf("%s: missing %s", path, name)
			}
		}
	}
}
