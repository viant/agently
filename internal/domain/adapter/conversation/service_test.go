package conversation

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	convmem "github.com/viant/agently/internal/dao/conversation/impl/memory"
	convread "github.com/viant/agently/internal/dao/conversation/read"
	convwrite "github.com/viant/agently/internal/dao/conversation/write"
)

// Helper to marshal selected fields for stable comparison.
func toGraph(rows []*convread.ConversationView) []map[string]interface{} {
	var out []map[string]interface{}
	for _, v := range rows {
		row := map[string]interface{}{"id": v.Id}
		if v.Summary != nil {
			row["summary"] = *v.Summary
		}
		if v.AgentName != nil {
			row["agentName"] = *v.AgentName
		}
		if v.UsageInputTokens != nil {
			row["in"] = *v.UsageInputTokens
		}
		if v.UsageOutputTokens != nil {
			row["out"] = *v.UsageOutputTokens
		}
		if v.UsageEmbeddingTokens != nil {
			row["emb"] = *v.UsageEmbeddingTokens
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	return out
}

func TestService_DataDriven(t *testing.T) {
	type testCase struct {
		name   string
		seed   []*convwrite.Conversation
		list   []convread.ConversationInputOption
		expect string
	}

	cases := []testCase{
		{
			name: "patch and list all",
			seed: []*convwrite.Conversation{
				func() *convwrite.Conversation {
					w := &convwrite.Conversation{Id: "c1", Has: &convwrite.ConversationHas{Id: true}}
					w.SetSummary("alpha")
					w.SetAgentName("coder")
					return w
				}(),
				func() *convwrite.Conversation {
					w := &convwrite.Conversation{Id: "c2", Has: &convwrite.ConversationHas{Id: true}}
					w.SetSummary("beta")
					return w
				}(),
			},
			list:   nil,
			expect: `[{"agentName":"coder","id":"c1","summary":"alpha"},{"id":"c2","summary":"beta"}]`,
		},
		{
			name: "filter by id",
			seed: []*convwrite.Conversation{
				func() *convwrite.Conversation {
					w := &convwrite.Conversation{Id: "c3", Has: &convwrite.ConversationHas{Id: true}}
					w.SetSummary("gamma")
					return w
				}(),
			},
			list:   []convread.ConversationInputOption{convread.WithID("c3")},
			expect: `[{"id":"c3","summary":"gamma"}]`,
		},
		{
			name: "filter by summary contains",
			seed: []*convwrite.Conversation{
				func() *convwrite.Conversation {
					w := &convwrite.Conversation{Id: "c4", Has: &convwrite.ConversationHas{Id: true}}
					w.SetSummary("hello world")
					return w
				}(),
				func() *convwrite.Conversation {
					w := &convwrite.Conversation{Id: "c5", Has: &convwrite.ConversationHas{Id: true}}
					w.SetSummary("bye!")
					return w
				}(),
			},
			list:   []convread.ConversationInputOption{convread.WithSummaryContains("world")},
			expect: `[{"id":"c4","summary":"hello world"}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			convDAO := convmem.New()
			svc := New(convDAO)

			if len(tc.seed) > 0 {
				if _, err := svc.Patch(ctx, tc.seed...); !assert.NoError(t, err) {
					return
				}
			}

			rows, err := svc.List(ctx, tc.list...)
			if !assert.NoError(t, err) {
				return
			}
			gotJSON, _ := json.Marshal(toGraph(rows))
			var gotSlice []map[string]interface{}
			_ = json.Unmarshal(gotJSON, &gotSlice)
			var expSlice []map[string]interface{}
			_ = json.Unmarshal([]byte(tc.expect), &expSlice)
			assert.EqualValues(t, expSlice, gotSlice)
		})
	}
}
