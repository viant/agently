package toolhandler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/cmd/service"
)

// TestServeHTTP validates path handling for various tool names. It relies on
// a zero-value *service.Service, which is sufficient because the handler
// short-circuits when the underlying executor is nil.
func TestServeHTTP(t *testing.T) {
	type testCase struct {
		name           string
		path           string
		method         string
		body           interface{}
		expectedStatus int
		expectedJSON   map[string]interface{}
	}

	cases := []testCase{
		{
			name:           "simple tool name",
			path:           "/printer:print",
			method:         http.MethodPost,
			body:           map[string]interface{}{"message": "hi"},
			expectedStatus: http.StatusOK,
			expectedJSON:   map[string]interface{}{"status": "ok"},
		},
		{
			name:           "nested tool name",
			path:           "/sql/query:select",
			method:         http.MethodPost,
			body:           map[string]interface{}{"sql": "SELECT 1"},
			expectedStatus: http.StatusOK,
			expectedJSON:   map[string]interface{}{"status": "ok"},
		},
		{
			name:           "missing tool name",
			path:           "/",
			method:         http.MethodPost,
			body:           map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			expectedJSON:   map[string]interface{}{"status": "ERROR"},
		},
		{
			name:           "wrong http method",
			path:           "/printer:print",
			method:         http.MethodGet,
			body:           nil,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedJSON:   map[string]interface{}{"status": "ERROR"},
		},
	}

	h := New(&service.Service{})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var bodyBytes []byte
			if tc.body != nil {
				bodyBytes, _ = json.Marshal(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(bodyBytes))
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			assert.EqualValues(t, tc.expectedStatus, rr.Code)

			// Basic JSON sanity check – decode and compare status field only.
			var got map[string]interface{}
			_ = json.Unmarshal(rr.Body.Bytes(), &got)
			assert.EqualValues(t, tc.expectedJSON["status"], got["status"])
		})
	}
}
