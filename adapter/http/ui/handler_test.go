package ui

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/metadata"
	"github.com/viant/forge/backend/types"
	"gopkg.in/yaml.v3"
)

// TestHandler_end_to_end covers both /navigation and /window endpoints using the
// embedded Forge metadata. It validates that the HTTP layer correctly delegates
// to forge/backend handlers and that baseURL rooting works as expected.
func TestHandler_end_to_end(t *testing.T) {
	root := "embed://localhost/"
	// Build handler using the same metadata FS as production code.
	handler := NewEmbeddedHandler(root, &metadata.FS)

	cases := []struct {
		name           string
		url            string
		wantStatusCode int
		wantBodyObj    any // pointer to struct slice that will hold expected data
		loadExpected   func() (any, error)
	}{
		{
			name:           "navigation",
			url:            "/navigation",
			wantStatusCode: 200,
			loadExpected: func() (any, error) {
				var out []types.NavigationItem
				data, err := metadata.FS.ReadFile("navigation.yaml")
				if err != nil {
					return nil, err
				}
				if err := yaml.Unmarshal(data, &out); err != nil {
					return nil, err
				}
				return out, nil
			},
		},
		{
			name:           "window chat",
			url:            "/window/chat",
			wantStatusCode: 200,
			loadExpected: func() (any, error) {
				var out types.Window
				data, err := metadata.FS.ReadFile("window/chat/main.yaml")
				if err != nil {
					return nil, err
				}
				if err := yaml.Unmarshal(data, &out); err != nil {
					return nil, err
				}
				return &out, nil
			},
		},
	}

	for _, tc := range cases {
		gotExp, err := tc.loadExpected()
		assert.NoError(t, err, tc.name)

		req := httptest.NewRequest("GET", tc.url, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.EqualValues(t, tc.wantStatusCode, rr.Code, tc.name)

		// Decode actual JSON into the same Go value type.
		var got any
		switch gotExp.(type) {
		case []types.NavigationItem:
			var v []types.NavigationItem
			assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &v), tc.name)
			got = v
		case *types.Window:
			var v types.Window
			assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &v), tc.name)
			got = &v
		default:
			t.Fatalf("unsupported expected type %T", gotExp)
		}

		assert.EqualValues(t, gotExp, got, tc.name)
	}
}
