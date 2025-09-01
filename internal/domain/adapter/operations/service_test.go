package operations

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	daofactory "github.com/viant/agently/internal/dao/factory"
	msgmem "github.com/viant/agently/internal/dao/message/impl/memory"
	msgw "github.com/viant/agently/internal/dao/message/write"
	mcmem "github.com/viant/agently/internal/dao/modelcall/impl/memory"
	mcread "github.com/viant/agently/internal/dao/modelcall/read"
	mcwrite "github.com/viant/agently/internal/dao/modelcall/write"
	plmem "github.com/viant/agently/internal/dao/payload/impl/memory"
	tcmem "github.com/viant/agently/internal/dao/toolcall/impl/memory"
	tcread "github.com/viant/agently/internal/dao/toolcall/read"
	tcwrite "github.com/viant/agently/internal/dao/toolcall/write"
	d "github.com/viant/agently/internal/domain"
	"sort"
	"testing"
	"time"
)

func TestService_DataDriven(t *testing.T) {
	type testCase struct {
		name   string
		seed   func(ctx context.Context, apis *daofactory.API)
		listBy string // "message" or "turn"
		id     string
		expect string // JSON array of ops [{kind:"model"|"tool", messageId, toolName?, model?, status?}]
		verify func(t *testing.T, ctx context.Context, apis *daofactory.API)
	}

	cases := []testCase{
		{
			name: "record model call and list by message",
			seed: func(ctx context.Context, apis *daofactory.API) {
				// seed message
				_, _ = apis.Message.Patch(ctx, &msgw.Message{Id: "m1", ConversationID: "c1", Has: &msgw.MessageHas{Id: true, ConversationID: true}})
				ops := New(apis)
				w := &mcwrite.ModelCall{}
				w.SetMessageID("m1")
				w.SetProvider("openai")
				w.SetModel("gpt-4o")
				w.SetModelKind("chat")
				_ = ops.RecordModelCall(ctx, w, "", "")
			},
			listBy: "message", id: "m1",
			expect: `[{"kind":"model","messageId":"m1","model":"gpt-4o","provider":"openai"}]`,
		},
		{
			name: "record tool call and list by message",
			seed: func(ctx context.Context, apis *daofactory.API) {
				_, _ = apis.Message.Patch(ctx, &msgw.Message{Id: "m2", ConversationID: "c1", Has: &msgw.MessageHas{Id: true, ConversationID: true}})
				ops := New(apis)
				tw := &tcwrite.ToolCall{}
				tw.SetMessageID("m2")
				tw.SetOpID("op1")
				tw.SetAttempt(1)
				tw.SetToolName("sys.echo")
				tw.SetToolKind("general")
				tw.SetStatus("completed")
				_ = ops.RecordToolCall(ctx, tw, "", "")
			},
			listBy: "message", id: "m2",
			expect: `[{"kind":"tool","messageId":"m2","tool":"sys.echo","status":"completed"}]`,
		},
		{
			name: "model call with tokens and timings",
			seed: func(ctx context.Context, apis *daofactory.API) {
				_, _ = apis.Message.Patch(ctx, &msgw.Message{Id: "m3", ConversationID: "c1", Has: &msgw.MessageHas{Id: true, ConversationID: true}})
				ops := New(apis)
				pt, ct, tt := 5, 7, 12
				started := time.Date(2025, 1, 2, 3, 4, 5, 6, time.UTC)
				completed := time.Date(2025, 1, 2, 3, 4, 6, 7, time.UTC)
				fr := "stop"
				cost := 0.42
				mw := &mcwrite.ModelCall{}
				mw.SetMessageID("m3")
				mw.SetProvider("openai")
				mw.SetModel("gpt-x")
				mw.SetModelKind("chat")
				mw.PromptTokens = &pt
				mw.CompletionTokens = &ct
				mw.TotalTokens = &tt
				mw.FinishReason = &fr
				mw.Cost = &cost
				mw.StartedAt = &started
				mw.CompletedAt = &completed
				if mw.Has == nil {
					mw.Has = &mcwrite.ModelCallHas{}
				}
				mw.Has.PromptTokens = true
				mw.Has.CompletionTokens = true
				mw.Has.TotalTokens = true
				mw.Has.FinishReason = true
				mw.Has.Cost = true
				mw.Has.StartedAt = true
				mw.Has.CompletedAt = true
				_ = ops.RecordModelCall(ctx, mw, "", "")
			},
			listBy: "message", id: "m3",
			expect: `[{"kind":"model","messageId":"m3","model":"gpt-x","provider":"openai","promptTokens":5,"completionTokens":7,"totalTokens":12,"finishReason":"stop","cost":0.42,"startedAt":"2025-01-02T03:04:05Z","completedAt":"2025-01-02T03:04:06Z"}]`,
		},
		{
			name: "tool call with error, cost and timings",
			seed: func(ctx context.Context, apis *daofactory.API) {
				_, _ = apis.Message.Patch(ctx, &msgw.Message{Id: "m4", ConversationID: "c1", Has: &msgw.MessageHas{Id: true, ConversationID: true}})
				ops := New(apis)
				started := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
				completed := time.Date(2025, 2, 3, 4, 5, 7, 0, time.UTC)
				errMsg := "boom"
				cost := 0.123
				tw := &tcwrite.ToolCall{}
				tw.SetMessageID("m4")
				tw.SetOpID("opX")
				tw.SetAttempt(2)
				tw.SetToolName("sys.fail")
				tw.SetToolKind("general")
				tw.SetStatus("failed")
				tw.StartedAt = &started
				tw.CompletedAt = &completed
				tw.ErrorMessage = &errMsg
				tw.Cost = &cost
				if tw.Has == nil {
					tw.Has = &tcwrite.ToolCallHas{}
				}
				tw.Has.StartedAt = true
				tw.Has.CompletedAt = true
				tw.Has.ErrorMessage = true
				tw.Has.Cost = true
				_ = ops.RecordToolCall(ctx, tw, "", "")
			},
			listBy: "message", id: "m4",
			expect: `[{"kind":"tool","messageId":"m4","tool":"sys.fail","status":"failed","error":"boom","cost":0.123,"startedAt":"2025-02-03T04:05:06Z","completedAt":"2025-02-03T04:05:07Z"}]`,
		},
		{
			name: "persist latency for model/tool",
			seed: func(ctx context.Context, apis *daofactory.API) {
				_, _ = apis.Message.Patch(ctx, &msgw.Message{Id: "m5", ConversationID: "c1", Has: &msgw.MessageHas{Id: true, ConversationID: true}})
				ops := New(apis)
				st := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
				ed := st.Add(1500 * time.Millisecond)
				mw := &mcwrite.ModelCall{}
				mw.SetMessageID("m5")
				mw.SetProvider("openai")
				mw.SetModel("gpt-4o")
				mw.SetModelKind("chat")
				mw.StartedAt = &st
				mw.CompletedAt = &ed
				if mw.Has == nil {
					mw.Has = &mcwrite.ModelCallHas{}
				}
				mw.Has.StartedAt = true
				mw.Has.CompletedAt = true
				_ = ops.RecordModelCall(ctx, mw, "", "")
				st2 := st.Add(10 * time.Second)
				ed2 := st2.Add(250 * time.Millisecond)
				tw := &tcwrite.ToolCall{}
				tw.SetMessageID("m5")
				tw.SetOpID("op1")
				tw.SetAttempt(1)
				tw.SetToolName("sys.echo")
				tw.SetToolKind("general")
				tw.SetStatus("completed")
				tw.StartedAt = &st2
				tw.CompletedAt = &ed2
				if tw.Has == nil {
					tw.Has = &tcwrite.ToolCallHas{}
				}
				tw.Has.StartedAt = true
				tw.Has.CompletedAt = true
				_ = ops.RecordToolCall(ctx, tw, "", "")
			},
			listBy: "message", id: "m5",
			expect: `[{"kind":"model","messageId":"m5","model":"gpt-4o","provider":"openai","startedAt":"2025-01-01T00:00:00Z","completedAt":"2025-01-01T00:00:01Z"},{"kind":"tool","messageId":"m5","tool":"sys.echo","status":"completed","startedAt":"2025-01-01T00:00:10Z","completedAt":"2025-01-01T00:00:10Z"}]`,
			verify: func(t *testing.T, ctx context.Context, apis *daofactory.API) {
				mrows, _ := apis.ModelCall.List(ctx, mcread.WithMessageID("m5"))
				if assert.True(t, len(mrows) > 0) {
					assert.NotNil(t, mrows[0].LatencyMS)
					assert.EqualValues(t, 1500, *mrows[0].LatencyMS)
				}
				trows, _ := apis.ToolCall.List(ctx, tcread.WithMessageID("m5"))
				if assert.True(t, len(trows) > 0) {
					assert.NotNil(t, trows[0].LatencyMS)
					assert.EqualValues(t, 250, *trows[0].LatencyMS)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			apis := &daofactory.API{Message: msgmem.New(), ModelCall: mcmem.New(), ToolCall: tcmem.New(), Payload: plmem.New()}
			// Seed
			tc.seed(ctx, apis)
			// Exercise
			var ops []*d.Operation
			var err error
			switch tc.listBy {
			case "message":
				ops, err = New(apis).GetByMessage(ctx, tc.id)
			case "turn":
				ops, err = New(apis).GetByTurn(ctx, tc.id)
			}
			if !assert.NoError(t, err) {
				return
			}
			// Build graph
			var graph []map[string]interface{}
			for _, o := range ops {
				row := map[string]interface{}{"messageId": o.MessageID}
				if o.Model != nil {
					row["kind"] = "model"
					row["model"] = o.Model.Call.Model
					row["provider"] = o.Model.Call.Provider
					if o.Model.Call.PromptTokens != nil {
						row["promptTokens"] = *o.Model.Call.PromptTokens
					}
					if o.Model.Call.CompletionTokens != nil {
						row["completionTokens"] = *o.Model.Call.CompletionTokens
					}
					if o.Model.Call.TotalTokens != nil {
						row["totalTokens"] = *o.Model.Call.TotalTokens
					}
					if o.Model.Call.FinishReason != nil {
						row["finishReason"] = *o.Model.Call.FinishReason
					}
					if o.Model.Call.Cost != nil {
						row["cost"] = *o.Model.Call.Cost
					}
					if o.Model.Call.StartedAt != nil {
						row["startedAt"] = o.Model.Call.StartedAt.UTC().Truncate(time.Second).Format(time.RFC3339)
					}
					if o.Model.Call.CompletedAt != nil {
						row["completedAt"] = o.Model.Call.CompletedAt.UTC().Truncate(time.Second).Format(time.RFC3339)
					}
				}
				if o.Tool != nil {
					row["kind"] = "tool"
					row["tool"] = o.Tool.Call.ToolName
					row["status"] = o.Tool.Call.Status
					if o.Tool.Call.ErrorMessage != nil {
						row["error"] = *o.Tool.Call.ErrorMessage
					}
					if o.Tool.Call.Cost != nil {
						row["cost"] = *o.Tool.Call.Cost
					}
					if o.Tool.Call.StartedAt != nil {
						row["startedAt"] = o.Tool.Call.StartedAt.UTC().Truncate(time.Second).Format(time.RFC3339)
					}
					if o.Tool.Call.CompletedAt != nil {
						row["completedAt"] = o.Tool.Call.CompletedAt.UTC().Truncate(time.Second).Format(time.RFC3339)
					}
				}
				graph = append(graph, row)
			}
			sort.SliceStable(graph, func(i, j int) bool { return graph[i]["messageId"].(string) < graph[j]["messageId"].(string) })
			gotJSON, _ := json.Marshal(graph)
			var gotSlice []map[string]interface{}
			_ = json.Unmarshal(gotJSON, &gotSlice)
			var expSlice []map[string]interface{}
			_ = json.Unmarshal([]byte(tc.expect), &expSlice)
			assert.EqualValues(t, expSlice, gotSlice)
			if tc.verify != nil {
				tc.verify(t, ctx, apis)
			}
		})
	}
}
