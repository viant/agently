package tool

import (
	"context"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/dao/conversation"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/datly/view"
)

func TestService(t *testing.T) {
	ctx := context.Background()

	goPath := os.Getenv("GOPATH")
	dbLocation := path.Join(goPath, "src/github.com/viant/agently/.db/agently.db")

	connector := view.NewConnector("agently", "sqlite", dbLocation)

	convSrv, err := conversation.New(ctx, connector)
	if !assert.NoError(t, err) {
		return
	}

	srv, err := New(ctx, connector)
	if !assert.NoError(t, err) {
		return
	}
	assert.NotNil(t, srv)

	var toolName = "test_tool"

	err = convSrv.AddMessage(context.Background(), "test_conv", memory.Message{
		Role:     "abc",
		Content:  "test content",
		ToolName: &toolName,
	})
	if !assert.Nil(t, err) {
		return
	}
	var args = `{"arg1": "value1", "arg2": "value2"}`
	err = srv.Add(ctx, &tool.Call{
		ConversationID: "test_conv",
		ToolName:       toolName,
		Arguments:      &args,
	})

	if !assert.Nil(t, err) {
		return
	}
	// Retrieve list (should not error even if empty)
	calls, err := srv.List(ctx, "test_conv")
	assert.NoError(t, err)
	assert.NotNil(t, calls)
}
