package sql

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/internal/dao/message/write"
	"github.com/viant/agently/internal/testutil/dbtest"
)

type msgExpected struct {
	Id             string
	ConversationID string
	Role           string
	Type           string
	Content        string
	HasCreatedAt   bool
}

func toExpectedMsg(v *MessageView) msgExpected {
	return msgExpected{
		Id: v.Id, ConversationID: v.ConversationID, Role: v.Role, Type: v.Type, Content: v.Content, HasCreatedAt: v.CreatedAt != nil,
	}
}

func Test_MessageWrite_InsertUpdate_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     []dbtest.ParameterizedSQL
		patch    []*write.Message
		verifyID string
		expected msgExpected
	}

	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))))
	ddlPath := filepath.Join(repoRoot, "script", "schema.ddl")

	cases := []testCase{
		{
			name: "insert new message",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c1", "A"}},
			},
			patch: []*write.Message{func() *write.Message {
				m := &write.Message{Has: &write.MessageHas{}}
				m.SetId("m1")
				m.SetConversationID("c1")
				m.SetRole("user")
				m.SetType("text")
				m.SetContent("hello")
				return m
			}()},
			verifyID: "m1",
			expected: msgExpected{Id: "m1", ConversationID: "c1", Role: "user", Type: "text", Content: "hello", HasCreatedAt: true},
		},
		{
			name: "update message content",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c2", "A"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content) VALUES (?,?,?,?,?)", Params: []interface{}{"m2", "c2", "assistant", "text", "old"}},
			},
			patch: []*write.Message{func() *write.Message {
				m := &write.Message{Has: &write.MessageHas{}}
				m.SetId("m2")
				m.SetConversationID("c2")
				m.SetRole("assistant")
				m.SetType("text")
				m.SetContent("new")
				return m
			}()},
			verifyID: "m2",
			expected: msgExpected{Id: "m2", ConversationID: "c2", Role: "assistant", Type: "text", Content: "new", HasCreatedAt: true},
		},
		{
			name: "mixed upsert",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary) VALUES (?,?)", Params: []interface{}{"c3", "A"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content) VALUES (?,?,?,?,?)", Params: []interface{}{"m3", "c3", "user", "text", "hi"}},
			},
			patch: []*write.Message{
				func() *write.Message { // update m3
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("m3")
					m.SetConversationID("c3")
					m.SetRole("user")
					m.SetType("text")
					m.SetContent("hi2")
					return m
				}(),
				func() *write.Message { // insert m4
					m := &write.Message{Has: &write.MessageHas{}}
					m.SetId("m4")
					m.SetConversationID("c3")
					m.SetRole("assistant")
					m.SetType("text")
					m.SetContent("ok")
					return m
				}(),
			},
			verifyID: "m4",
			expected: msgExpected{Id: "m4", ConversationID: "c3", Role: "assistant", Type: "text", Content: "ok", HasCreatedAt: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-message-post")
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
			actual := toExpectedMsg(rows[0])
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
