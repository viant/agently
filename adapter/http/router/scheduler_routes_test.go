package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test that scheduler routes are registered once with a shared helper and
// dispatch to the provided handler. Uses a simple data-driven table.
func Test_registerSchedulerRoutes(t *testing.T) {
	mux := http.NewServeMux()

	// dummy handler that always returns 204 and a header for identification
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Handler", "scheduler")
		w.WriteHeader(http.StatusNoContent)
	})

	registerSchedulerRoutes(mux, h)

	type testCase struct {
		name       string
		path       string
		wantStatus int
		wantHeader string
	}

	cases := []testCase{
		{name: "root with slash", path: "/v1/api/agently/scheduler/", wantStatus: http.StatusNoContent, wantHeader: "scheduler"},
		{name: "schedule base", path: "/v1/api/agently/schedule", wantStatus: http.StatusNoContent, wantHeader: "scheduler"},
		{name: "schedule run", path: "/v1/api/agently/schedule-run", wantStatus: http.StatusNoContent, wantHeader: "scheduler"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.EqualValues(t, tc.wantStatus, rr.Code, tc.name)
		assert.EqualValues(t, tc.wantHeader, rr.Header().Get("X-Handler"), tc.name)
	}
}
