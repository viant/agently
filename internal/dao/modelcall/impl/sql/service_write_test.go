//go:build cgo

package sql

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	w "github.com/viant/agently/internal/dao/modelcall/write"
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

type expected struct {
	MessageID string
	Provider  string
	Model     string
}

func toExpected(v *ModelCallView) expected {
	return expected{MessageID: v.MessageID, Provider: v.Provider, Model: v.Model}
}

func Test_ModelCallWrite_InsertUpdate_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     []dbtest.ParameterizedSQL
		patch    []*w.ModelCall
		verifyID string
		expected expected
	}

	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))))
	ddlPath := filepath.Join(repoRoot, "script", "schema.ddl")

	cases := []testCase{
		{
			name: "insert new model call",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c1", "A"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content) VALUES (?,?,?,?,?)", Params: []interface{}{"m1", "c1", "assistant", "text", "answer"}},
			},
			patch: []*w.ModelCall{func() *w.ModelCall {
				mc := &w.ModelCall{Has: &w.ModelCallHas{}}
				mc.SetMessageID("m1")
				mc.SetProvider("openai")
				mc.SetModel("gpt-4o-mini")
				mc.SetModelKind("chat")
				return mc
			}()},
			verifyID: "m1",
			expected: expected{MessageID: "m1", Provider: "openai", Model: "gpt-4o-mini"},
		},
		{
			name: "update existing model call",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c2", "A"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content) VALUES (?,?,?,?,?)", Params: []interface{}{"m2", "c2", "assistant", "text", "answer"}},
				{SQL: "INSERT INTO model_calls (message_id, provider, model, model_kind) VALUES (?,?,?,?)", Params: []interface{}{"m2", "openai", "gpt-4o-mini", "chat"}},
			},
			patch: []*w.ModelCall{func() *w.ModelCall {
				mc := &w.ModelCall{Has: &w.ModelCallHas{}}
				mc.SetMessageID("m2")
				mc.SetProvider("google")
				mc.SetModel("gemini")
				mc.SetModelKind("chat")
				return mc
			}()},
			verifyID: "m2",
			expected: expected{MessageID: "m2", Provider: "google", Model: "gemini"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-modelcall-post")
			defer cleanup()
			dbtest.LoadDDLFromFile(t, db, ddlPath)
			dbtest.ExecAll(t, db, tc.seed)

			d := newDatly(t, dbPath)
			svc := New(context.Background(), d)

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
