package workspace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/viant/agently/service"
	"gopkg.in/yaml.v3"
)

// doRequest is a helper that builds an HTTP request, executes it against the
// supplied handler and returns the recorded response.
func doRequest(h http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandler_CRUD(t *testing.T) {
	root := t.TempDir()
	_ = os.Setenv("AGENTLY_ROOT", root)

	// Pre-create workspace directories expected by repositories to avoid
	// "path not found" errors on fs.List.
	for _, dir := range []string{"agents", "models", "workflows", "mcp"} {
		_ = os.MkdirAll(root+"/"+dir, 0o755)
	}

	svc := service.New(nil, service.Options{})
	handler := NewHandler(svc)

	// 1. List on empty workspace – expect []
	rec := doRequest(handler, http.MethodGet, "/v1/workspace/agents", nil)
	assert.EqualValues(t, http.StatusOK, rec.Code)
	var respWrapper struct {
		Status string        `json:"status"`
		Data   []interface{} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &respWrapper))
	assert.EqualValues(t, 0, len(respWrapper.Data))

	// 2. PUT new YAML resource
	yaml := []byte("name: foo\n")
	rec = doRequest(handler, http.MethodPut, "/v1/workspace/models/foo", yaml)
	assert.EqualValues(t, http.StatusOK, rec.Code)

	// 3. GET resource – expect same bytes
	rec = doRequest(handler, http.MethodGet, "/v1/workspace/models/foo", nil)
	assert.EqualValues(t, http.StatusOK, rec.Code)
	// Expect wrapper with raw YAML content inside data field.
	var getResp struct {
		Status string         `json:"status"`
		Data   map[string]any `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &getResp))
	// Re-marshal to YAML to compare.
	actualYAML, _ := yaml.Marshal(getResp.Data)
	assert.EqualValues(t, yaml, actualYAML)

	// 4. List now returns ["foo"]
	rec = doRequest(handler, http.MethodGet, "/v1/workspace/models", nil)
	assert.EqualValues(t, http.StatusOK, rec.Code)
	respWrapper = struct {
		Status string        `json:"status"`
		Data   []interface{} `json:"data"`
	}{}
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &respWrapper))
	// Extract names from items.
	var names []string
	for _, item := range respWrapper.Data {
		if m, ok := item.(map[string]any); ok {
			if name, ok := m["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	assert.EqualValues(t, []string{"foo"}, names)

	// 5. DELETE resource
	rec = doRequest(handler, http.MethodDelete, "/v1/workspace/models/foo", nil)
	assert.EqualValues(t, http.StatusOK, rec.Code)

	// 6. List empty again
	rec = doRequest(handler, http.MethodGet, "/v1/workspace/models", nil)
	assert.EqualValues(t, http.StatusOK, rec.Code)
	respWrapper = struct {
		Status string        `json:"status"`
		Data   []interface{} `json:"data"`
	}{}
	assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &respWrapper))
	assert.EqualValues(t, 0, len(respWrapper.Data))
}
