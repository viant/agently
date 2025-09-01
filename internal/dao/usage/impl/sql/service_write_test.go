//go:build cgo

package sql

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	w "github.com/viant/agently/internal/dao/usage/write"
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

type uexp struct {
	Id           string
	In, Out, Emb int
}

func TestUsage_Patch(t *testing.T) {
	type testCase struct {
		name     string
		seed     []dbtest.ParameterizedSQL
		patch    []*w.Usage
		verifyID string
		expected uexp
	}

	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename)))))))
	ddlPath := filepath.Join(repoRoot, "internal", "script", "schema.ddl")

	cases := []testCase{
		{
			name: "insert new conversation usage",
			seed: []dbtest.ParameterizedSQL{},
			patch: []*w.Usage{func() *w.Usage {
				u := &w.Usage{Has: &w.UsageHas{}}
				u.SetConversationID("c1")
				u.SetUsageInputTokens(10)
				u.SetUsageOutputTokens(2)
				u.SetUsageEmbeddingTokens(1)
				return u
			}()},
			verifyID: "c1",
			expected: uexp{Id: "c1", In: 10, Out: 2, Emb: 1},
		},
		{
			name: "update existing usage",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, usage_input_tokens, usage_output_tokens, usage_embedding_tokens) VALUES (?,?,?,?,?)", Params: []interface{}{"c2", "X", 1, 2, 3}},
			},
			patch: []*w.Usage{func() *w.Usage {
				u := &w.Usage{Has: &w.UsageHas{}}
				u.SetConversationID("c2")
				u.SetUsageInputTokens(20)
				u.SetUsageOutputTokens(5)
				u.SetUsageEmbeddingTokens(0)
				return u
			}()},
			verifyID: "c2",
			expected: uexp{Id: "c2", In: 20, Out: 5, Emb: 0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-usage-post")
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

			// verify
			rows, err := db.Query("SELECT id, usage_input_tokens, usage_output_tokens, usage_embedding_tokens FROM conversation WHERE id = ?", tc.verifyID)
			if !assert.Nil(t, err) {
				return
			}
			defer rows.Close()
			if !rows.Next() {
				t.Fatalf("no row for %s", tc.verifyID)
			}
			var id string
			var in, outT, emb int
			assert.Nil(t, rows.Scan(&id, &in, &outT, &emb))
			assert.EqualValues(t, tc.expected, uexp{Id: id, In: in, Out: outT, Emb: emb})
		})
	}
}
