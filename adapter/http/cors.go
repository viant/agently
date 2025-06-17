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
		// Always set the CORS headers so that they are present on both the
		// pre-flight response and the actual response.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle pre-flight request directly.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
