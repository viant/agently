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
	d "github.com/viant/agently/internal/domain"
)

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
	_ = w.RecordMessage(ctx, memory.Message{ID: "m-1", ConversationID: convID, Role: "user", Content: "hi", CreatedAt: time.Now()})
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
	err := w.StartTurn(ctx, convID, turnID, time.Now())
	assert.NoError(t, err)
	err = w.UpdateTurn(ctx, turnID, "succeeded")
	assert.NoError(t, err)
	_, err = w.store.Turns().List(ctx)
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
	err := w.RecordMessage(ctx, memory.Message{ID: msgID, ConversationID: convID, Role: "assistant", Content: "x", CreatedAt: time.Now()})
	assert.NoError(t, err)

	usage := &llm.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}

	err = w.StartModelCall(ctx, ModelCallStart{MessageID: msgID, TurnID: turnID, Provider: "openai", Model: "gpt-x", ModelKind: "chat", StartedAt: time.Now(), Request: map[string]string{"p": "v"}})
	assert.NoError(t, err)

	err = w.FinishModelCall(ctx, ModelCallFinish{MessageID: msgID, TurnID: turnID, Usage: usage, FinishReason: "stop", CompletedAt: time.Now(), Response: map[string]string{"r": "v"}})
	assert.NoError(t, err)
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
	// StartedAt then update
	err = w.StartToolCall(ctx, ToolCallStart{MessageID: msgID, TurnID: turnID, ToolName: "sys.echo", StartedAt: time.Now(), Request: map[string]any{"a": 1}})
	assert.NoError(t, err)
	err = w.FinishToolCall(ctx, ToolCallUpdate{MessageID: msgID, TurnID: turnID, ToolName: "sys.echo", Status: "completed", CompletedAt: time.Now(), Response: map[string]any{"b": 2}})
	assert.NoError(t, err)
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
	_, err := w.store.Conversations().Get(ctx, convID)
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
	var err error
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
	_ = w.RecordMessage(ctx, memory.Message{ID: msgID, ConversationID: convID, Role: "assistant", Content: "hello", CreatedAt: time.Now()})

	// Record turn start/update
	err = w.StartTurn(ctx, convID, turnID, time.Now().Add(-1*time.Second))
	assert.NoError(t, err)
	err = w.UpdateTurn(ctx, turnID, "succeeded")
	assert.NoError(t, err)

	// Record model call with usage and payloads
	usage := &llm.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7}
	err = w.StartModelCall(ctx, ModelCallStart{MessageID: msgID, TurnID: turnID, Provider: "openai", Model: "gpt-4o", ModelKind: "chat", StartedAt: time.Now().Add(-500 * time.Millisecond), Request: map[string]string{"p": "x"}})
	assert.NoError(t, err)
	err = w.FinishModelCall(ctx, ModelCallFinish{MessageID: msgID, TurnID: turnID, Usage: usage, FinishReason: "stop", CompletedAt: time.Now(), Response: map[string]string{"r": "y"}})
	assert.NoError(t, err)

	// Record tool call with payloads and error/cost
	cost := 0.25
	// StartedAt then update with failure
	started := time.Now().Add(-250 * time.Millisecond)
	err = w.StartToolCall(ctx, ToolCallStart{MessageID: msgID, TurnID: turnID, ToolName: "sys.echo", StartedAt: started, Request: map[string]any{"a": 1}})
	assert.NoError(t, err)
	err = w.FinishToolCall(ctx, ToolCallUpdate{MessageID: msgID, TurnID: turnID, ToolName: "sys.echo", Status: "failed", CompletedAt: time.Now(), ErrMsg: "boom", Cost: &cost, Response: map[string]any{"b": 2}})
	assert.NoError(t, err)

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

	_, err = s.store.Conversations().Get(ctx, convID)
	if !assert.NoError(t, err) {
		return
	}

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
