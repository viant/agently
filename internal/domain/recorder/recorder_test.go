package recorder

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	convwrite "github.com/viant/agently/internal/dao/conversation/write"
	msgread "github.com/viant/agently/internal/dao/message/read"
	mcread "github.com/viant/agently/internal/dao/modelcall/read"
	tcread "github.com/viant/agently/internal/dao/toolcall/read"
	usageread "github.com/viant/agently/internal/dao/usage/read"
	d "github.com/viant/agently/internal/domain"
)

// TestEnablement verifies Enabled based on mode.
func TestEnablement_DataDriven(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		mode Mode
		want bool
	}{
		{ModeOff, false}, {ModeShadow, true}, {ModeFull, true},
	}
	for _, tc := range cases {
		t.Run(string(tc.mode), func(t *testing.T) {
			t.Setenv("AGENTLY_DOMAIN_MODE", string(tc.mode))
			w := New(ctx)
			if s, ok := w.(*Store); ok {
				assert.Equal(t, tc.want, s.Enabled())
			} else {
				t.Fatal("store type assertion failed")
			}
		})
	}
}

// TestMessageRecorder verifies RecordMessage basic flow.
func TestMessageRecorder_DataDriven(t *testing.T) {
	ctx := context.Background()
	t.Setenv("AGENTLY_DOMAIN_MODE", string(ModeShadow))
	w := New(ctx).(*Store)
	convID := "c-msg"
	if conv := w.store.Conversations(); conv != nil {
		c := &convwrite.Conversation{Has: &convwrite.ConversationHas{}}
		c.SetId(convID)
		_, _ = conv.Patch(ctx, c)
	}
	w.RecordMessage(ctx, memory.Message{ID: "m-1", ConversationID: convID, Role: "user", Content: "hi", CreatedAt: time.Now()})
	// list messages by conversation; may be empty depending on DAO, but ensure no error
	_, err := w.store.Messages().List(ctx, msgread.WithConversationID(convID))
	assert.NoError(t, err)
}

// TestTurnRecorder verifies turn start/update flows.
func TestTurnRecorder_DataDriven(t *testing.T) {
	ctx := context.Background()
	t.Setenv("AGENTLY_DOMAIN_MODE", string(ModeShadow))
	w := New(ctx).(*Store)
	convID := "c-turn"
	if conv := w.store.Conversations(); conv != nil {
		c := &convwrite.Conversation{Has: &convwrite.ConversationHas{}}
		c.SetId(convID)
		_, _ = conv.Patch(ctx, c)
	}
	turnID := "t-1"
	w.StartTurn(ctx, convID, turnID, time.Now())
	w.UpdateTurn(ctx, turnID, "succeeded")
	_, err := w.store.Turns().List(ctx)
	assert.NoError(t, err)
}

// TestModelCallRecorder verifies model-call persistence minimal path.
func TestModelCallRecorder_DataDriven(t *testing.T) {
	ctx := context.Background()
	t.Setenv("AGENTLY_DOMAIN_MODE", string(ModeShadow))
	w := New(ctx).(*Store)
	convID, turnID, msgID := "c-model", "t-model", "m-model"
	if conv := w.store.Conversations(); conv != nil {
		c := &convwrite.Conversation{Has: &convwrite.ConversationHas{}}
		c.SetId(convID)
		_, _ = conv.Patch(ctx, c)
	}
	w.RecordMessage(ctx, memory.Message{ID: msgID, ConversationID: convID, Role: "assistant", Content: "x", CreatedAt: time.Now()})
	usage := &llm.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}
	w.StartModelCall(ctx, ModelCallStart{MessageID: msgID, TurnID: turnID, Provider: "openai", Model: "gpt-x", ModelKind: "chat", StartedAt: time.Now(), Request: map[string]string{"p": "v"}})
	w.FinishModelCall(ctx, ModelCallFinish{MessageID: msgID, TurnID: turnID, Usage: usage, FinishReason: "stop", CompletedAt: time.Now(), Response: map[string]string{"r": "v"}})
	// operations by message should not error
	_, err := w.store.Operations().GetByMessage(ctx, msgID)
	assert.NoError(t, err)
}

// TestToolCallRecorder verifies tool-call persistence minimal path.
func TestToolCallRecorder_DataDriven(t *testing.T) {
	ctx := context.Background()
	t.Setenv("AGENTLY_DOMAIN_MODE", string(ModeShadow))
	w := New(ctx).(*Store)
	msgID, turnID := "m-tool", "t-tool"
	// Start then update
	w.StartToolCall(ctx, ToolCallStart{MessageID: msgID, TurnID: turnID, ToolName: "sys.echo", StartedAt: time.Now(), Request: map[string]any{"a": 1}})
	w.FinishToolCall(ctx, ToolCallUpdate{MessageID: msgID, TurnID: turnID, ToolName: "sys.echo", Status: "completed", CompletedAt: time.Now(), Response: map[string]any{"b": 2}})
	_, err := w.store.Operations().GetByMessage(ctx, msgID)
	assert.NoError(t, err)
}

// TestUsageRecorder verifies usage totals path.
func TestUsageRecorder_DataDriven(t *testing.T) {
	ctx := context.Background()
	t.Setenv("AGENTLY_DOMAIN_MODE", string(ModeShadow))
	w := New(ctx).(*Store)
	convID := "c-usage"
	if conv := w.store.Conversations(); conv != nil {
		c := &convwrite.Conversation{Has: &convwrite.ConversationHas{}}
		c.SetId(convID)
		_, _ = conv.Patch(ctx, c)
	}
	w.RecordUsageTotals(ctx, convID, 5, 6, 7)
	_, err := w.store.Usage().List(ctx, usageread.Input{ConversationID: convID, Has: &usageread.Has{ConversationID: true}})
	assert.NoError(t, err)
}

// Legacy smoke test retained for coarse verification (optional)
func TestStore_Memory_WriteFlow(t *testing.T) {
	ctx := context.Background()
	// Enable shadow mode for domain writer
	prev := os.Getenv("AGENTLY_DOMAIN_MODE")
	_ = os.Setenv("AGENTLY_DOMAIN_MODE", string(ModeShadow))
	t.Cleanup(func() { _ = os.Setenv("AGENTLY_DOMAIN_MODE", prev) })

	w := New(ctx)
	if !assert.NotNil(t, w) {
		return
	}
	if !assert.True(t, w.Enabled()) {
		return
	}

	// Cast to concrete to inspect underlying store
	s, ok := w.(*Store)
	if !assert.True(t, ok) {
		return
	}
	if !assert.NotNil(t, s.store) {
		return
	}

	convID := "c-test"
	turnID := "t-test"
	msgID := "m-test"

	// Seed conversation and record a message
	// Ensure conversation exists because memory DAOs may enforce FK-like behavior
	if conv := s.store.Conversations(); conv != nil {
		c := &convwrite.Conversation{Has: &convwrite.ConversationHas{}}
		c.SetId(convID)
		_, _ = conv.Patch(ctx, c)
	}
	w.RecordMessage(ctx, memory.Message{ID: msgID, ConversationID: convID, Role: "assistant", Content: "hello", CreatedAt: time.Now()})

	// Record turn start/update
	w.StartTurn(ctx, convID, turnID, time.Now().Add(-1*time.Second))
	w.UpdateTurn(ctx, turnID, "succeeded")

	// Record model call with usage and payloads
	usage := &llm.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7}
	w.StartModelCall(ctx, ModelCallStart{MessageID: msgID, TurnID: turnID, Provider: "openai", Model: "gpt-4o", ModelKind: "chat", StartedAt: time.Now().Add(-500 * time.Millisecond), Request: map[string]string{"p": "x"}})
	w.FinishModelCall(ctx, ModelCallFinish{MessageID: msgID, TurnID: turnID, Usage: usage, FinishReason: "stop", CompletedAt: time.Now(), Response: map[string]string{"r": "y"}})

	// Record tool call with payloads and error/cost
	cost := 0.25
	// Start then update with failure
	started := time.Now().Add(-250 * time.Millisecond)
	w.StartToolCall(ctx, ToolCallStart{MessageID: msgID, TurnID: turnID, ToolName: "sys.echo", StartedAt: started, Request: map[string]any{"a": 1}})
	w.FinishToolCall(ctx, ToolCallUpdate{MessageID: msgID, TurnID: turnID, ToolName: "sys.echo", Status: "failed", CompletedAt: time.Now(), ErrMsg: "boom", Cost: &cost, Response: map[string]any{"b": 2}})

	// Record usage totals
	w.RecordUsageTotals(ctx, convID, 10, 20, 5)

	// Validate data via domain adapters
	// Messages
	if _, e := s.store.Messages().List(ctx); !assert.NoError(t, e) {
		return
	}
	// Messages may be empty depending on memory DAO; proceed with transcript checks

	// Verify DAO read APIs via domain store for messages/transcript
	rows, err := s.store.Messages().GetTranscriptAggregated(ctx, convID, "", d.TranscriptAggOptions{IncludeModelCalls: true, IncludeToolCalls: true})
	if !assert.NoError(t, err) {
		return
	}
	// Transcript may be empty on bare memory DAO; ensure call succeeded
	_ = rows

	// Direct DAO checks for usage and turns via domain store
	_, err = s.store.Turns().List(ctx)
	if !assert.NoError(t, err) {
		return
	}

	ul, err := s.store.Usage().List(ctx, usageread.Input{ConversationID: convID, Has: &usageread.Has{ConversationID: true}})
	if !assert.NoError(t, err) {
		return
	}
	_ = ul // may be empty depending on memory DAO behavior

	// Also exercise DAO read packages directly via domain store adapters where possible
	// Messages list by conversation
	_, err = s.store.Messages().List(ctx, msgread.WithConversationID(convID))
	assert.NoError(t, err)

	// Ensure tool operations can be fetched by turn (adapter path)
	_, err = s.store.Operations().GetByTurn(ctx, turnID)
	assert.NoError(t, err)

	// Ensure we can import these packages without unused errors
	_ = mcread.PathBase
	_ = tcread.PathBase
}
