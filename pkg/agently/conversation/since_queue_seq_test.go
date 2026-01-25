package conversation_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	convcli "github.com/viant/agently/client/conversation"
	convsvc "github.com/viant/agently/internal/service/conversation"
	sqlitesvc "github.com/viant/agently/internal/service/sqlite"
	convwrite "github.com/viant/agently/pkg/agently/conversation/write"
	"github.com/viant/datly"
	"github.com/viant/datly/view"
	_ "modernc.org/sqlite"
)

func TestConversationSince_IncludesTurnsByQueueSeq_SQLite(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	sqliteSvc := sqlitesvc.New(root)
	dsn, err := sqliteSvc.Ensure(ctx)
	require.NoError(t, err)

	dao, err := datly.New(ctx)
	require.NoError(t, err)
	require.NoError(t, dao.AddConnectors(ctx, view.NewConnector("agently", "sqlite", dsn)))

	svc, err := convsvc.New(ctx, dao)
	require.NoError(t, err)

	convID := "conv_since_queue_seq"
	conv := &convcli.MutableConversation{}
	conv.Has = &convwrite.ConversationHas{}
	conv.SetId(convID)
	conv.SetVisibility("public")
	require.NoError(t, svc.PatchConversations(ctx, conv))

	anchorID := "turn_anchor"
	runningID := "turn_running"

	running := convcli.NewTurn()
	running.SetId(runningID)
	running.SetConversationID(convID)
	running.SetStatus("running")
	running.SetQueueSeq(200)
	require.NoError(t, svc.PatchTurn(ctx, running))

	anchor := convcli.NewTurn()
	anchor.SetId(anchorID)
	anchor.SetConversationID(convID)
	anchor.SetStatus("succeeded")
	anchor.SetQueueSeq(100)
	require.NoError(t, svc.PatchTurn(ctx, anchor))

	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.ExecContext(ctx, "UPDATE turn SET created_at = ? WHERE id = ?", "2026-01-01T00:00:00Z", runningID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "UPDATE turn SET created_at = ? WHERE id = ?", "2026-02-01T00:00:00Z", anchorID)
	require.NoError(t, err)

	got, err := svc.GetConversation(ctx, convID, convcli.WithSince(anchorID))
	require.NoError(t, err)
	require.NotNil(t, got)

	seen := map[string]bool{}
	for _, turn := range got.GetTranscript() {
		if turn == nil {
			continue
		}
		seen[turn.Id] = true
	}

	require.True(t, seen[anchorID], "expected transcript to include anchor turn")
	require.True(t, seen[runningID], "expected transcript to include running turn by queue_seq ordering")
}
