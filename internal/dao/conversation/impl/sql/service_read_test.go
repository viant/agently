package sql

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/internal/dao/conversation/read"
	"github.com/viant/agently/internal/testutil/dbtest"
	_ "modernc.org/sqlite"
)

func TestService_GetConversation(t *testing.T) {
	// data-driven scenarios: by id, by summary, by other predicates
	type testCase struct {
		name     string
		seed     []dbtest.ParameterizedSQL
		exec     func(srv *Service) (interface{}, error)
		expected interface{}
	}

	convID := "550e8400-e29b-41d4-a716-446655440000"
	// resolve schema path once
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))))
	ddlPath := filepath.Join(repoRoot, "script", "schema.ddl")

	cases := []testCase{
		{
			name: "by id",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{convID, "Alpha", "AgentA"}},
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"c2", "Beta", "AgentB"}},
			},
			exec: func(srv *Service) (interface{}, error) {
				rows, err := srv.GetConversations(context.Background(), read.WithID(convID))
				if err != nil {
					return nil, err
				}
				if len(rows) == 0 {
					return "", nil
				}
				return rows[0].Id, nil
			},
			expected: convID,
		},
		{
			name: "by summary contains",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"c3", "Project Alpha", "AgentA"}},
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"c4", "Alpha Phase 2", "AgentB"}},
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"c5", "Gamma", "AgentC"}},
			},
			exec: func(srv *Service) (interface{}, error) {
				rows, err := srv.GetConversations(context.Background(), read.WithSummaryContains("Alpha"))
				if err != nil {
					return nil, err
				}
				var ids []string
				for _, r := range rows {
					ids = append(ids, r.Id)
				}
				return ids, nil
			},
			expected: []string{"c3", "c4"},
		},
		{
			name: "by title contains",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, title, agent_name) VALUES (?,?,?,?)", Params: []interface{}{"p1", "x", "Alpha launch", "AgentA"}},
				{SQL: "INSERT INTO conversation (id, summary, title, agent_name) VALUES (?,?,?,?)", Params: []interface{}{"p2", "x", "Roadmap Alpha", "AgentB"}},
				{SQL: "INSERT INTO conversation (id, summary, title, agent_name) VALUES (?,?,?,?)", Params: []interface{}{"p3", "x", "Misc", "AgentC"}},
			},
			exec: func(srv *Service) (interface{}, error) {
				rows, err := srv.GetConversations(context.Background(), read.WithTitleContains("Alpha"))
				if err != nil {
					return nil, err
				}
				var ids []string
				for _, r := range rows {
					ids = append(ids, r.Id)
				}
				return ids, nil
			},
			expected: []string{"p1", "p2"},
		},
		{
			name: "by agent_name contains",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"a1", "x", "AgentA"}},
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"a2", "x", "AgentB"}},
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"a3", "x", "AgentA"}},
			},
			exec: func(srv *Service) (interface{}, error) {
				rows, err := srv.GetConversations(context.Background(), read.WithAgentNameContains("AgentA"))
				if err != nil {
					return nil, err
				}
				var ids []string
				for _, r := range rows {
					ids = append(ids, r.Id)
				}
				return ids, nil
			},
			expected: []string{"a1", "a3"},
		},
		{
			name: "by agent_id equals",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_id) VALUES (?,?,?)", Params: []interface{}{"b1", "x", "A1"}},
				{SQL: "INSERT INTO conversation (id, summary, agent_id) VALUES (?,?,?)", Params: []interface{}{"b2", "x", "A2"}},
				{SQL: "INSERT INTO conversation (id, summary, agent_id) VALUES (?,?,?)", Params: []interface{}{"b3", "x", "A1"}},
			},
			exec: func(srv *Service) (interface{}, error) {
				rows, err := srv.GetConversations(context.Background(), read.WithAgentID("A1"))
				if err != nil {
					return nil, err
				}
				var ids []string
				for _, r := range rows {
					ids = append(ids, r.Id)
				}
				return ids, nil
			},
			expected: []string{"b1", "b3"},
		},
		{
			name: "by visibility equals",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, visibility) VALUES (?,?,?)", Params: []interface{}{"v1", "x", "private"}},
				{SQL: "INSERT INTO conversation (id, summary, visibility) VALUES (?,?,?)", Params: []interface{}{"v2", "x", "shared"}},
				{SQL: "INSERT INTO conversation (id, summary, visibility) VALUES (?,?,?)", Params: []interface{}{"v3", "x", "org"}},
			},
			exec: func(srv *Service) (interface{}, error) {
				rows, err := srv.GetConversations(context.Background(), read.WithVisibility("shared"))
				if err != nil {
					return nil, err
				}
				var ids []string
				for _, r := range rows {
					ids = append(ids, r.Id)
				}
				return ids, nil
			},
			expected: []string{"v2"},
		},
		{
			name: "by archived IN (1)",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, archived) VALUES (?,?,?)", Params: []interface{}{"r1", "x", 1}},
				{SQL: "INSERT INTO conversation (id, summary, archived) VALUES (?,?,?)", Params: []interface{}{"r2", "x", 0}},
				{SQL: "INSERT INTO conversation (id, summary, archived) VALUES (?,?,?)", Params: []interface{}{"r3", "x", 1}},
			},
			exec: func(srv *Service) (interface{}, error) {
				rows, err := srv.GetConversations(context.Background(), read.WithArchived(1))
				if err != nil {
					return nil, err
				}
				var ids []string
				for _, r := range rows {
					ids = append(ids, r.Id)
				}
				return ids, nil
			},
			expected: []string{"r1", "r3"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-conv")
			defer cleanup()

			dbtest.LoadDDLFromFile(t, db, ddlPath)
			dbtest.ExecAll(t, db, tc.seed)

			srv := newV2Service(t, dbPath)
			actual, err := tc.exec(srv)
			if !assert.Nil(t, err) {
				return
			}
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
