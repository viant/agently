//go:build cgo

package sql

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	ur "github.com/viant/agently/internal/dao/usage/read"
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
	if err := ur.DefineComponent(context.Background(), srv); err != nil {
		t.Fatalf("register: %v", err)
	}
	return srv
}

func TestUsage_View(t *testing.T) {
	type testCase struct {
		name           string
		seed           []dbtest.ParameterizedSQL
		conversationID string
		expected       []struct {
			provider string
			model    string
			total    int
		}
	}

	// resolve DDL
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename)))))))
	ddlPath := filepath.Join(repoRoot, "internal", "script", "schema.ddl")

	cases := []testCase{
		{
			name:           "aggregate per provider/model per conversation",
			conversationID: "c1",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"c1", "X", "A"}},
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"c2", "Y", "B"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m1", "c1", "assistant", "text", "a", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m2", "c1", "assistant", "text", "b", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m3", "c2", "assistant", "text", "c", 0}},
				{SQL: "INSERT INTO model_calls (message_id, provider, model, model_kind, prompt_tokens, completion_tokens, total_tokens, cost, cache_hit) VALUES (?,?,?,?,?,?,?,?,?)",
					Params: []interface{}{"m1", "openai", "gpt-4o", "chat", 10, 20, 30, 0.05, 1}},
				{SQL: "INSERT INTO model_calls (message_id, provider, model, model_kind, prompt_tokens, completion_tokens, total_tokens, cost, cache_hit) VALUES (?,?,?,?,?,?,?,?,?)",
					Params: []interface{}{"m2", "openai", "gpt-4o", "chat", 5, 10, 15, 0.03, 0}},
				{SQL: "INSERT INTO model_calls (message_id, provider, model, model_kind, prompt_tokens, completion_tokens, total_tokens, cost, cache_hit) VALUES (?,?,?,?,?,?,?,?,?)",
					Params: []interface{}{"m3", "openai", "gpt-4o", "chat", 100, 0, 100, 0.2, 0}},
			},
			expected: []struct {
				provider, model string
				total           int
			}{
				{provider: "openai", model: "gpt-4o", total: 45},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-usage")
			defer cleanup()
			dbtest.LoadDDLFromFile(t, db, ddlPath)
			dbtest.ExecAll(t, db, tc.seed)

			d := newDatly(t, dbPath)
			out := &ur.Output{}
			uri := ur.PathByConversation
			uri = strings.ReplaceAll(uri, "{conversationId}", tc.conversationID)
			_, err := d.Operate(context.Background(), datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&ur.Input{ConversationID: tc.conversationID, Has: &ur.Has{ConversationID: true}}))
			if !assert.Nil(t, err) {
				return
			}

			// build map provider+model -> total_tokens
			got := map[string]int{}
			for _, r := range out.Data {
				if r.TotalTokens != nil {
					got[r.Provider+"|"+r.Model] = *r.TotalTokens
				}
			}
			for _, exp := range tc.expected {
				key := exp.provider + "|" + exp.model
				assert.EqualValues(t, exp.total, got[key])
			}
		})
	}
}
