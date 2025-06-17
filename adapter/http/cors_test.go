package http

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWithCORS validates that the middleware adds the expected CORS headers
// and that it properly handles pre-flight OPTIONS requests while still
// forwarding regular requests to the wrapped handler.
func TestWithCORS(t *testing.T) {
	var calls int32

	// stub handler increments an atomic counter so that we can verify whether
	// it was executed.
	stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNoContent)
	})

	wrapped := WithCORS(stub)

	testCases := []struct {
		name           string
		method         string
		expectedStatus int
		expectedCalls  int32
	}{
		{
			name:           "preflight OPTIONS returns 200 and does not call next",
			method:         http.MethodOptions,
			expectedStatus: http.StatusOK,
			expectedCalls:  0,
		},
		{
			name:           "regular GET passes through to next handler",
			method:         http.MethodGet,
			expectedStatus: http.StatusNoContent,
			expectedCalls:  1,
		},
	}

	for _, tc := range testCases {
		// reset call counter between iterations
		atomic.StoreInt32(&calls, 0)

		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/", nil)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			res := rec.Result()

			// Validate status code.
			assert.EqualValues(t, tc.expectedStatus, res.StatusCode)

			// Validate CORS headers are present.
			assert.EqualValues(t, "*", res.Header.Get("Access-Control-Allow-Origin"))
			assert.NotEmpty(t, res.Header.Get("Access-Control-Allow-Methods"))
			assert.NotEmpty(t, res.Header.Get("Access-Control-Allow-Headers"))

			// Validate call count matches expectation.
			assert.EqualValues(t, tc.expectedCalls, atomic.LoadInt32(&calls))
		})
	}
}
