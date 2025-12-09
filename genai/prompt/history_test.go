package prompt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

func TestMessage_ToLLM(t *testing.T) {
	now := time.Now().UTC()
	m := &Message{
		Role:    "user",
		Content: "hello",
		Attachment: []*Attachment{
			{Name: "b.txt", URI: "file://b.txt", Content: "B"},
			{Name: "a.txt", URI: "file://a.txt", Content: "A"},
		},
		CreatedAt: now,
	}

	got := m.ToLLM()
	assert.EqualValues(t, "user", got.Role.String())
	assert.EqualValues(t, "hello", got.Content)
	// Attachments should be sorted by URI for stable order. We only
	// assert on the first two items as providers may add extra
	// content items.
	if assert.GreaterOrEqual(t, len(got.Items), 2) {
		assert.EqualValues(t, "a.txt", got.Items[0].Name)
		assert.EqualValues(t, "b.txt", got.Items[1].Name)
	}
}

func TestHistory_LLMMessages_UsesTurnsChronologically(t *testing.T) {
	// Two turns with interleaved times; ensure LLMMessages respects
	// turn/message ordering derived from Turns.
	t1Time := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t2Time := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)

	h := &History{
		Past: []*Turn{
			{
				ID:        "t1",
				StartedAt: t1Time,
				Messages:  []*Message{{Role: "user", Content: "u1", CreatedAt: t1Time}},
			},
			{
				ID:        "t2",
				StartedAt: t2Time,
				Messages:  []*Message{{Role: "assistant", Content: "a1", CreatedAt: t2Time}},
			},
		},
	}

	got := h.LLMMessages()
	if assert.EqualValues(t, 2, len(got)) {
		assert.EqualValues(t, "user", got[0].Role.String())
		assert.EqualValues(t, "u1", got[0].Content)
		assert.EqualValues(t, "assistant", got[1].Role.String())
		assert.EqualValues(t, "a1", got[1].Content)
	}
}

func TestHistory_LLMMessages_IncludesToolResultsForAllTurns(t *testing.T) {
	toolArgs := map[string]interface{}{"foo": "bar"}
	h := History{
		Past: []*Turn{
			{
				ID: "past",
				Messages: []*Message{
					{Kind: MessageKindChatUser, Role: "user", Content: "old question", CreatedAt: time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)},
					{Kind: MessageKindToolResult, Role: "assistant", Content: "old result", ToolOpID: "call-old", ToolName: "system/time", ToolArgs: toolArgs, CreatedAt: time.Date(2025, 1, 1, 8, 1, 0, 0, time.UTC)},
				},
			},
		},
		Current: &Turn{
			ID: "current",
			Messages: []*Message{
				{Kind: MessageKindChatUser, Role: "user", Content: "what time?", CreatedAt: time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)},
				{Kind: MessageKindToolResult, Role: "assistant", Content: "Mon Dec 8", ToolOpID: "call-1", ToolName: "shell/date.now", ToolArgs: toolArgs, CreatedAt: time.Date(2025, 1, 1, 9, 1, 0, 0, time.UTC)},
			},
		},
	}
	got := h.LLMMessages()
	if assert.EqualValues(t, 6, len(got)) {
		assert.EqualValues(t, "user", got[0].Role.String())        // past user
		if assert.EqualValues(t, llm.RoleAssistant, got[1].Role) { // past tool call assistant wrapper
			if assert.Len(t, got[1].ToolCalls, 1) {
				assert.EqualValues(t, "call-old", got[1].ToolCalls[0].ID)
				assert.EqualValues(t, "system-time", got[1].ToolCalls[0].Name)
			}
		}
		assert.EqualValues(t, llm.RoleTool, got[2].Role)
		assert.EqualValues(t, "call-old", got[2].ToolCallId)
		assert.EqualValues(t, "user", got[3].Role.String()) // current user
		if assert.EqualValues(t, llm.RoleAssistant, got[4].Role) {
			if assert.Len(t, got[4].ToolCalls, 1) {
				assert.EqualValues(t, "call-1", got[4].ToolCalls[0].ID)
				assert.EqualValues(t, "shell_date-now", got[4].ToolCalls[0].Name)
			}
		}
		assert.EqualValues(t, llm.RoleTool, got[5].Role)
		assert.EqualValues(t, "call-1", got[5].ToolCallId)
	}
}
