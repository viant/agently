package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	conv "github.com/viant/agently/internal/dao/conversation"
	"github.com/viant/agently/internal/dao/conversation/read"
	w "github.com/viant/agently/internal/dao/conversation/write"
)

func TestMemoryConversation_BasicPatchAndRead(t *testing.T) {
	svc := New()

	alpha := "Alpha"
	agentA := "AgentA"

	// insert
	out, err := svc.PatchConversations(context.Background(), func() *w.Conversation {
		c := &w.Conversation{Has: &w.ConversationHas{}}
		c.SetId("c1")
		c.SetSummary(alpha)
		c.SetAgentName(agentA)
		return c
	}())
	if !assert.Nil(t, err) {
		return
	}
	assert.NotNil(t, out)

	// read by id
	rows, err := svc.GetConversations(context.Background(), read.WithID("c1"))
	if !assert.Nil(t, err) {
		return
	}
	if !assert.Equal(t, 1, len(rows)) {
		return
	}
	got := rows[0]
	if !assert.NotNil(t, got.CreatedAt) {
		return
	}
	if !assert.NotNil(t, got.LastActivity) {
		return
	}
	if !assert.Equal(t, "c1", got.Id) {
		return
	}
	if !assert.NotNil(t, got.Summary) || !assert.Equal(t, alpha, *got.Summary) {
		return
	}
	if !assert.NotNil(t, got.AgentName) || !assert.Equal(t, agentA, *got.AgentName) {
		return
	}

	// update usage and summary
	beta := "Beta"
	out, err = svc.PatchConversations(context.Background(), func() *w.Conversation {
		c := &w.Conversation{Has: &w.ConversationHas{}}
		c.SetId("c1")
		c.SetSummary(beta)
		c.SetUsageInputTokens(10)
		return c
	}())
	if !assert.Nil(t, err) {
		return
	}
	assert.NotNil(t, out)

	rows, err = svc.GetConversations(context.Background(), read.WithID("c1"))
	if !assert.Nil(t, err) {
		return
	}
	got = rows[0]
	if !assert.NotNil(t, got.UsageInputTokens) {
		return
	}
	assert.Equal(t, 10, *got.UsageInputTokens)
	if !assert.NotNil(t, got.Summary) {
		return
	}
	assert.Equal(t, beta, *got.Summary)

	// list filter by summary contains
	rows, err = svc.GetConversations(context.Background(), read.WithSummaryContains("Be"))
	if !assert.Nil(t, err) {
		return
	}
	assert.Equal(t, 1, len(rows))

	// ensure it implements API
	var _ conv.API = svc
}
