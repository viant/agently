package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLLMContextPolicy verifies that LLMContextPolicy keeps only user and
// assistant roles while skipping summary/summarized statuses.
func TestLLMContextPolicy(t *testing.T) {
	ctx := context.Background()

	input := []Message{
		{ID: "u1", Role: "user", Content: "hi"},
		{ID: "a1", Role: "assistant", Content: "hello"},
		{ID: "t1", Role: "tool", Content: "internal", Status: "done"},
		{ID: "s1", Role: "system", Content: "meta"},
		{ID: "sm1", Role: "system", Content: "summary", Status: "summary"},
		{ID: "u2", Role: "user", Content: "next"},
	}

	expected := []Message{
		{ID: "u1", Role: "user", Content: "hi"},
		{ID: "a1", Role: "assistant", Content: "hello"},
		{ID: "u2", Role: "user", Content: "next"},
	}

	policy := LLMContextPolicy()
	actual, err := policy.Apply(ctx, input)
	assert.NoError(t, err)
	assert.EqualValues(t, expected, actual)
}
