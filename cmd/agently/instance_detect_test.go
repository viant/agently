package agently

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchWorkspaceMetadata_FlatPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspace/metadata" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{
			"workspaceRoot":"/tmp/agently",
			"defaults":{"agent":"coder","model":"openai_gpt-5.4"},
			"models":["openai_gpt-5.4","openai_gpt-5.2"]
		}`))
	}))
	defer server.Close()

	meta, ok := fetchWorkspaceMetadata(context.Background(), server.URL)
	require.True(t, ok)
	require.NotNil(t, meta)
	assert.Equal(t, "/tmp/agently", meta.WorkspaceRoot)
	assert.Equal(t, "coder", meta.DefaultAgent)
	assert.Equal(t, "openai_gpt-5.4", meta.DefaultModel)
	assert.Equal(t, []string{"openai_gpt-5.4", "openai_gpt-5.2"}, meta.Models)
}

func TestFetchWorkspaceMetadata_WrappedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workspace/metadata" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{
			"status":"ok",
			"data":{
				"workspaceRoot":"/tmp/agently",
				"defaults":{"agent":"chatter","model":"openai_gpt-5.2"},
				"models":["openai_gpt-5.2"]
			}
		}`))
	}))
	defer server.Close()

	meta, ok := fetchWorkspaceMetadata(context.Background(), server.URL)
	require.True(t, ok)
	require.NotNil(t, meta)
	assert.Equal(t, "/tmp/agently", meta.WorkspaceRoot)
	assert.Equal(t, "chatter", meta.DefaultAgent)
	assert.Equal(t, "openai_gpt-5.2", meta.DefaultModel)
	assert.Equal(t, []string{"openai_gpt-5.2"}, meta.Models)
}

func TestIsAgentlyServeProcess(t *testing.T) {
	assert.True(t, isAgentlyServeProcess([]string{"/tmp/go-build/exe/agently", "serve", "-a:9393"}))
	assert.True(t, isAgentlyServeProcess([]string{"go", "run", "../agently", "serve", "-a:9393"}))
	assert.False(t, isAgentlyServeProcess([]string{"/tmp/agently-app", "serve", "--addr", ":9001"}))
	assert.False(t, isAgentlyServeProcess([]string{"/tmp/go-build/exe/agently", "query", "-a", "coder"}))
}
