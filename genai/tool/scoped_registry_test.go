package tool

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
)

type captureRegistry struct {
	lastConversationID string
}

func (c *captureRegistry) Definitions() []llm.ToolDefinition { return nil }
func (c *captureRegistry) MatchDefinition(string) []*llm.ToolDefinition {
	return nil
}
func (c *captureRegistry) GetDefinition(string) (*llm.ToolDefinition, bool) { return nil, false }
func (c *captureRegistry) MustHaveTools([]string) ([]llm.Tool, error)       { return nil, nil }
func (c *captureRegistry) Execute(ctx context.Context, _ string, _ map[string]interface{}) (string, error) {
	c.lastConversationID = memory.ConversationIDFromContext(ctx)
	return "", nil
}
func (c *captureRegistry) SetDebugLogger(io.Writer) {}
func (c *captureRegistry) Initialize(context.Context) {
}

func TestScopedRegistry_InjectsConversationID_WhenModelMessageIDPresent(t *testing.T) {
	inner := &captureRegistry{}
	reg := WithConversation(inner, "conv-123")

	ctx := context.WithValue(context.Background(), memory.ModelMessageIDKey, "msg-1")
	_, err := reg.Execute(ctx, "noop", nil)
	assert.NoError(t, err)
	assert.Equal(t, "conv-123", inner.lastConversationID)
}
