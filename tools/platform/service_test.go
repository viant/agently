package platform

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	runtimerequestctx "github.com/viant/agently-core/runtime/requestctx"
)

func TestService_ExitCodeDefaultsToZero(t *testing.T) {
	svc := New()
	out := &ExitCodeOutput{}
	err := svc.exitCode(context.Background(), &ExitCodeInput{ConversationID: "conv-1"}, out)
	require.NoError(t, err)
	require.Equal(t, "conv-1", out.ConversationID)
	require.Equal(t, 0, out.Code)
}

func TestService_SetExitCodeUsesConversationContext(t *testing.T) {
	svc := New()
	ctx := runtimerequestctx.WithConversationID(context.Background(), "conv-ctx")

	setOut := &ExitCodeOutput{}
	err := svc.setExitCode(ctx, &SetExitCodeInput{Code: 17}, setOut)
	require.NoError(t, err)
	require.Equal(t, "conv-ctx", setOut.ConversationID)
	require.Equal(t, 17, setOut.Code)

	getOut := &ExitCodeOutput{}
	err = svc.exitCode(context.Background(), &ExitCodeInput{ConversationID: "conv-ctx"}, getOut)
	require.NoError(t, err)
	require.Equal(t, 17, getOut.Code)
}
