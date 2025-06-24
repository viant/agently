package conversation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/usage"
)

// stubHandler returns a fixed usage aggregator so that we can verify that the
// Manager persists the counts into the supplied UsageStore.
func handlerWithUsage(u *usage.Aggregator) QueryHandler {
	return func(ctx context.Context, in *agentpkg.QueryInput, out *agentpkg.QueryOutput) error {
		out.Content = "ok"
		out.Usage = u
		return nil
	}
}

func TestManager_UsageStoreIntegration(t *testing.T) {
	// Prepare aggregator with two models.
	agg := &usage.Aggregator{}
	agg.Add("gpt-3.5", 100, 20, 5)
	agg.Add("ada-002", 0, 0, 50)

	store := memory.NewUsageStore()
	mgr := New(memory.NewHistoryStore(), nil, handlerWithUsage(agg), WithUsageStore(store))

	in := &agentpkg.QueryInput{ConversationID: "conv1", AgentName: "dummy", Query: "hi"}
	_, err := mgr.Accept(context.Background(), in)
	assert.NoError(t, err)

	// Totals should match aggregator sums.
	p, c, e := store.Totals("conv1")
	assert.EqualValues(t, 100, p)
	assert.EqualValues(t, 20, c)
	assert.EqualValues(t, 55, e) // 5 + 50

	aggConv := store.Aggregator("conv1")
	assert.NotNil(t, aggConv)
	assert.ElementsMatch(t, []string{"gpt-3.5", "ada-002"}, aggConv.Keys())
}
