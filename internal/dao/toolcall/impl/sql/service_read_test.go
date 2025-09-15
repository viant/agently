//go:build cgo

package sql

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/internal/testutil/dbtest"
	"github.com/viant/datly"
	"github.com/viant/datly/view"
	_ "modernc.org/sqlite"
)

func newDatlyRead(t *testing.T, dbPath string) *datly.Service {
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

func TestToolCall_List(t *testing.T) {
	type testCase struct {
		name     string
		seed     []dbtest.ParameterizedSQL
		exec     func(srv *Service) (interface{}, error)
		expected interface{}
	}

	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename)))))))
	ddlPath := filepath.Join(repoRoot, "internal", "script", "schema.ddl")

	cases := []testCase{
		{
			name: "by conversation",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c1", "A"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content) VALUES (?,?,?,?,?)", Params: []interface{}{"m1", "c1", "tool", "tool_op", "x"}},
				{SQL: "INSERT INTO tool_call (message_id, op_id, attempt, tool_name, tool_kind, status) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m1", "op1", 1, "search", "general", "completed"}},
			},
			exec: func(srv *Service) (interface{}, error) {
				rows, err := srv.List(context.Background(), WithConversationID("c1"))
				if err != nil {
					return nil, err
				}
				var ids []string
				for _, r := range rows {
					ids = append(ids, r.MessageID)
				}
				return ids, nil
			},
			expected: []string{"m1"},
		},
		{
			name: "by tool name",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c1", "A"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content) VALUES (?,?,?,?,?)", Params: []interface{}{"m1", "c1", "tool", "tool_op", "x"}},
				{SQL: "INSERT INTO tool_call (message_id, op_id, attempt, tool_name, tool_kind, status) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m1", "op1", 1, "search", "general", "completed"}},
			},
			exec: func(srv *Service) (interface{}, error) {
				rows, err := srv.List(context.Background(), WithToolName("search"))
				if err != nil {
					return nil, err
				}
				return len(rows), nil
			},
			expected: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-toolcall-read")
			defer cleanup()
			dbtest.LoadDDLFromFile(t, db, ddlPath)
			dbtest.ExecAll(t, db, tc.seed)
			d := newDatlyRead(t, dbPath)
			svc := New(context.Background(), d)
			actual, err := tc.exec(svc)
			if !assert.Nil(t, err) {
				return
			}
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
