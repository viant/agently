package resources

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	agmodel "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/memory"
	convmem "github.com/viant/agently/internal/service/conversation/memory"
)

func TestRead_RootID_Local_TextMode_MaxBytesApplied_WhenNoMaxLines(t *testing.T) {
	rootURL := tempDirURL(t)
	content := strings.Repeat("x", 9000)
	require.Greater(t, len(content), 8192)
	writeFile(t, rootURL, "foo.txt", content)

	agentID := "test-agent"
	convClient := convmem.New()
	conv := apiconv.NewConversation()
	conv.SetId("conv-1")
	conv.SetAgentId(agentID)
	require.NoError(t, convClient.PatchConversations(context.Background(), conv))

	svc := New(nil,
		WithConversationClient(convClient),
		WithAgentFinder(&testAgentFinder{agent: &agmodel.Agent{
			Identity: agmodel.Identity{ID: agentID},
			Resources: []*agmodel.Resource{
				{ID: "local", URI: rootURL, Role: "user"},
			},
		}}),
	)

	ctx := memory.WithConversationID(context.Background(), "conv-1")
	var out ReadOutput
	err := svc.read(ctx, &ReadInput{
		RootID:   "local",
		Path:     "foo.txt",
		Mode:     "text",
		MaxBytes: 8192,
	}, &out)
	require.NoError(t, err)

	assert.Equal(t, "foo.txt", out.Path)
	assert.Equal(t, "text", out.ModeApplied)
	assert.Equal(t, len(content), out.Size)
	assert.Equal(t, 8192, out.Returned)
	assert.Equal(t, len(content)-8192, out.Remaining)
	assert.Equal(t, content[:8192], out.Content)
	if assert.NotNil(t, out.Continuation) && assert.NotNil(t, out.Continuation.NextRange) && assert.NotNil(t, out.Continuation.NextRange.Bytes) {
		assert.Equal(t, 8192, out.Continuation.NextRange.Bytes.Offset)
		assert.Equal(t, len(content)-8192, out.Continuation.NextRange.Bytes.Length)
	}
}
