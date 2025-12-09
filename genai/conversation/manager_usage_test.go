package conversation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	agentpkg "github.com/viant/agently/genai/service/agent"

	"github.com/viant/agently/genai/usage"
)

// handlerWithUsage returns a fixed usage aggregator so that we can verify that the
// Manager propagates usage from the handler into the returned output.
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
	agg.Add("gpt-3.5", 100, 20, 5, 0)
	agg.Add("ada-002", 0, 0, 50, 0)

	mgr := New(handlerWithUsage(agg))

	in := &agentpkg.QueryInput{ConversationID: "conv1", AgentID: "dummy", Query: "hi"}
	out, err := mgr.Accept(context.Background(), in)
	assert.NoError(t, err)

	if assert.NotNil(t, out) && assert.NotNil(t, out.Usage) {
		// Totals should match aggregator sums returned by the handler.
		p, c, e, cached := out.Usage.Totals()
		assert.EqualValues(t, 100, p)
		assert.EqualValues(t, 20, c)
		assert.EqualValues(t, 55, e) // 5 + 50
		assert.EqualValues(t, 0, cached)

		assert.ElementsMatch(t, []string{"gpt-3.5", "ada-002"}, out.Usage.Keys())
	}
}
