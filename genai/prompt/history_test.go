package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

func TestHistory_LLMMessages_ToolExposure(t *testing.T) {
	buildToolMsg := func(opID, name, content string) *Message {
		return &Message{
			Kind:       MessageKindToolResult,
			Role:       "assistant",
			ToolOpID:   opID,
			ToolName:   name,
			ToolArgs:   map[string]interface{}{"foo": "bar"},
			Content:    content,
			Attachment: nil,
		}
	}

	testCases := []struct {
		name          string
		history       History
		wantToolNames []string
	}{
		{
			name: "turn-exposure-keeps-current-turn-tools",
			history: History{
				Past: []*Turn{
					{ID: "t-1", Messages: []*Message{buildToolMsg("op-1", "old_tool", "old output")}},
					{ID: "t-2", Messages: []*Message{buildToolMsg("op-2", "new_tool", "new output")}},
				},
				CurrentTurnID: "t-2",
				ToolExposure:  "turn",
			},
			wantToolNames: []string{"new_tool"},
		},
		{
			name: "conversation-exposure-includes-all-tools",
			history: History{
				Past: []*Turn{
					{ID: "t-1", Messages: []*Message{buildToolMsg("op-1", "old_tool", "old output")}},
					{ID: "t-2", Messages: []*Message{buildToolMsg("op-2", "new_tool", "new output")}},
				},
				CurrentTurnID: "t-2",
				ToolExposure:  "conversation",
			},
			wantToolNames: []string{"old_tool", "new_tool"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			messages := tc.history.LLMMessages()
			got := extractToolNames(messages)
			assert.EqualValues(t, tc.wantToolNames, got)
		})
	}
}

func TestHistory_LLMMessages_NoToolDedup(t *testing.T) {
	buildToolMsg := func(content string) *Message {
		return &Message{
			Kind:     MessageKindToolResult,
			Role:     "assistant",
			ToolOpID: "op-1",
			ToolName: "dup_tool",
			Content:  content,
		}
	}
	buildAssistant := func(content string) *Message {
		return &Message{
			Kind:    MessageKindChatAssistant,
			Role:    "assistant",
			Content: content,
		}
	}

	testCases := []struct {
		name          string
		history       History
		targetContent string
		wantAssistant int
	}{
		{
			name: "assistant-echo-retained",
			history: History{
				Past: []*Turn{
					{
						ID: "t-1",
						Messages: []*Message{
							buildToolMsg("same output"),
							buildAssistant("same output"),
						},
					},
				},
				ToolExposure: "conversation",
			},
			targetContent: "same output",
			wantAssistant: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msgs := tc.history.LLMMessages()
			got := countAssistantByContent(msgs, tc.targetContent)
			assert.EqualValues(t, tc.wantAssistant, got)
		})
	}
}

func TestHistory_LLMMessages_ToolResultIncludesAttachments(t *testing.T) {
	toolMsg := &Message{
		Kind:     MessageKindToolResult,
		Role:     "assistant",
		ToolOpID: "op-1",
		ToolName: "resources.readImage",
		ToolArgs: map[string]interface{}{},
		Content:  `{"status":"ok"}`,
		Attachment: []*Attachment{
			{Name: "img.png", Mime: "image/png", Data: []byte{1, 2, 3}},
		},
	}
	h := History{
		Past: []*Turn{{ID: "t-1", Messages: []*Message{toolMsg}}},
	}
	msgs := h.LLMMessages()

	var sawBinary bool
	for _, m := range msgs {
		if m.Role != llm.RoleTool {
			continue
		}
		for _, it := range m.Items {
			if it.Type == llm.ContentTypeBinary && strings.HasPrefix(it.MimeType, "image/") {
				sawBinary = true
				break
			}
		}
	}
	assert.EqualValues(t, true, sawBinary)
}

func extractToolNames(messages []llm.Message) []string {
	var out []string
	for _, m := range messages {
		if len(m.ToolCalls) == 0 {
			continue
		}
		for _, tc := range m.ToolCalls {
			out = append(out, tc.Name)
		}
	}
	return out
}

func countAssistantByContent(messages []llm.Message, content string) int {
	count := 0
	for _, m := range messages {
		if strings.EqualFold(m.Role.String(), llm.RoleAssistant.String()) && strings.TrimSpace(m.Content) == strings.TrimSpace(content) {
			count++
		}
	}
	return count
}
