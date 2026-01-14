package conversation

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	convcli "github.com/viant/agently/client/conversation"
	convwrite "github.com/viant/agently/pkg/agently/conversation/write"
)

func TestConversationSince_IncludesTurnsByQueueSeq_SQLite(t *testing.T) {
	ctx := context.Background()

	// Use an isolated workspace with a temp SQLite DB.
	tmp := t.TempDir()
	// Ensure path exists; set AGENTLY_WORKSPACE so NewDatly picks sqlite.
	_ = os.Setenv("AGENTLY_WORKSPACE", tmp)
	// Ensure no external DB overrides.
	_ = os.Unsetenv("AGENTLY_DB_DRIVER")
	_ = os.Unsetenv("AGENTLY_DB_DSN")
	t.Cleanup(func() {
		sharedDAO = nil
		daoOnce = sync.Once{}
	})

	dao, err := NewDatly(ctx)
	if err != nil {
		t.Fatalf("NewDatly: %v", err)
	}
	svc, err := New(ctx, dao)
	if err != nil {
		t.Fatalf("conversation.New: %v", err)
	}

	convID := "conv_since_queue_seq"
	conv := &convcli.MutableConversation{}
	conv.Has = &convwrite.ConversationHas{}
	conv.SetId(convID)
	conv.SetVisibility("public")
	if err := svc.PatchConversations(ctx, conv); err != nil {
		t.Fatalf("PatchConversations: %v", err)
	}

	// Simulate user reordering queued turns by queue_seq:
	// - anchorTurn1 has later created_at but smaller queue_seq (executed earlier)
	// - turn2 has earlier created_at but larger queue_seq (executed later, currently running)
	anchorID := "turn_anchor"
	runningID := "turn_running"

	running := convcli.NewTurn()
	running.SetId(runningID)
	running.SetConversationID(convID)
	running.SetStatus("running")
	running.SetQueueSeq(200)
	if err := svc.PatchTurn(ctx, running); err != nil {
		t.Fatalf("PatchTurn running: %v", err)
	}

	anchor := convcli.NewTurn()
	anchor.SetId(anchorID)
	anchor.SetConversationID(convID)
	anchor.SetStatus("succeeded")
	anchor.SetQueueSeq(100)
	if err := svc.PatchTurn(ctx, anchor); err != nil {
		t.Fatalf("PatchTurn anchor: %v", err)
	}

	// Force created_at ordering to be opposite of queue_seq ordering.
	dbPath := filepath.Join(tmp, "db", "agently.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, "UPDATE turn SET created_at = ? WHERE id = ?", "2026-01-01T00:00:00Z", runningID); err != nil {
		t.Fatalf("set created_at (running): %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE turn SET created_at = ? WHERE id = ?", "2026-02-01T00:00:00Z", anchorID); err != nil {
		t.Fatalf("set created_at (anchor): %v", err)
	}

	got, err := svc.GetConversation(ctx, convID, convcli.WithSince(anchorID))
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if got == nil {
		t.Fatalf("expected conversation, got nil")
	}

	seen := map[string]bool{}
	for _, turn := range got.GetTranscript() {
		if turn == nil {
			continue
		}
		seen[turn.Id] = true
	}

	if !seen[anchorID] {
		t.Fatalf("expected transcript to include anchor turn %q", anchorID)
	}
	if !seen[runningID] {
		t.Fatalf("expected transcript to include running turn %q (queue_seq ordering), but it was omitted", runningID)
	}
}
