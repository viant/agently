package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFunctionCall(t *testing.T) {
	cases := []struct {
		desc string
		name string
		args map[string]interface{}
	}{
		{desc: "no args", name: "fn", args: map[string]interface{}{}},
		{desc: "with args", name: "fn", args: map[string]interface{}{"foo": "bar", "num": 42}},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			fc := NewFunctionCall(tc.name, tc.args)
			assert.EqualValues(t, tc.name, fc.Name)
			var got map[string]interface{}
			err := json.Unmarshal([]byte(fc.Arguments), &got)
			assert.NoError(t, err)
			assert.EqualValues(t, tc.args, got)
		})
	}
}

func TestNewToolCall(t *testing.T) {
	cases := []struct {
		desc string
		args map[string]interface{}
	}{
		{desc: "no args", args: map[string]interface{}{}},
		{desc: "with args", args: map[string]interface{}{"city": "Paris", "units": "C"}},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			// prepare a copy to ensure original is not modified
			input := make(map[string]interface{}, len(tc.args))
			for k, v := range tc.args {
				input[k] = v
			}
			tl := NewToolCall("", "toolName", input)
			assert.NotEmpty(t, tl.ID)
			assert.EqualValues(t, "toolName", tl.Name)
			assert.EqualValues(t, tc.args, tl.Arguments)
			assert.EqualValues(t, "function", tl.Type)
			var got map[string]interface{}
			err := json.Unmarshal([]byte(tl.Function.Arguments), &got)
			assert.NoError(t, err)
			assert.EqualValues(t, tc.args, got)
		})
	}
}

func TestNewAssistantMessageWithToolCalls(t *testing.T) {
	cases := []struct {
		desc  string
		calls []ToolCall
	}{
		{desc: "empty calls", calls: nil},
		{desc: "multiple calls", calls: []ToolCall{
			NewToolCall("id", "a", map[string]interface{}{"x": 1}),
			NewToolCall("", "b", map[string]interface{}{"y": 2}),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			msg := NewAssistantMessageWithToolCalls(tc.calls...)
			assert.EqualValues(t, RoleAssistant, msg.Role)
			assert.EqualValues(t, tc.calls, msg.ToolCalls)
		})
	}
}

func TestTextMessageHelpers(t *testing.T) {
	cases := []struct {
		desc   string
		msg    Message
		exRole MessageRole
	}{
		{desc: "user role", msg: NewUserMessage("hello"), exRole: RoleUser},
		{desc: "system role", msg: NewSystemMessage("hello"), exRole: RoleSystem},
		{desc: "assistant role", msg: NewAssistantMessage("hello"), exRole: RoleAssistant},
		{desc: "tool role", msg: NewToolMessage("nm", "hello"), exRole: RoleTool},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			assert.EqualValues(t, tc.exRole, tc.msg.Role)
			assert.Len(t, tc.msg.Items, 1)
			assert.EqualValues(t, "hello", tc.msg.Items[0].Data)
			assert.EqualValues(t, "hello", tc.msg.Content)
		})
	}
}

func TestNewUserMessageWithImage(t *testing.T) {
	cases := []struct {
		desc   string
		text   string
		url    string
		detail string
	}{
		{desc: "basic image", text: "caption", url: "http://img", detail: "high"},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			msg := NewUserMessageWithImage(tc.text, tc.url, tc.detail)
			assert.EqualValues(t, RoleUser, msg.Role)
			assert.Len(t, msg.Items, 2)
			assert.EqualValues(t, tc.text, msg.Items[0].Data)
			assert.EqualValues(t, tc.url, msg.Items[1].Data)
			assert.EqualValues(t, map[string]interface{}{"detail": tc.detail}, msg.Items[1].Metadata)
		})
	}
}

func TestNewToolResultMessage(t *testing.T) {
	cases := []struct {
		desc    string
		call    ToolCall
		content string
	}{
		{
			desc:    "basic tool result",
			call:    NewToolCall("id-123", "toolName", map[string]interface{}{"foo": "bar"}),
			content: "result text",
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			msg := NewToolResultMessage(tc.call, tc.content)
			assert.EqualValues(t, RoleTool, msg.Role)
			assert.EqualValues(t, tc.call.Name, msg.Name)
			assert.EqualValues(t, tc.call.ID, msg.ToolCallId)
			assert.Len(t, msg.Items, 1)
			assert.EqualValues(t, tc.content, msg.Items[0].Data)
			assert.EqualValues(t, tc.content, msg.Content)
		})
	}
}
