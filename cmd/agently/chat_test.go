package agently

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAttachments_RejectsEmptyPath(t *testing.T) {
	_, err := parseAttachments([]string{""})
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "attachment path is required")
}

func TestParseAttachments_RejectsUnsupportedType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.unknownext")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	_, err := parseAttachments([]string{path})
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "unsupported attachment type")
}

func TestChatCmdExecute_UsesCoreSDKHTTPFlow(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	var authHeaders []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/conversations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"conv-test"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/agent/query":
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			assert.NotEmpty(t, payload)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"conversationId":"conv-test","content":"Hello from core sdk"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd := &ChatCmd{
		API:     srv.URL,
		AgentID: "simple",
		Query:   []string{"Hi"},
		User:    "test-user",
		Token:   "jwt-token",
	}

	err := cmd.Execute(nil)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, requests, "POST /v1/conversations")
	assert.Contains(t, requests, "GET /v1/stream")
	assert.Contains(t, requests, "POST /v1/agent/query")
	assert.Contains(t, authHeaders, "Bearer jwt-token")
}
