package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	read "github.com/viant/agently/internal/dao/usage/read"
	write "github.com/viant/agently/internal/dao/usage/write"
)

func TestMemoryUsage_Aggregate_List(t *testing.T) {
	svc := New()
	t0 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)
	svc.SeedCall(modelCall{ConversationID: "c1", Provider: "openai", Model: "gpt", TotalTokens: 10, PromptTokens: 6, CompletionTokens: 4, Cost: 0.01, CacheHit: 1, StartedAt: &t0, CompletedAt: &t1})
	svc.SeedCall(modelCall{ConversationID: "c1", Provider: "openai", Model: "gpt", TotalTokens: 5, PromptTokens: 3, CompletionTokens: 2, Cost: 0.02, CacheHit: 0, StartedAt: &t1, CompletedAt: &t1})

	rows, err := svc.List(context.Background(), read.Input{ConversationID: "c1", Has: &read.Has{ConversationID: true}})
	if !assert.Nil(t, err) {
		return
	}
	if !assert.Equal(t, 1, len(rows)) {
		return
	}
	got := rows[0]
	if assert.NotNil(t, got.TotalTokens) {
		assert.Equal(t, 15, *got.TotalTokens)
	}
	if assert.NotNil(t, got.CallsCount) {
		assert.Equal(t, 2, *got.CallsCount)
	}
}

func TestMemoryUsage_PatchTotals(t *testing.T) {
	svc := New()
	out, err := svc.Patch(context.Background(), &write.Usage{Id: "c1", UsageInputTokens: 10, UsageOutputTokens: 3, UsageEmbeddingTokens: 1, Has: &write.UsageHas{Id: true, UsageInputTokens: true, UsageOutputTokens: true, UsageEmbeddingTokens: true}})
	if !assert.Nil(t, err) {
		return
	}
	assert.NotNil(t, out)
	// totals stored internally; this test ensures Patch does not error and can be called again
	out, err = svc.Patch(context.Background(), &write.Usage{Id: "c1", UsageInputTokens: 20, Has: &write.UsageHas{Id: true, UsageInputTokens: true}})
	if !assert.Nil(t, err) {
		return
	}
	assert.NotNil(t, out)
}
