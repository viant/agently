package http

import (
	"net/http"
)

// WithCORS is a lightweight middleware that adds permissive CORS headers to each
// request and handles CORS pre-flight (OPTIONS) requests automatically.
//
// The intent is to make the agently HTTP API consumable from browser based
// front-ends without requiring an additional proxy. The policy is intentionally
// very permissive – it allows any origin, common HTTP methods and standard
// headers. If stricter rules are required the middleware can be expanded to
// accept a configuration struct but for now the simple default is sufficient.
func WithCORS(next http.Handler) http.Handler {
	if next == nil {
		return nil
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reflect the Origin to support credentialed requests (cookies) from dev hosts
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin, Access-Control-Request-Headers, Access-Control-Request-Method")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			// Fallback for non-CORS requests
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		// Methods and headers
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		reqHeaders := r.Header.Get("Access-Control-Request-Headers")
		if reqHeaders == "" {
			reqHeaders = "Content-Type, Authorization"
		}
		w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		// Optional: cache preflight
		w.Header().Set("Access-Control-Max-Age", "600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
