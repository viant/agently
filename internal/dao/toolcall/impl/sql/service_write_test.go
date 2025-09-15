//go:build cgo

package sql

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	w "github.com/viant/agently/internal/dao/toolcall/write"
	"github.com/viant/agently/internal/testutil/dbtest"
	"github.com/viant/datly"
	"github.com/viant/datly/view"
	_ "modernc.org/sqlite"
)

func newDatlySvc(t *testing.T, dbPath string) *Service {
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
	return New(context.Background(), srv)
}

type expected struct {
	MessageID, OpID, ToolName, Status string
	Attempt                           int
}

func toExpected(v *ToolCallView) expected {
	return expected{MessageID: v.MessageID, OpID: v.OpID, ToolName: v.ToolName, Status: v.Status, Attempt: v.Attempt}
}

func Test_ToolCallWrite_InsertUpdate_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     []dbtest.ParameterizedSQL
		patch    []*w.ToolCall
		verifyID string
		expected expected
	}

	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename)))))))
	ddlPath := filepath.Join(repoRoot, "internal", "script", "schema.ddl")

	cases := []testCase{
		{
			name: "insert new tool call (defaults attempt=1)",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c1", "A"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content) VALUES (?,?,?,?,?)", Params: []interface{}{"m1", "c1", "tool", "tool_op", "x"}},
			},
			patch: []*w.ToolCall{func() *w.ToolCall {
				tc := &w.ToolCall{Has: &w.ToolCallHas{}}
				tc.SetMessageID("m1")
				tc.SetOpID("op-1")
				tc.SetToolName("search")
				tc.SetToolKind("general")
				tc.SetStatus("completed")
				return tc
			}()},
			verifyID: "m1",
			expected: expected{MessageID: "m1", OpID: "op-1", ToolName: "search", Status: "completed", Attempt: 1},
		},
		{
			name: "update tool call status",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c2", "A"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content) VALUES (?,?,?,?,?)", Params: []interface{}{"m2", "c2", "tool", "tool_op", "x"}},
				{SQL: "INSERT INTO tool_call (message_id, op_id, attempt, tool_name, tool_kind, status) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m2", "op-1", 1, "search", "general", "running"}},
			},
			patch: []*w.ToolCall{func() *w.ToolCall {
				tc := &w.ToolCall{Has: &w.ToolCallHas{}}
				tc.SetMessageID("m2")
				tc.SetOpID("op-1")
				tc.SetToolName("search")
				tc.SetToolKind("general")
				tc.SetStatus("completed")
				return tc
			}()},
			verifyID: "m2",
			expected: expected{MessageID: "m2", OpID: "op-1", ToolName: "search", Status: "completed", Attempt: 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-toolcall-post")
			defer cleanup()
			dbtest.LoadDDLFromFile(t, db, ddlPath)
			dbtest.ExecAll(t, db, tc.seed)

			svc := newDatlySvc(t, dbPath)
			out, err := svc.Patch(context.Background(), tc.patch...)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.NotNil(t, out) {
				return
			}

			rows, err := svc.List(context.Background(), WithMessageIDs(tc.verifyID))
			if !assert.Nil(t, err) {
				return
			}
			if !assert.True(t, len(rows) == 1) {
				return
			}
			actual := toExpected(rows[0])
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
