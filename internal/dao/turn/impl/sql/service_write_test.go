package sql

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	w "github.com/viant/agently/internal/dao/turn/write"
	"github.com/viant/agently/internal/testutil/dbtest"
)

type turnExpected struct {
	Id             string
	ConversationID string
	Status         string
	HasCreatedAt   bool
}

func toExpectedTurn(v *TurnView) turnExpected {
	return turnExpected{
		Id:             v.Id,
		ConversationID: v.ConversationID,
		Status:         v.Status,
		HasCreatedAt:   v.CreatedAt != nil,
	}
}

func Test_TurnWrite_InsertUpdate_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     []dbtest.ParameterizedSQL
		patch    []*w.Turn
		verifyID string
		expected turnExpected
	}

	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename)))))))
	ddlPath := filepath.Join(repoRoot, "internal", "script", "schema.ddl")

	cases := []testCase{
		{
			name: "insert new turn",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c1", "A"}},
			},
			patch: []*w.Turn{func() *w.Turn {
				t := &w.Turn{Has: &w.TurnHas{}}
				t.SetId("t1")
				t.SetConversationID("c1")
				t.SetStatus("running")
				return t
			}()},
			verifyID: "t1",
			expected: turnExpected{Id: "t1", ConversationID: "c1", Status: "running", HasCreatedAt: true},
		},
		{
			name: "update existing turn status",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c2", "A"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t2", "c2", "pending"}},
			},
			patch: []*w.Turn{func() *w.Turn {
				t := &w.Turn{Has: &w.TurnHas{}}
				t.SetId("t2")
				t.SetConversationID("c2")
				t.SetStatus("succeeded")
				return t
			}()},
			verifyID: "t2",
			expected: turnExpected{Id: "t2", ConversationID: "c2", Status: "succeeded", HasCreatedAt: true},
		},
		{
			name: "mixed upsert",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c3", "A"}},
				{SQL: "INSERT INTO turn (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t3", "c3", "pending"}},
			},
			patch: []*w.Turn{
				func() *w.Turn { // update t3
					t := &w.Turn{Has: &w.TurnHas{}}
					t.SetId("t3")
					t.SetConversationID("c3")
					t.SetStatus("failed")
					return t
				}(),
				func() *w.Turn { // insert t4
					t := &w.Turn{Has: &w.TurnHas{}}
					t.SetId("t4")
					t.SetConversationID("c3")
					t.SetStatus("running")
					return t
				}(),
			},
			verifyID: "t4",
			expected: turnExpected{Id: "t4", ConversationID: "c3", Status: "running", HasCreatedAt: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-turn-post")
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

			rows, err := svc.List(context.Background(), WithIDs(tc.verifyID))
			if !assert.Nil(t, err) {
				return
			}
			if !assert.True(t, len(rows) == 1) {
				return
			}
			actual := toExpectedTurn(rows[0])
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
