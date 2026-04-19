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

func TestSplitPIDAndArgs(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		wantPID int
		wantCmd string
		wantOK  bool
	}{
		{"typical", "  1234 /usr/bin/agently serve -a:9393", 1234, "/usr/bin/agently serve -a:9393", true},
		{"tabs", "\t42\t/bin/agently\tserve", 42, "/bin/agently\tserve", true},
		{"empty line", "", 0, "", false},
		{"non-numeric", "abc /bin/agently serve", 0, "", false},
		{"zero pid", "0 /bin/agently serve", 0, "", false},
		{"negative pid", "-1 /bin/agently", 0, "", false},
		{"pid only", "1234", 1234, "", true},
	}
	for _, tc := range cases {
		pid, rest, ok := splitPIDAndArgs(tc.line)
		assert.Equalf(t, tc.wantOK, ok, tc.name)
		assert.Equalf(t, tc.wantPID, pid, tc.name)
		assert.Equalf(t, tc.wantCmd, rest, tc.name)
	}
}

func TestTokenizeCommand(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"simple", "/bin/agently serve -a:9393", []string{"/bin/agently", "serve", "-a:9393"}},
		{"double-quoted path with space", `"/Users/a b/agently" serve`, []string{"/Users/a b/agently", "serve"}},
		{"single-quoted path", `'/opt/my agent/agently' serve --addr :9001`, []string{"/opt/my agent/agently", "serve", "--addr", ":9001"}},
		{"collapsed whitespace", "a   b\t\tc", []string{"a", "b", "c"}},
		{"empty", "", nil},
		{"only spaces", "   \t ", nil},
		{"unterminated quote", `/bin/agently "serve --addr :9001`, []string{"/bin/agently", "serve --addr :9001"}},
	}
	for _, tc := range cases {
		got := tokenizeCommand(tc.in)
		assert.Equalf(t, tc.want, got, tc.name)
	}
}

func TestProcessesFromPS_QuotedPath(t *testing.T) {
	// End-to-end check of the parsing pipeline with a synthetic ps line:
	// quoted executable path containing a space should still be detected.
	line := `  9876 "/Users/a b/bin/agently" serve -a:9393`
	pid, rest, ok := splitPIDAndArgs(line)
	require.True(t, ok)
	assert.Equal(t, 9876, pid)
	args := tokenizeCommand(rest)
	assert.Equal(t, []string{"/Users/a b/bin/agently", "serve", "-a:9393"}, args)
	assert.True(t, isAgentlyServeProcess(args))
	assert.Equal(t, 9393, parsePortFromArgs(args))
}
