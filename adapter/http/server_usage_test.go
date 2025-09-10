package http

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	agentpkg "github.com/viant/agently/genai/service/agent"

	"github.com/viant/agently/genai/conversation"
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
	agg.Add("gpt-3.5", 10, 2, 1, 0)
	agg.Add("ada-002", 0, 0, 5, 0)

	uStore := memory.NewUsageStore()
	mgr := conversation.New(memory.NewHistoryStore(), nil, usageEchoHandler(agg), conversation.WithUsageStore(uStore))

	// Trigger one Accept call so that usage gets recorded.
	_, _ = mgr.Accept(context.Background(), &agentpkg.QueryInput{ConversationID: "conv1", AgentName: "dummy"})

	// StartedAt HTTP server
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

	var usageResp UsageResponse
	assert.NoError(t, json.NewDecoder(resp.Body).Decode(&usageResp))

	if assert.Len(t, usageResp.Data, 1) {
		data := usageResp.Data[0]
		assert.EqualValues(t, "conv1", data.ConversationID)
		assert.EqualValues(t, 10, data.InputTokens)
		assert.EqualValues(t, 2, data.OutputTokens)
		assert.EqualValues(t, 6, data.EmbeddingTokens)

		// Validate perModel entries
		assert.Len(t, data.PerModel, 2)
		names := []string{data.PerModel[0].Model, data.PerModel[1].Model}
		assert.Contains(t, names, "gpt-3.5")
		assert.Contains(t, names, "ada-002")
	}
}
