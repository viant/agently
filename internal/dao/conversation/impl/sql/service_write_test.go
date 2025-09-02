package sql_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	conv "github.com/viant/agently/internal/dao/conversation/impl/sql"
	"github.com/viant/agently/internal/dao/conversation/read"
	w "github.com/viant/agently/internal/dao/conversation/write"
	"github.com/viant/agently/internal/testutil/dbtest"
	"github.com/viant/datly/view"
)

// helper to construct conversation service (read + write)
func newConversationService(t *testing.T, dbPath string) *conv.Service {
	t.Helper()
	connector := view.NewConnector("agently", "sqlite", dbPath)
	srv, err := conv.New(context.Background(), connector)
	if err != nil {
		t.Fatalf("conversation.New: %v", err)
	}
	return srv
}

type convExpected struct {
	Id                   string
	Summary              *string
	AgentName            *string
	UsageInputTokens     *int
	HasCreatedAt         bool
	HasLastActivity      bool
	DefaultModelProvider *string
	DefaultModel         *string
	DefaultModelParams   *string
	Metadata             *string
}

func toExpected(v *read.ConversationView) convExpected {
	var agentName *string = v.AgentName
	var usageIn *int = v.UsageInputTokens
	return convExpected{
		Id:                   v.Id,
		Summary:              v.Summary,
		AgentName:            agentName,
		UsageInputTokens:     usageIn,
		HasCreatedAt:         v.CreatedAt != nil,
		HasLastActivity:      v.LastActivity != nil,
		DefaultModelProvider: v.DefaultModelProvider,
		DefaultModel:         v.DefaultModel,
		DefaultModelParams:   v.DefaultModelParams,
		Metadata:             v.Metadata,
	}
}

func Test_PostConversation_InsertAndUpdate_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     []dbtest.ParameterizedSQL
		patch    []*w.Conversation
		verifyID string
		expected convExpected
	}

	// resolve DDL
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))))
	ddlPath := filepath.Join(repoRoot, "script", "schema.ddl")

	alpha := "Alpha"
	agentA := "AgentA"
	beta := "Beta"
	ten := 10
	zero := 0

	cases := []testCase{
		{
			name: "insert new conversation",
			seed: nil,
			patch: []*w.Conversation{func() *w.Conversation {
				c := &w.Conversation{Has: &w.ConversationHas{}}
				c.SetId("c1")
				c.SetSummary(alpha)
				c.SetAgentName(agentA)
				return c
			}()},
			verifyID: "c1",
			expected: convExpected{Id: "c1", Summary: &alpha, AgentName: &agentA, UsageInputTokens: &zero, HasCreatedAt: true, HasLastActivity: true},
		},
		{
			name: "set default model and metadata",
			seed: nil,
			patch: []*w.Conversation{func() *w.Conversation {
				c := &w.Conversation{Has: &w.ConversationHas{}}
				c.SetId("c5")
				c.SetAgentName(agentA)
				c.SetDefaultModelProvider("openai")
				c.SetDefaultModel("gpt-4o-mini")
				c.SetDefaultModelParams("{\"temperature\":0.3}")
				c.SetMetadata("{\"tools\":[\"search\",\"web\"]}")
				return c
			}()},
			verifyID: "c5",
			expected: func() convExpected {
				prov := "openai"
				model := "gpt-4o-mini"
				params := "{\"temperature\":0.3}"
				meta := "{\"tools\":[\"search\",\"web\"]}"
				return convExpected{Id: "c5", AgentName: &agentA, UsageInputTokens: &zero, HasCreatedAt: true, HasLastActivity: true,
					DefaultModelProvider: &prov, DefaultModel: &model, DefaultModelParams: &params, Metadata: &meta}
			}(),
		},
		{
			name: "update existing conversation (summary, usage tokens)",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name, created_at, last_activity, usage_input_tokens) VALUES (?,?,?,?,?,?)", Params: []interface{}{"c2", "Old", "AgentB", "2024-01-01 10:00:00", "2024-01-01 10:00:00", 3}},
			},
			patch: []*w.Conversation{func() *w.Conversation {
				c := &w.Conversation{Has: &w.ConversationHas{}}
				c.SetId("c2")
				c.SetSummary(beta)
				c.SetUsageInputTokens(ten)
				return c
			}()},
			verifyID: "c2",
			expected: func() convExpected {
				agentB := "AgentB"
				return convExpected{Id: "c2", Summary: &beta, AgentName: &agentB, UsageInputTokens: &ten, HasCreatedAt: true, HasLastActivity: true}
			}(),
		},
		{
			name: "mixed upsert (one existing, one new)",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"c3", "X", "AgentX"}},
			},
			patch: []*w.Conversation{
				func() *w.Conversation {
					c := &w.Conversation{Has: &w.ConversationHas{}}
					c.SetId("c3")
					c.SetSummary(beta)
					return c
				}(), // update existing
				func() *w.Conversation {
					c := &w.Conversation{Has: &w.ConversationHas{}}
					c.SetId("c4")
					c.SetSummary(alpha)
					c.SetAgentName(agentA)
					return c
				}(), // insert new
			},
			verifyID: "c4",
			expected: convExpected{Id: "c4", Summary: &alpha, AgentName: &agentA, UsageInputTokens: &zero, HasCreatedAt: true, HasLastActivity: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-conv-post")
			defer cleanup()
			dbtest.LoadDDLFromFile(t, db, ddlPath)
			dbtest.ExecAll(t, db, tc.seed)

			svc := newConversationService(t, dbPath)
			out, err := svc.PatchConversations(context.Background(), tc.patch...)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.NotNil(t, out) {
				return
			}

			// read back using same service
			rows, err := svc.GetConversations(context.Background(), conv.WithID(tc.verifyID))
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
