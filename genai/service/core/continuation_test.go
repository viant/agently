package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

func TestBuildContinuationRequest_IncludesAssistantToolCalls(t *testing.T) {
	svc := &Service{}
	ctx := memory.WithTurnMeta(context.Background(), memory.TurnMeta{ConversationID: "conv-1"})
	history := &prompt.History{
		Traces:       map[string]*prompt.Trace{},
		LastResponse: &prompt.Trace{ID: "resp-123", At: time.Now()},
	}
	toolKey := prompt.KindToolCall.Key("call-1")
	history.Traces[toolKey] = &prompt.Trace{ID: "resp-123"}
	toolKey2 := prompt.KindToolCall.Key("call-2")
	history.Traces[toolKey2] = &prompt.Trace{ID: "resp-other"}

	req := &llm.GenerateRequest{}
	req.Messages = append(req.Messages,
		llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "toolA"}, {ID: "call-2", Name: "toolB"}}},
		llm.Message{Role: llm.RoleTool, ToolCallId: "call-1"},
		llm.Message{Role: llm.RoleTool, ToolCallId: "call-2"},
	)

	cont := svc.BuildContinuationRequest(ctx, req, history)
	if assert.NotNil(t, cont) {
		assert.Equal(t, "resp-123", cont.PreviousResponseID)
		if assert.Len(t, cont.Messages, 2) {
			assistantMsg := cont.Messages[0]
			toolMsg := cont.Messages[1]
			assert.Equal(t, llm.RoleAssistant, assistantMsg.Role)
			if assert.Len(t, assistantMsg.ToolCalls, 1) {
				assert.Equal(t, "call-1", assistantMsg.ToolCalls[0].ID)
			}
			assert.Equal(t, "call-1", toolMsg.ToolCallId)
		}
	}
}

func TestGroupMessagesByAnchor_IncludesAssistantMessages(t *testing.T) {
	respID := "resp-1"
	timeRef := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	toolTrace := respID
	traces := apiconv.IndexedMessages{
		"call-1": {ToolCall: &agconv.ToolCallView{TraceId: &toolTrace}},
		respID:   {ModelCall: &agconv.ModelCallView{TraceId: &respID}, CreatedAt: timeRef},
	}
	testCases := []struct {
		name     string
		messages []llm.Message
	}{
		{
			name: "single anchor",
			messages: []llm.Message{
				{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "run"}}},
				{Role: llm.RoleTool, ToolCallId: "call-1"},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			groups, order, latest := groupMessagesByAnchor(tc.messages, traces)
			if assert.Len(t, order, 1) {
				assert.EqualValues(t, respID, order[0])
			}
			assert.EqualValues(t, respID, latest)
			if assert.Contains(t, groups, respID) {
				if assert.Len(t, groups[respID], 2) {
					assert.EqualValues(t, llm.RoleAssistant, groups[respID][0].Role)
					assert.EqualValues(t, llm.RoleTool, groups[respID][1].Role)
					assert.EqualValues(t, "call-1", groups[respID][1].ToolCallId)
				}
			}
		})
	}
}
