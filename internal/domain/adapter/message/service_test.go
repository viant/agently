package message

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	daofactory "github.com/viant/agently/internal/dao/factory"
	msgmem "github.com/viant/agently/internal/dao/message/impl/memory"
	msgread "github.com/viant/agently/internal/dao/message/read"
	mcmem "github.com/viant/agently/internal/dao/modelcall/impl/memory"
	plmem "github.com/viant/agently/internal/dao/payload/impl/memory"
	tcmem "github.com/viant/agently/internal/dao/toolcall/impl/memory"
	d "github.com/viant/agently/internal/domain"
	msgw "github.com/viant/agently/pkg/agently/message"
	mcw "github.com/viant/agently/pkg/agently/modelcall"
	plw "github.com/viant/agently/pkg/agently/payload"
	tcw "github.com/viant/agently/pkg/agently/toolcall"
	"sort"
	"testing"
)

type aggSnapshot struct {
	Items []aggItem
}

type aggItem struct {
	ID           string
	Role         string
	HasModel     bool
	HasTool      bool
	ModelReqBody int
	ModelResBody int
	ToolError    string
	ToolCost     float64
	ModelFinish  string
	ModelCost    float64
}

func snapshotOf(agg *d.AggregatedTranscript) aggSnapshot {
	out := aggSnapshot{}
	for _, m := range agg.Messages {
		it := aggItem{ID: m.Message.ID, Role: m.Message.Role}
		if m.Model != nil {
			it.HasModel = true
			if m.Model.Request != nil && m.Model.Request.InlineBody != nil {
				it.ModelReqBody = len(*m.Model.Request.InlineBody)
			}
			if m.Model.Response != nil && m.Model.Response.InlineBody != nil {
				it.ModelResBody = len(*m.Model.Response.InlineBody)
			}
			if m.Model.Call.FinishReason != nil {
				it.ModelFinish = *m.Model.Call.FinishReason
			}
			if m.Model.Call.Cost != nil {
				it.ModelCost = *m.Model.Call.Cost
			}
		}
		if m.Tool != nil {
			it.HasTool = true
			if m.Tool.Call.ErrorMessage != nil {
				it.ToolError = *m.Tool.Call.ErrorMessage
			}
			if m.Tool.Call.Cost != nil {
				it.ToolCost = *m.Tool.Call.Cost
			}
		}
		out.Items = append(out.Items, it)
	}
	return out
}

func TestService_GetTranscriptAggregated_DataDriven(t *testing.T) {
	type testCase struct {
		name             string
		opts             d.TranscriptAggOptions
		seedInlineBodies bool
		expect           aggSnapshot
	}

	cases := []testCase{
		{
			name:   "preview payloads with tools",
			opts:   d.TranscriptAggOptions{IncludeTools: true, IncludeModelCalls: true, IncludeToolCalls: true, PayloadLevel: d.PayloadPreview, ExcludeInterim: true},
			expect: aggSnapshot{Items: []aggItem{{ID: "a1", Role: "assistant", HasModel: true, HasTool: true}}},
		},
		{
			name:   "no payloads, tools excluded",
			opts:   d.TranscriptAggOptions{IncludeTools: false, IncludeModelCalls: true, IncludeToolCalls: false, PayloadLevel: d.PayloadNone, ExcludeInterim: true},
			expect: aggSnapshot{Items: []aggItem{{ID: "a1", Role: "assistant", HasModel: true}}},
		},
		{
			name:             "inline if small includes bodies",
			opts:             d.TranscriptAggOptions{IncludeTools: true, IncludeModelCalls: true, IncludeToolCalls: true, PayloadLevel: d.PayloadInlineIfSmall, PayloadInlineMaxB: 1024, ExcludeInterim: true},
			seedInlineBodies: true,
			// expected sizes are populated dynamically below using len(reqBody)/len(respBody)
			expect: aggSnapshot{Items: []aggItem{{ID: "a1", Role: "assistant", HasModel: true, HasTool: true}}},
		},
		{
			name:   "tool error and cost in transcript",
			opts:   d.TranscriptAggOptions{IncludeTools: true, IncludeModelCalls: false, IncludeToolCalls: true, PayloadLevel: d.PayloadPreview, ExcludeInterim: true},
			expect: aggSnapshot{Items: []aggItem{{ID: "a1", Role: "assistant", HasTool: true, ToolError: "boom", ToolCost: 0.5}}},
		},
		{
			name:   "model finish and cost in transcript",
			opts:   d.TranscriptAggOptions{IncludeTools: false, IncludeModelCalls: true, IncludeToolCalls: false, PayloadLevel: d.PayloadNone, ExcludeInterim: true},
			expect: aggSnapshot{Items: []aggItem{{ID: "a1", Role: "assistant", HasModel: true, ModelFinish: "stop", ModelCost: 1.23}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			msgDAO := msgmem.New()
			mcDAO := mcmem.New()
			tcDAO := tcmem.New()
			plDAO := plmem.New()

			// Seed payloads
			reqBody := []byte(`{"prompt":"hello"}`)
			respBody := []byte(`{"text":"world"}`)
			pReq := &plw.Payload{Id: "p-req", Kind: "model_request", MimeType: "application/json", SizeBytes: len(reqBody), Storage: "inline", Has: &plw.PayloadHas{Id: true, Kind: true, MimeType: true, SizeBytes: true, Storage: true}}
			pRes := &plw.Payload{Id: "p-res", Kind: "model_response", MimeType: "application/json", SizeBytes: len(respBody), Storage: "inline", Has: &plw.PayloadHas{Id: true, Kind: true, MimeType: true, SizeBytes: true, Storage: true}}
			if tc.seedInlineBodies {
				pReq.SetInlineBody(reqBody)
				pRes.SetInlineBody(respBody)
			}
			_, _ = plDAO.Patch(ctx, pReq, pRes)

			// Seed messages
			_, _ = msgDAO.Patch(ctx,
				&msgw.Message{Id: "u1", ConversationID: "c1", Role: "user", Type: "text", Content: "hello", Has: &msgw.MessageHas{Id: true, ConversationID: true, Role: true, Type: true, Content: true}},
				&msgw.Message{Id: "a1", ConversationID: "c1", TurnID: strp("t1"), Role: "assistant", Type: "text", Content: "hi", Has: &msgw.MessageHas{Id: true, ConversationID: true, TurnID: true, Role: true, Type: true, Content: true}},
			)

			// Seed model call
			mc := &mcw.ModelCall{MessageID: "a1", Provider: "openai", Model: "gpt-4o", ModelKind: "chat",
				RequestPayloadID: strp("p-req"), ResponsePayloadID: strp("p-res"), Has: &mcw.ModelCallHas{MessageID: true, Provider: true, Model: true, ModelKind: true, RequestPayloadID: true, ResponsePayloadID: true}}
			if tc.name == "model finish and cost in transcript" {
				fr := "stop"
				cost := 1.23
				mc.FinishReason = &fr
				mc.Cost = &cost
				mc.Has.FinishReason = true
				mc.Has.Cost = true
			}
			_, _ = mcDAO.Patch(ctx, mc)

			// Seed tool call
			reqSnap := `{"a":1}`
			respSnap := `{"b":2}`
			// Default completed tool entry
			_, _ = tcDAO.Patch(ctx, &tcw.ToolCall{MessageID: "a1", OpID: "op1", Attempt: 1, ToolName: "sys.echo", ToolKind: "general", Status: "completed",
				RequestSnapshot: &reqSnap, ResponseSnapshot: &respSnap, Has: &tcw.ToolCallHas{MessageID: true, OpID: true, Attempt: true, ToolName: true, ToolKind: true, Status: true, RequestSnapshot: true, ResponseSnapshot: true}})
			// Enrich with error/cost for the specific case
			if tc.name == "tool error and cost in transcript" {
				errMsg := "boom"
				cost := 0.5
				_, _ = tcDAO.Patch(ctx, &tcw.ToolCall{MessageID: "a1", OpID: "op1", Attempt: 2, ToolName: "sys.echo", ToolKind: "general", Status: "failed",
					ErrorMessage: &errMsg, Cost: &cost,
					Has: &tcw.ToolCallHas{MessageID: true, OpID: true, Attempt: true, ToolName: true, ToolKind: true, Status: true, ErrorMessage: true, Cost: true}})
			}

			apis := &daofactory.API{Message: msgDAO, Payload: plDAO, ModelCall: mcDAO, ToolCall: tcDAO}
			svc := New(apis)
			agg, err := svc.GetTranscriptAggregated(ctx, "c1", "t1", tc.opts)
			if !assert.NoError(t, err) {
				return
			}
			got := snapshotOf(agg)
			// Build expected dynamically for inline-if-small case to match actual sizes
			expected := tc.expect
			if tc.seedInlineBodies && tc.opts.PayloadLevel == d.PayloadInlineIfSmall {
				if assert.True(t, len(expected.Items) == 1) {
					expected.Items[0].ModelReqBody = len(reqBody)
					expected.Items[0].ModelResBody = len(respBody)
				}
			}
			// Convert to JSON graph and compare – data-driven full graph check
			toJSON := func(s aggSnapshot) string {
				var rows []map[string]interface{}
				for _, it := range s.Items {
					row := map[string]interface{}{"id": it.ID, "role": it.Role, "hasModel": it.HasModel, "hasTool": it.HasTool}
					if it.ModelReqBody > 0 {
						row["modelReqBody"] = it.ModelReqBody
					}
					if it.ModelResBody > 0 {
						row["modelResBody"] = it.ModelResBody
					}
					if it.ModelFinish != "" {
						row["modelFinish"] = it.ModelFinish
					}
					if it.ModelCost > 0 {
						row["modelCost"] = it.ModelCost
					}
					if it.ToolError != "" {
						row["toolError"] = it.ToolError
					}
					if it.ToolCost > 0 {
						row["toolCost"] = it.ToolCost
					}
					rows = append(rows, row)
				}
				sort.SliceStable(rows, func(i, j int) bool { return rows[i]["id"].(string) < rows[j]["id"].(string) })
				b, _ := json.Marshal(rows)
				return string(b)
			}
			assert.EqualValues(t, toJSON(expected), toJSON(got))
		})
	}
}

func strp(s string) *string { return &s }

// ---------------------- Update semantics tests ----------------------

func TestService_Update_MessageFields(t *testing.T) {
	type updCase struct {
		name   string
		patch  func() *msgw.Message
		expect string
	}

	cases := []updCase{
		{
			name: "update content only",
			patch: func() *msgw.Message {
				w := &msgw.Message{Id: "m1", Has: &msgw.MessageHas{Id: true}}
				w.SetContent("hello world")
				return w
			},
			expect: `[{"content":"hello world","id":"m1","role":"user","type":"text"}]`,
		},
		{
			name: "set tool name",
			patch: func() *msgw.Message {
				w := &msgw.Message{Id: "m1", Has: &msgw.MessageHas{Id: true}}
				w.SetToolName("sys.echo")
				return w
			},
			expect: `[{"content":"hello","id":"m1","role":"user","type":"text","toolName":"sys.echo"}]`,
		},
		{
			name: "set interim and tool name and content",
			patch: func() *msgw.Message {
				interim := 1
				w := &msgw.Message{Id: "m1", Has: &msgw.MessageHas{Id: true}}
				w.SetContent("hello world")
				w.SetToolName("sys.echo")
				w.Interim = &interim
				w.Has.Interim = true
				return w
			},
			expect: `[{"content":"hello world","id":"m1","role":"user","type":"text","toolName":"sys.echo","interim":1}]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			msgDAO := msgmem.New()
			plDAO := plmem.New()
			mcDAO := mcmem.New()
			tcDAO := tcmem.New()

			// Seed initial message
			_, _ = msgDAO.Patch(ctx, &msgw.Message{Id: "m1", ConversationID: "c1", Role: "user", Type: "text", Content: "hello", Has: &msgw.MessageHas{Id: true, ConversationID: true, Role: true, Type: true, Content: true}})

			apis := &daofactory.API{Message: msgDAO, Payload: plDAO, ModelCall: mcDAO, ToolCall: tcDAO}
			svc := New(apis)

			// Apply patch
			_, err := svc.Patch(ctx, tc.patch())
			if !assert.NoError(t, err) {
				return
			}

			// List and verify using full-graph JSON snapshot
			out, err := svc.List(ctx, msgread.WithConversationID("c1"))
			if !assert.NoError(t, err) {
				return
			}
			var graph []map[string]interface{}
			for _, m := range out {
				row := map[string]interface{}{"id": m.Id, "role": m.Role, "type": m.Type, "content": m.Content}
				if m.ToolName != nil {
					row["toolName"] = *m.ToolName
				}
				if m.Interim != nil {
					row["interim"] = *m.Interim
				}
				graph = append(graph, row)
			}
			sort.SliceStable(graph, func(i, j int) bool { return graph[i]["id"].(string) < graph[j]["id"].(string) })
			gotJSON, _ := json.Marshal(graph)
			var gotSlice []map[string]interface{}
			_ = json.Unmarshal(gotJSON, &gotSlice)
			var expSlice []map[string]interface{}
			_ = json.Unmarshal([]byte(tc.expect), &expSlice)
			assert.EqualValues(t, expSlice, gotSlice)
		})
	}
}
