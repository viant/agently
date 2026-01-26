package read

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently/internal/testutil/dbtest"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
	_ "modernc.org/sqlite"
)

func TestActiveTurnRead_SQLite(t *testing.T) {
	type testCase struct {
		name           string
		conversationID string
		seed           func(t *testing.T, db *sql.DB)
		expectID       string
		expectStatus   string
	}

	now := time.Now().UTC()

	cases := []testCase{
		{
			name:           "no active turn returns empty",
			conversationID: "c1",
			seed: func(t *testing.T, db *sql.DB) {
				t.Helper()
				insertConversation(t, db, "c1")
				_, err := db.Exec(
					`INSERT INTO turn (id, conversation_id, created_at, status) VALUES (?, ?, ?, ?)`,
					"t1", "c1", now.Add(-2*time.Minute), "queued",
				)
				require.NoError(t, err)
				insertConversation(t, db, "c2")
				_, err = db.Exec(
					`INSERT INTO turn (id, conversation_id, created_at, status) VALUES (?, ?, ?, ?)`,
					"t2", "c2", now.Add(-1*time.Minute), "running",
				)
				require.NoError(t, err)
			},
		},
		{
			name:           "picks running turn",
			conversationID: "c3",
			seed: func(t *testing.T, db *sql.DB) {
				t.Helper()
				insertConversation(t, db, "c3")
				_, err := db.Exec(
					`INSERT INTO turn (id, conversation_id, created_at, status) VALUES (?, ?, ?, ?)`,
					"t3", "c3", now.Add(-1*time.Minute), "running",
				)
				require.NoError(t, err)
			},
			expectID:     "t3",
			expectStatus: "running",
		},
		{
			name:           "picks latest active",
			conversationID: "c4",
			seed: func(t *testing.T, db *sql.DB) {
				t.Helper()
				insertConversation(t, db, "c4")
				_, err := db.Exec(
					`INSERT INTO turn (id, conversation_id, created_at, status) VALUES (?, ?, ?, ?)`,
					"t4", "c4", now.Add(-2*time.Minute), "running",
				)
				require.NoError(t, err)
				_, err = db.Exec(
					`INSERT INTO turn (id, conversation_id, created_at, status) VALUES (?, ?, ?, ?)`,
					"t5", "c4", now.Add(-1*time.Minute), "waiting_for_user",
				)
				require.NoError(t, err)
			},
			expectID:     "t5",
			expectStatus: "waiting_for_user",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-turn-read")
			t.Cleanup(cleanup)
			dbtest.LoadSQLiteSchema(t, db)

			if tc.seed != nil {
				tc.seed(t, db)
			}

			ctx := context.Background()
			dao, err := newDatlyService(ctx, dbPath)
			require.NoError(t, err)

			require.NoError(t, DefineActiveTurnComponent(ctx, dao))

			in := &ActiveTurnInput{ConversationID: tc.conversationID, Has: &ActiveTurnInputHas{ConversationID: true}}
			out := &ActiveTurnOutput{}
			_, err = dao.Operate(ctx,
				datly.WithPath(contract.NewPath("GET", ActiveTurnPathURI)),
				datly.WithInput(in),
				datly.WithOutput(out),
			)
			require.NoError(t, err)

			if tc.expectID == "" {
				require.Len(t, out.Data, 0)
				return
			}
			require.Len(t, out.Data, 1)
			require.Equal(t, tc.expectID, out.Data[0].Id)
			require.Equal(t, tc.expectStatus, out.Data[0].Status)
		})
	}
}

func newDatlyService(ctx context.Context, dbPath string) (*datly.Service, error) {
	svc, err := datly.New(ctx)
	if err != nil {
		return nil, err
	}
	conn := view.NewConnector("agently", "sqlite", dbPath)
	if err := svc.AddConnectors(ctx, conn); err != nil {
		return nil, err
	}
	return svc, nil
}

func insertConversation(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO conversation (id) VALUES (?)`, id)
	require.NoError(t, err)
}
