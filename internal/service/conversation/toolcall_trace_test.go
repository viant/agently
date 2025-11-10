package conversation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	convcli "github.com/viant/agently/client/conversation"
	convwrite "github.com/viant/agently/pkg/agently/conversation/write"
	toolcallwrite "github.com/viant/agently/pkg/agently/toolcall/write"
)

// TestToolCallTraceByOp_SQLite verifies that the Datly view for reading
// tool_call by op_id returns the persisted trace_id (LLM response.id anchor).
func TestToolCallTraceByOp_SQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Use an isolated workspace with a temp SQLite DB.
	tmp := t.TempDir()
	// Ensure path exists; set AGENTLY_WORKSPACE so NewDatly picks sqlite.
	_ = os.Setenv("AGENTLY_WORKSPACE", tmp)
	// Ensure no external DB overrides.
	_ = os.Unsetenv("AGENTLY_DB_DRIVER")
	_ = os.Unsetenv("AGENTLY_DB_DSN")

	// Create Datly service and conversation API.
	dao, err := NewDatly(ctx)
	if err != nil {
		t.Fatalf("NewDatly: %v", err)
	}
	svc, err := New(ctx, dao)
	if err != nil {
		t.Fatalf("conversation.New: %v", err)
	}

	// Prepare minimal conversation/message/tool_call rows via write components.
	convID := "conv_trace_test"
	conv := &convcli.MutableConversation{}
	// initialize Has marker for setters
	conv.Has = &convwrite.ConversationHas{}
	conv.SetId(convID)
	conv.SetVisibility("private")
	if err := svc.PatchConversations(ctx, conv); err != nil {
		t.Fatalf("PatchConversations: %v", err)
	}

	msgID := "msg_1"
	now := time.Now()
	msg := convcli.NewMessage()
	msg.SetId(msgID)
	msg.SetConversationID(convID)
	msg.SetRole("tool")
	msg.SetType("tool_op")
	msg.SetCreatedAt(now)
	if err := svc.PatchMessage(ctx, (*convcli.MutableMessage)(msg)); err != nil {
		t.Fatalf("PatchMessage: %v", err)
	}

	opID := "call_abc123"
	trace := "resp_test_anchor_001"
	tc := &convcli.MutableToolCall{}
	tc.SetMessageID(msgID)
	tc.SetOpID(opID)
	tc.SetAttempt(1)
	tc.SetToolName("test/tool")
	tc.SetToolKind("general")
	tc.SetStatus("completed")
	// Persist the trace (anchor)
	tc.TraceID = &trace
	tc.Has = &toolcallwrite.ToolCallHas{TraceID: true}
	if err := svc.PatchToolCall(ctx, tc); err != nil {
		t.Fatalf("PatchToolCall: %v", err)
	}

	// Exercise the read path
	got, err := svc.ToolCallTraceByOp(ctx, convID, opID)
	if err != nil {
		t.Fatalf("ToolCallTraceByOp error: %v", err)
	}
	if got != trace {
		t.Fatalf("expected trace %q, got %q", trace, got)
	}

	// sanity: DB file created
	if _, err := os.Stat(filepath.Join(tmp, "db", "agently.db")); err != nil {
		t.Fatalf("expected sqlite db file: %v", err)
	}
}
