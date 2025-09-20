package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	convcli "github.com/viant/agently/client/conversation"
	mem "github.com/viant/agently/internal/service/conversation/memory"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	msgw "github.com/viant/agently/pkg/agently/message/write"
	mcallw "github.com/viant/agently/pkg/agently/modelcall/write"
	toolw "github.com/viant/agently/pkg/agently/toolcall/write"
	turnw "github.com/viant/agently/pkg/agently/turn/write"
)

// dd-style data-driven test using testCase with input and expected output
func TestClient_GetConversation_DataDriven(t *testing.T) {
	ctx := context.Background()
	c := mem.New()

	// Arrange fixed timestamps
	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Minute)
	t2 := t0.Add(2 * time.Minute)

	// Seed conversation
	conv := convcli.NewConversation()
	conv.SetId("c1")
	conv.SetCreatedAt(t0)
	conv.SetAgentName("agentA")
	conv.SetTitle("A title")
	assert.NoError(t, c.PatchConversations(ctx, conv))

	// Seed turns
	turn1 := &turnw.Turn{Has: &turnw.TurnHas{}}
	turn1.SetId("t1")
	turn1.SetConversationID("c1")
	turn1.SetStatus("ok")
	turn1.SetCreatedAt(t1)
	assert.NoError(t, c.PatchTurn(ctx, (*convcli.MutableTurn)(turn1)))

	turn2 := &turnw.Turn{Has: &turnw.TurnHas{}}
	turn2.SetId("t2")
	turn2.SetConversationID("c1")
	turn2.SetStatus("ok")
	turn2.SetCreatedAt(t2)
	assert.NoError(t, c.PatchTurn(ctx, (*convcli.MutableTurn)(turn2)))

	// Seed messages
	m1 := &msgw.Message{Has: &msgw.MessageHas{}}
	m1.SetId("m1")
	m1.SetConversationID("c1")
	m1.SetTurnID("t1")
	m1.SetRole("user")
	m1.SetType("text")
	m1.SetContent("hello")
	m1.SetCreatedAt(t1)
	assert.NoError(t, c.PatchMessage(ctx, (*convcli.MutableMessage)(m1)))

	m2 := &msgw.Message{Has: &msgw.MessageHas{}}
	m2.SetId("m2")
	m2.SetConversationID("c1")
	m2.SetTurnID("t2")
	m2.SetRole("assistant")
	m2.SetType("text")
	m2.SetContent("world")
	m2.SetCreatedAt(t2)
	assert.NoError(t, c.PatchMessage(ctx, (*convcli.MutableMessage)(m2)))

	// Attach model call to m2
	mc := &mcallw.ModelCall{Has: &mcallw.ModelCallHas{}}
	mc.SetMessageID("m2")
	mc.SetProvider("openai")
	mc.SetModel("gpt-4o")
	mc.SetModelKind("chat")
	mc.SetStatus("ok")
	assert.NoError(t, c.PatchModelCall(ctx, (*convcli.MutableModelCall)(mc)))

	// Attach tool call to m2 (as separate tool message)
	m3 := &msgw.Message{Has: &msgw.MessageHas{}}
	m3.SetId("m3")
	m3.SetConversationID("c1")
	m3.SetTurnID("t2")
	m3.SetRole("assistant")
	m3.SetType("tool")
	m3.SetContent("call:toolX")
	m3.SetCreatedAt(t2)
	assert.NoError(t, c.PatchMessage(ctx, (*convcli.MutableMessage)(m3)))

	tc := &toolw.ToolCall{Has: &toolw.ToolCallHas{}}
	tc.SetMessageID("m3")
	tc.SetOpID("op-1")
	tc.SetAttempt(1)
	tc.SetToolName("toolX")
	tc.SetToolKind("http")
	tc.SetStatus("ok")
	assert.NoError(t, c.PatchToolCall(ctx, (*convcli.MutableToolCall)(tc)))

	srv := convcli.NewService(c)

	type testCase struct {
		name string
		req  convcli.GetRequest
		exp  *convcli.GetResponse
	}

	// Build expected base conversation using agconv then cast to client type
	agBase := &agconv.ConversationView{
		Id:         "c1",
		AgentName:  ptrS("agentA"),
		Title:      ptrS("A title"),
		Visibility: "",
		CreatedAt:  t0,
		Transcript: []*agconv.TranscriptView{
			{Id: "t1", ConversationId: "c1", CreatedAt: t1, Status: "ok", Message: []*agconv.MessageView{{Id: "m1", ConversationId: "c1", TurnId: ptrS("t1"), Role: "user", Type: "text", Content: ptrS("hello"), CreatedAt: t1}}},
			{Id: "t2", ConversationId: "c1", CreatedAt: t2, Status: "ok", Message: []*agconv.MessageView{
				{Id: "m2", ConversationId: "c1", TurnId: ptrS("t2"), Role: "assistant", Type: "text", Content: ptrS("world"), CreatedAt: t2},
				{Id: "m3", ConversationId: "c1", TurnId: ptrS("t2"), Role: "assistant", Type: "tool", Content: ptrS("call:toolX"), CreatedAt: t2},
			}},
		},
	}
	base := toClient(agBase)

	// Expected with model included
	withModel := toClient(cloneAg(agBase))
	withModel.Transcript[1].Message[0].ModelCall = &agconv.ModelCallView{MessageId: "m2", Provider: "openai", Model: "gpt-4o", ModelKind: "chat", Status: "ok"}

	// Expected with tool included
	withTool := toClient(cloneAg(agBase))
	withTool.Transcript[1].Message[1].ToolCall = &agconv.ToolCallView{MessageId: "m3", OpId: "op-1", Attempt: 1, ToolName: "toolX", ToolKind: "http", Status: "ok"}

	// Expected since t2 only
	sinceT2 := toClient(cloneAg(agBase))
	sinceT2.Transcript = sinceT2.Transcript[1:]

	cases := []testCase{
		{
			name: "no options: no model/tool",
			req:  convcli.GetRequest{Id: "c1"},
			exp:  &convcli.GetResponse{Conversation: base},
		},
		{
			name: "include model",
			req:  convcli.GetRequest{Id: "c1", IncludeModelCall: true},
			exp:  &convcli.GetResponse{Conversation: withModel},
		},
		{
			name: "include tool",
			req:  convcli.GetRequest{Id: "c1", IncludeToolCall: true},
			exp:  &convcli.GetResponse{Conversation: withTool},
		},
		{
			name: "since t2",
			req:  convcli.GetRequest{Id: "c1", Since: "t2"},
			exp:  &convcli.GetResponse{Conversation: sinceT2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := srv.Get(ctx, tc.req)
			assert.NoError(t, err)
			assert.EqualValues(t, tc.exp, got)
		})
	}
}

func TestClient_GetConversations_ListSummary(t *testing.T) {
	ctx := context.Background()
	c := mem.New()

	conv := convcli.NewConversation()
	conv.SetId("c1")
	conv.SetAgentName("agentA")
	conv.SetCreatedAt(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	assert.NoError(t, c.PatchConversations(ctx, conv))

	// Add a turn but list should not include transcript
	turn := &turnw.Turn{Has: &turnw.TurnHas{}}
	turn.SetId("t1")
	turn.SetConversationID("c1")
	turn.SetStatus("ok")
	assert.NoError(t, c.PatchTurn(ctx, (*convcli.MutableTurn)(turn)))

	items, err := c.GetConversations(ctx)
	assert.NoError(t, err)
	expected := []*convcli.Conversation{{
		Id:        "c1",
		AgentName: ptrS("agentA"),
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}}
	assert.EqualValues(t, expected, items)
}

// Helpers for building expected data
func ptrS(s string) *string { return &s }

// toClient casts agconv view to client view
func toClient(v *agconv.ConversationView) *convcli.Conversation {
	c := convcli.Conversation(*v)
	return &c
}

// cloneAg deep copies an agconv conversation
func cloneAg(in *agconv.ConversationView) *agconv.ConversationView {
	if in == nil {
		return nil
	}
	cp := *in
	if in.Transcript != nil {
		cp.Transcript = make([]*agconv.TranscriptView, len(in.Transcript))
		for i, tv := range in.Transcript {
			if tv == nil {
				continue
			}
			tt := *tv
			if tv.Message != nil {
				tt.Message = make([]*agconv.MessageView, len(tv.Message))
				for j, mv := range tv.Message {
					if mv == nil {
						continue
					}
					mm := *mv
					if mv.ModelCall != nil {
						tmp := *mv.ModelCall
						mm.ModelCall = &tmp
					}
					if mv.ToolCall != nil {
						tmp := *mv.ToolCall
						mm.ToolCall = &tmp
					}
					tt.Message[j] = &mm
				}
			}
			cp.Transcript[i] = &tt
		}
	}
	return &cp
}
