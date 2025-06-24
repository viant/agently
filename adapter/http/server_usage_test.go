package http

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/viant/agently/genai/conversation"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/usage"
)

// usageEchoHandler returns a fixed Usage aggregator so that the manager stores it.
func usageEchoHandler(u *usage.Aggregator) conversation.QueryHandler {
	return func(_ context.Context, _ *agentpkg.QueryInput, out *agentpkg.QueryOutput) error {
		out.Content = "ok"
		out.Usage = u
		return nil
	}
}

func TestServer_GetUsage(t *testing.T) {
	// Prepare aggregator with some numbers.
	agg := &usage.Aggregator{}
	agg.Add("gpt-3.5", 10, 2, 1)
	agg.Add("ada-002", 0, 0, 5)

	uStore := memory.NewUsageStore()
	mgr := conversation.New(memory.NewHistoryStore(), nil, usageEchoHandler(agg), conversation.WithUsageStore(uStore))

	// Trigger one Accept call so that usage gets recorded.
	_, _ = mgr.Accept(context.Background(), &agentpkg.QueryInput{ConversationID: "conv1", AgentName: "dummy"})

	// Start HTTP server
	ln, errLn := net.Listen("tcp4", "127.0.0.1:0")
	if errLn != nil {
		t.Skip("cannot allocate listener")
	}
	srv := httptest.NewUnstartedServer(NewServer(mgr))
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/v1/api/conversations/conv1/usage")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.EqualValues(t, http.StatusOK, resp.StatusCode)

	var wrapper struct {
		Status string                 `json:"status"`
		Data   map[string]interface{} `json:"data"`
	}
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&wrapper))

	data := wrapper.Data
	assert.EqualValues(t, float64(10), data["inputTokens"]) // JSON numbers decoded as float64
	assert.EqualValues(t, float64(2), data["outputTokens"])
	assert.EqualValues(t, float64(6), data["embeddingTokens"])

	// Validate perModel map presence
	perModel, ok := data["perModel"].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, perModel, "gpt-3.5")
	assert.Contains(t, perModel, "ada-002")
}
