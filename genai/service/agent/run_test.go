package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/executor/config"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/usage"
	daofactory "github.com/viant/agently/internal/dao/factory"
	storeadapter "github.com/viant/agently/internal/domain/adapter"
	recpkg "github.com/viant/agently/internal/domain/recorder"
	"github.com/viant/agently/internal/testutil/dbtest"
	"github.com/viant/datly"
	"github.com/viant/datly/view"
	_ "modernc.org/sqlite"
)

// fakeModel is a minimal LLM model used for unit tests.
type fakeModel struct {
	content     string
	canStream   bool
	canUseTools bool
}

func (m *fakeModel) Generate(ctx context.Context, _ *llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return &llm.GenerateResponse{Choices: []llm.Choice{{
		Message:      llm.Message{Role: llm.RoleAssistant, Content: m.content},
		FinishReason: "stop",
	}}}, nil
}

func (m *fakeModel) Implements(feature string) bool {
	switch feature {
	case "can-stream":
		return m.canStream
	case "can-use-tools":
		return m.canUseTools
	}
	return false
}

// fakeFinder returns the provided model regardless of id.
type fakeFinder struct{ model llm.Model }

func (f *fakeFinder) Find(ctx context.Context, id string) (llm.Model, error) { return f.model, nil }

// TestService_Query_DataDriven covers Query with SQLite-backed store using table-driven cases.
func TestService_Query_DataDriven(t *testing.T) {
	type testCase struct {
		name           string
		input          *QueryInput
		modelContent   string
		expectedOutput *QueryOutput
		// when non-empty, validate conversation metadata["context"] against expected map
		expectedCtx map[string]interface{}
	}

	// Locate repository root to load shared DDL.
	_, filename, _, _ := runtime.Caller(0)
	ddlPath := filepath.Join(filepath.Dir(filename), "../../../internal/script/schema.ddl")

	cases := []testCase{
		{
			name: "simple content, no override",
			input: &QueryInput{
				Agent: &agentmdl.Agent{
					Identity:       agentmdl.Identity{Name: "demo"},
					ModelSelection: llm.ModelSelection{Model: "m1"},
					Prompt:         &prompt.Prompt{Text: "User: {{.Task.Prompt}}"},
					SystemPrompt:   &prompt.Prompt{Text: "Act as helpful assistant"},
				},
				Query: "hello",
			},
			modelContent: "hi",
			expectedOutput: &QueryOutput{
				// Plan is non-nil but empty when no tools/elicitation are present
				Plan:    plan.New(),
				Content: "hi",
				Model:   "m1",
				Usage:   &usage.Aggregator{},
			},
		},
		{
			name: "model override and context merge",
			input: &QueryInput{
				Agent: &agentmdl.Agent{
					Identity:       agentmdl.Identity{Name: "demo2"},
					ModelSelection: llm.ModelSelection{Model: "base"},
					Prompt:         &prompt.Prompt{Text: "Q: {{.Task.Prompt}}"},
					SystemPrompt:   &prompt.Prompt{Text: "System policy"},
				},
				ModelOverride: "override-model",
				Query:         `{"foo":"bar","x":1}`,
				Context:       map[string]interface{}{"x": 2},
			},
			modelContent: "ok",
			expectedOutput: &QueryOutput{
				Plan:    plan.New(),
				Content: "ok",
				Model:   "override-model",
				Usage:   &usage.Aggregator{},
			},
			expectedCtx: map[string]interface{}{"foo": "bar", "x": float64(2)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: temp SQLite with schema
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-agent-query")
			defer cleanup()
			dbtest.LoadDDLFromFile(t, db, ddlPath)

			// Datly + DAO-backed store
			dao, err := datly.New(context.Background())
			if err != nil {
				t.Fatalf("datly new: %v", err)
			}
			if err := dao.AddConnectors(context.Background(), view.NewConnector("agently", "sqlite", dbPath)); err != nil {
				t.Fatalf("add connector: %v", err)
			}
			apis, err := daofactory.New(context.Background(), daofactory.DAOSQL, dao)
			if err != nil {
				t.Fatalf("dao factory: %v", err)
			}
			store := storeadapter.New(apis.Conversation, apis.Message, apis.Turn, apis.ModelCall, apis.ToolCall, apis.Payload, apis.Usage)

			// Use a no-op recorder to isolate Query and avoid DAO coupling in this unit test
			var rec recpkg.Recorder = &noopRecorder{}

			// LLM core with fake finder/model
			finder := &fakeFinder{model: &fakeModel{content: tc.modelContent}}
			coreSvc := core.New(finder, nil, nil)

			// Ensure embedding model is set to bypass augmentation path when no knowledge is defined
			if tc.input != nil && tc.input.EmbeddingModel == "" {
				tc.input.EmbeddingModel = "dummy"
			}

			// Service under test (defaults must be non-nil)
			defaults := &config.Defaults{Embedder: "dummy"}
			svc := New(coreSvc, nil, nil, nil, nil, rec, store, defaults)

			// Act
			out := &QueryOutput{}
			err = svc.Query(context.Background(), tc.input, out)
			if !assert.NoError(t, err) {
				return
			}

			// Prepare expected by echoing agent pointer and aligning dynamic fields for comparison
			tc.expectedOutput.Agent = tc.input.Agent
			if tc.expectedOutput.Plan != nil && out.Plan != nil {
				tc.expectedOutput.Plan.ID = out.Plan.ID
			}
			// Align aggregator instance (empty state) to avoid pointer inequality noise
			tc.expectedOutput.Usage = out.Usage

			// Align dynamic conversation ID for deterministic comparison
			tc.expectedOutput.ConversationID = out.ConversationID

			// Assert output equality (single value assertion per case)
			assert.EqualValues(t, tc.expectedOutput, out)

			// Optionally assert conversation context persisted when provided
			if tc.expectedCtx != nil {
				// Fetch conversation and compare metadata.context
				cv, gErr := store.Conversations().Get(context.Background(), tc.input.ConversationID)
				if assert.NoError(t, gErr) && cv != nil && cv.Metadata != nil {
					var meta map[string]interface{}
					_ = json.Unmarshal([]byte(*cv.Metadata), &meta)
					gotCtx, _ := meta["context"].(map[string]interface{})
					assert.EqualValues(t, tc.expectedCtx, gotCtx)
				}
			}
		})
	}
}

// noopRecorder implements recorder.Recorder with no-ops for unit testing.
type noopRecorder struct{}

// noopRecorder retained for compatibility; no methods.
func (n *noopRecorder) RecordUsageTotals(ctx context.Context, conversationID string, input, output, embed int) {
}
