package sql

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/internal/testutil/dbtest"
	"github.com/viant/datly"
	"github.com/viant/datly/view"
	_ "modernc.org/sqlite"
)

func newDatly(t *testing.T, dbPath string) *datly.Service {
	t.Helper()
	srv, err := datly.New(context.Background())
	if err != nil {
		t.Fatalf("datly new: %v", err)
	}
	if err := srv.AddConnectors(context.Background(), view.NewConnector("agently", "sqlite", dbPath)); err != nil {
		t.Fatalf("add connector: %v", err)
	}
	if err := Register(context.Background(), srv); err != nil {
		t.Fatalf("register: %v", err)
	}
	return srv
}

func TestService_List(t *testing.T) {
	type testCase struct {
		name     string
		seed     []dbtest.ParameterizedSQL
		opts     []InputOption
		expected []string
	}

	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename)))))))
	ddlPath := filepath.Join(repoRoot, "internal", "script", "schema.ddl")

	cases := []testCase{
		{
			name: "by status",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c1", "A"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t1", "c1", "running"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t2", "c1", "failed"}},
			},
			opts:     []InputOption{WithConversationID("c1"), WithStatus("failed")},
			expected: []string{"t2"},
		},
		{
			name: "by conversation",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c1", "A"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status, created_at) VALUES (?,?,?,?)", Params: []interface{}{"t1", "c1", "running", "2024-01-01 10:00:00"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status, created_at) VALUES (?,?,?,?)", Params: []interface{}{"t2", "c1", "succeeded", "2024-01-01 12:00:00"}},
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c2", "B"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t3", "c2", "running"}},
			},
			opts:     []InputOption{WithConversationID("c1")},
			expected: []string{"t1", "t2"},
		},

		{
			name: "since filter",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c1", "A"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status, created_at) VALUES (?,?,?,?)", Params: []interface{}{"t1", "c1", "running", "2024-01-01 10:00:00"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status, created_at) VALUES (?,?,?,?)", Params: []interface{}{"t2", "c1", "running", "2024-01-01 12:00:00"}},
			},
			opts:     []InputOption{WithConversationID("c1"), WithSince(mustParse("2006-01-02 15:04:05", "2024-01-01 11:00:00"))},
			expected: []string{"t2"},
		},
		{
			name: "ids IN",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c1", "A"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t1", "c1", "running"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t2", "c1", "running"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t3", "c1", "running"}},
			},
			opts:     []InputOption{WithConversationID("c1"), WithIDs("t1", "t3")},
			expected: []string{"t1", "t3"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-turn")
			defer cleanup()
			dbtest.LoadDDLFromFile(t, db, ddlPath)
			dbtest.ExecAll(t, db, tc.seed)

			d := newDatly(t, dbPath)
			svc := New(context.Background(), d)

			rows, err := svc.List(context.Background(), tc.opts...)
			if !assert.Nil(t, err) {
				return
			}
			var ids []string
			for _, r := range rows {
				ids = append(ids, r.Id)
			}
			assert.EqualValues(t, tc.expected, ids)
		})
	}
}

func mustParse(layout, value string) time.Time { t, _ := time.Parse(layout, value); return t }
