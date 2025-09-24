package router_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/adapter/http/workspace"
	"github.com/viant/agently/cmd/service"
	"github.com/viant/mcp"
)

// response is a minimal shape of workspace handler JSON envelope
type response struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data"`
}

func withTempWorkspace(t *testing.T) (root string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	// Ensure the workspace directories exist
	_ = os.MkdirAll(filepath.Join(dir, "mcp"), 0o755)
	// Point workspace to temp dir to avoid polluting user env
	old := os.Getenv("AGENTLY_ROOT")
	_ = os.Setenv("AGENTLY_ROOT", dir)
	return dir, func() { _ = os.Setenv("AGENTLY_ROOT", old) }
}

func Test_WorkspaceAlias_MCP(t *testing.T) {
	// Arrange workspace and pre-populate one MCP server
	_, restore := withTempWorkspace(t)
	defer restore()

	svc := service.New(nil, service.Options{})
	// Save a minimal MCP client definition
	ctx := context.Background()
	err := svc.MCPRepo().Save(ctx, "local", &mcp.ClientOptions{Name: "local"})
	assert.EqualValues(t, nil, err)

	handler := workspace.NewHandler(svc)
	mux := http.NewServeMux()
	mux.Handle("/v1/workspace/", handler)
	mux.Handle("/v1/api/workspace/", http.StripPrefix("/v1/api/", handler))

	type testCase struct {
		name       string
		path       string
		wantStatus int
		// contains is a substring to look for in the JSON-encoded response body
		contains string
	}

	cases := []testCase{
		{
			name:       "list canonical",
			path:       "/v1/workspace/mcp",
			wantStatus: http.StatusOK,
			contains:   "local",
		},
		{
			name:       "list alias",
			path:       "/v1/api/workspace/mcp",
			wantStatus: http.StatusOK,
			contains:   "local",
		},
		{
			name:       "get canonical item",
			path:       "/v1/workspace/mcp/local",
			wantStatus: http.StatusOK,
			contains:   "local",
		},
		{
			name:       "get alias item",
			path:       "/v1/api/workspace/mcp/local",
			wantStatus: http.StatusOK,
			contains:   "local",
		},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		assert.EqualValues(t, tc.wantStatus, rr.Code, tc.name)
		// Quick JSON parse to ensure envelope shape, then verify substring
		var got response
		_ = json.Unmarshal(rr.Body.Bytes(), &got)
		// Response should always include status
		assert.EqualValues(t, "ok", got.Status, tc.name)
		assert.EqualValues(t, true, string(rr.Body.Bytes()) != "", tc.name)
		assert.EqualValues(t, true, strings.Contains(rr.Body.String(), tc.contains), tc.name)
	}
}
