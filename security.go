package main

import "net/http"

// csp says where the browser may load things from: only our own site.
// Inline styles stay allowed (the jar height, htmx's own style tag),
// inline scripts don't, so injected HTML can't run code.
const csp = "default-src 'self'; " +
	"img-src 'self' data:; " + // data: for the ⭐ favicon
	"style-src 'self' 'unsafe-inline'; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; " +
	"frame-ancestors 'none'" // no other site may show us in a frame

// securityHeaders adds browser safety headers to every response
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Frame-Options", "DENY")           // frame-ancestors for older browsers
		h.Set("X-Content-Type-Options", "nosniff") // trust our Content-Type, don't guess
		h.Set("Referrer-Policy", "same-origin")    // don't leak our URL to other sites
		next.ServeHTTP(w, r)
	})
}
