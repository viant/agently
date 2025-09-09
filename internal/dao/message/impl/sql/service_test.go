//go:build cgo

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
		name                   string
		seed                   []dbtest.ParameterizedSQL
		opts                   []InputOption
		expectedMessageIDs     []string
		expectedToolCallCount  int
		expectedToolCallNames  []string
		expectedModelCallCount int
		expectedModelProviders []string
	}

	cases := []testCase{
		{
			name: "with model call join",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cvM", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"mA", "cvM", "assistant", "text", "answer", 0}},
				{SQL: "INSERT INTO model_calls (message_id, provider, model, model_kind) VALUES (?,?,?,?)", Params: []interface{}{"mA", "openai", "gpt-4o-mini", "chat"}},
			},
			opts:                   []InputOption{WithConversationID("cvM")},
			expectedMessageIDs:     []string{"mA"},
			expectedModelCallCount: 1,
			expectedModelProviders: []string{"openai"},
		},
		{
			name: "with tool call join",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv1", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m3", "cv1", "tool", "tool_op", "call", 0}},
				{SQL: "INSERT INTO tool_calls (message_id, turn_id, op_id, attempt, tool_name, tool_kind, status) VALUES (?,?,?,?,?,?,?)", Params: []interface{}{"m3", nil, "op-1", 1, "search", "general", "completed"}},
			},
			opts:                  []InputOption{WithConversationID("cv1"), WithType("tool_op")},
			expectedMessageIDs:    []string{"m3"},
			expectedToolCallCount: 1,
			expectedToolCallNames: []string{"search"},
		},
		{
			name: "by conversation",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv1", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m1", "cv1", "user", "text", "hello", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m2", "cv1", "assistant", "text", "world", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m3", "cv1", "tool", "tool_op", "call", 1}},
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv2", "Beta", "AgentB"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m4", "cv2", "user", "text", "ignore", 0}},
			},
			opts:               []InputOption{WithConversationID("cv1")},
			expectedMessageIDs: []string{"m1", "m2", "m3"},
		},
		{
			name: "by conversation + role",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv1", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m1", "cv1", "user", "text", "hello", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m2", "cv1", "assistant", "text", "world", 0}},
			},
			opts:               []InputOption{WithConversationID("cv1"), WithRole("assistant")},
			expectedMessageIDs: []string{"m2"},
		},
		{
			name: "by type",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv1", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m3", "cv1", "tool", "tool_op", "call", 1}},
			},
			opts:               []InputOption{WithConversationID("cv1"), WithType("tool_op")},
			expectedMessageIDs: []string{"m3"},
		},

		{
			name: "by interim",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv1", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m1", "cv1", "user", "text", "hello", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m3", "cv1", "tool", "tool_op", "call", 1}},
			},
			opts:               []InputOption{WithConversationID("cv1"), WithInterim(1)},
			expectedMessageIDs: []string{"m3"},
		},
		{
			name: "by ids",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv1", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m1", "cv1", "user", "text", "hello", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m2", "cv1", "assistant", "text", "world", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, role, type, content, interim) VALUES (?,?,?,?,?,?)", Params: []interface{}{"m3", "cv1", "tool", "tool_op", "call", 1}},
			},
			opts:               []InputOption{WithConversationID("cv1"), WithIDs("m1", "m3")},
			expectedMessageIDs: []string{"m1", "m3"},
		},
		{
			name: "since timestamp",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv1", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO message (id, conversation_id, created_at, role, type, content, interim) VALUES (?,?,?,?,?,?,?)", Params: []interface{}{"m1", "cv1", "2024-01-01 10:00:00", "user", "text", "old", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, created_at, role, type, content, interim) VALUES (?,?,?,?,?,?,?)", Params: []interface{}{"m2", "cv1", "2024-01-01 12:00:00", "assistant", "text", "new", 0}},
			},
			// since 2024-01-01 11:00:00 expect only m2
			opts:               []InputOption{WithConversationID("cv1"), WithSince(mustParse("2006-01-02 15:04:05", "2024-01-01 11:00:00"))},
			expectedMessageIDs: []string{"m2"},
		},
	}

	// resolve DDL
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))))
	ddlPath := filepath.Join(repoRoot, "script", "schema.ddl")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-message")
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
			toolCount := 0
			var toolNames []string
			modelCount := 0
			var modelProviders []string
			for _, r := range rows {
				ids = append(ids, r.Id)
				if r.ToolCall != nil {
					toolCount++
					toolNames = append(toolNames, r.ToolCall.ToolName)
				}
				if r.ModelCall != nil {
					modelCount++
					modelProviders = append(modelProviders, r.ModelCall.Provider)
				}
			}
			assert.EqualValues(t, tc.expectedMessageIDs, ids)
			assert.EqualValues(t, tc.expectedToolCallCount, toolCount)
			if tc.expectedToolCallNames != nil {
				assert.ElementsMatch(t, tc.expectedToolCallNames, toolNames)
			}
			assert.EqualValues(t, tc.expectedModelCallCount, modelCount)
			if tc.expectedModelProviders != nil {
				assert.ElementsMatch(t, tc.expectedModelProviders, modelProviders)
			}
		})
	}
}

func mustParse(layout, value string) time.Time {
	t, _ := time.Parse(layout, value)
	return t
}

func TestService_GetTranscript(t *testing.T) {
	type testCase struct {
		name               string
		seed               []dbtest.ParameterizedSQL
		conversationID     string
		turnID             string
		opts               []InputOption
		expectedMessageIDs []string
	}

	// resolve DDL
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))))
	ddlPath := filepath.Join(repoRoot, "script", "schema.ddl")

	cases := []testCase{
		{
			name:           "dedup by op_id + request_hash; exclude control and interim; keep latest attempt",
			conversationID: "cv1",
			turnID:         "t1",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv1", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO turns (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t1", "cv1", "running"}},
				// user (seq 1)
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim, created_at) VALUES (?,?,?,?,?,?,?,?,?)", Params: []interface{}{"m1", "cv1", "t1", 1, "user", "text", "q", 0, "2024-01-01 10:00:00"}},
				// tool opA/rh1 attempt 1 (seq 2) – should be dropped due to newer attempt
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim, created_at) VALUES (?,?,?,?,?,?,?,?,?)", Params: []interface{}{"m2", "cv1", "t1", 2, "tool", "tool_op", "call a1", 0, "2024-01-01 10:01:00"}},
				{SQL: "INSERT INTO tool_calls (message_id, turn_id, op_id, attempt, tool_name, tool_kind, status, request_hash) VALUES (?,?,?,?,?,?,?,?)", Params: []interface{}{"m2", "t1", "opA", 1, "fetch", "general", "completed", "rh1"}},
				// tool opA/rh1 attempt 2 (seq 3) – should be kept
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim, created_at) VALUES (?,?,?,?,?,?,?,?,?)", Params: []interface{}{"m3", "cv1", "t1", 3, "tool", "tool_op", "call a2", 0, "2024-01-01 10:02:00"}},
				{SQL: "INSERT INTO tool_calls (message_id, turn_id, op_id, attempt, tool_name, tool_kind, status, request_hash) VALUES (?,?,?,?,?,?,?,?)", Params: []interface{}{"m3", "t1", "opA", 2, "fetch", "general", "completed", "rh1"}},
				// tool opA/rh2 attempt 3 (seq 4) – different args, should be kept
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim, created_at) VALUES (?,?,?,?,?,?,?,?,?)", Params: []interface{}{"m4", "cv1", "t1", 4, "tool", "tool_op", "call a3", 0, "2024-01-01 10:03:00"}},
				{SQL: "INSERT INTO tool_calls (message_id, turn_id, op_id, attempt, tool_name, tool_kind, status, request_hash) VALUES (?,?,?,?,?,?,?,?)", Params: []interface{}{"m4", "t1", "opA", 3, "fetch", "general", "completed", "rh2"}},
				// control type (seq 5) – should be excluded
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim, created_at) VALUES (?,?,?,?,?,?,?,?,?)", Params: []interface{}{"m5", "cv1", "t1", 5, "control", "control", "ctrl", 0, "2024-01-01 10:04:00"}},
				// interim tool (seq 6) – should be excluded by default
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim, created_at) VALUES (?,?,?,?,?,?,?,?,?)", Params: []interface{}{"m6", "cv1", "t1", 6, "tool", "tool_op", "call interim", 1, "2024-01-01 10:05:00"}},
				{SQL: "INSERT INTO tool_calls (message_id, turn_id, op_id, attempt, tool_name, tool_kind, status, request_hash) VALUES (?,?,?,?,?,?,?,?)", Params: []interface{}{"m6", "t1", "opB", 1, "fetch", "general", "running", "rhX"}},
				// assistant (seq 7)
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim, created_at) VALUES (?,?,?,?,?,?,?,?,?)", Params: []interface{}{"m7", "cv1", "t1", 7, "assistant", "text", "ans", 0, "2024-01-01 10:06:00"}},
			},
			expectedMessageIDs: []string{"m1", "m3", "m4", "m7"},
		},
		{
			name:           "dedup fallback by op_id when request_hash missing",
			conversationID: "cv2",
			turnID:         "t2",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv2", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO turns (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t2", "cv2", "running"}},
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim) VALUES (?,?,?,?,?,?,?,?)", Params: []interface{}{"u1", "cv2", "t2", 1, "user", "text", "q", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim) VALUES (?,?,?,?,?,?,?,?)", Params: []interface{}{"ta1", "cv2", "t2", 2, "tool", "tool_op", "a1", 0}},
				{SQL: "INSERT INTO tool_calls (message_id, turn_id, op_id, attempt, tool_name, tool_kind, status) VALUES (?,?,?,?,?,?,?)", Params: []interface{}{"ta1", "t2", "opX", 1, "calc", "general", "completed"}},
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim) VALUES (?,?,?,?,?,?,?,?)", Params: []interface{}{"ta2", "cv2", "t2", 3, "tool", "tool_op", "a2", 0}},
				{SQL: "INSERT INTO tool_calls (message_id, turn_id, op_id, attempt, tool_name, tool_kind, status) VALUES (?,?,?,?,?,?,?)", Params: []interface{}{"ta2", "t2", "opX", 2, "calc", "general", "completed"}},
				{SQL: "INSERT INTO message (id, conversation_id, turn_id, sequence, role, type, content, interim) VALUES (?,?,?,?,?,?,?,?)", Params: []interface{}{"as1", "cv2", "t2", 4, "assistant", "text", "ans", 0}},
			},
			expectedMessageIDs: []string{"u1", "ta2", "as1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-message-transcript")
			defer cleanup()
			dbtest.LoadDDLFromFile(t, db, ddlPath)
			dbtest.ExecAll(t, db, tc.seed)

			d := newDatly(t, dbPath)
			svc := New(context.Background(), d)

			// turn id provided via options when needed
			rows, err := svc.GetTranscript(context.Background(), tc.conversationID, tc.opts...)
			if !assert.Nil(t, err) {
				return
			}
			var ids []string
			for _, r := range rows {
				ids = append(ids, r.Id)
			}
			assert.EqualValues(t, tc.expectedMessageIDs, ids)
		})
	}
}

func TestService_GetConversation(t *testing.T) {
	type testCase struct {
		name               string
		seed               []dbtest.ParameterizedSQL
		conversationID     string
		opts               []InputOption
		expectedMessageIDs []string
	}

	// resolve DDL
	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))))
	ddlPath := filepath.Join(repoRoot, "script", "schema.ddl")

	cases := []testCase{
		{
			name:           "default: only user/assistant (no tool)",
			conversationID: "cv3",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO conversation (id, summary, agent_name) VALUES (?,?,?)", Params: []interface{}{"cv3", "Alpha", "AgentA"}},
				{SQL: "INSERT INTO turns (id, conversation_id, status) VALUES (?,?,?)", Params: []interface{}{"t3", "cv3", "running"}},
				{SQL: "INSERT INTO message (id, conversation_id, created_at, role, type, content, interim) VALUES (?,?,?,?,?,?,?)", Params: []interface{}{"u1", "cv3", "2024-01-01 10:00:00", "user", "text", "hi", 0}},
				{SQL: "INSERT INTO message (id, conversation_id, created_at, role, type, content, interim) VALUES (?,?,?,?,?,?,?)", Params: []interface{}{"tool1", "cv3", "2024-01-01 10:01:00", "tool", "tool_op", "t", 0}},
				{SQL: "INSERT INTO tool_calls (message_id, turn_id, op_id, attempt, tool_name, tool_kind, status, request_hash) VALUES (?,?,?,?,?,?,?,?)", Params: []interface{}{"tool1", "t3", "opZ", 1, "web", "general", "completed", "rh9"}},
				{SQL: "INSERT INTO message (id, conversation_id, created_at, role, type, content, interim) VALUES (?,?,?,?,?,?,?)", Params: []interface{}{"a1", "cv3", "2024-01-01 10:02:00", "assistant", "text", "ok", 0}},
			},
			expectedMessageIDs: []string{"u1", "a1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-message-conv")
			defer cleanup()
			dbtest.LoadDDLFromFile(t, db, ddlPath)
			dbtest.ExecAll(t, db, tc.seed)

			d := newDatly(t, dbPath)
			svc := New(context.Background(), d)

			rows, err := svc.GetConversation(context.Background(), tc.conversationID, tc.opts...)
			if !assert.Nil(t, err) {
				return
			}
			var ids []string
			for _, r := range rows {
				ids = append(ids, r.Id)
			}
			assert.EqualValues(t, tc.expectedMessageIDs, ids)
		})
	}
}
