package agent

import (
	"context"

	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/usage"
	"github.com/viant/fluxor/model/types"
)

// QueryInput represents the input for querying an agent's knowledge
type QueryInput struct {
	// ConversationID is an optional identifier for the conversation session.
	// If provided, conversation history will be tracked and reused.
	ConversationID string `json:"conversationId,omitempty"`
	// Optional client-supplied identifier for the user message. When empty the
	// service will generate a UUID.
	MessageID       string          `json:"messageId,omitempty"`
	AgentName       string          `json:"agentName"`       // Path to the agent configuration
	Agent           *agentmdl.Agent `json:"agent"`           // Agent to use (alternative to AgentName)
	Query           string          `json:"query"`           // The query to submit
	MaxResponseSize int             `json:"maxResponseSize"` // Maximum size of the response in bytes
	MaxDocuments    int             `json:"maxDocuments"`    // Maximum number of documents to retrieve
	IncludeFile     bool            `json:"includeFile"`     // Whether to include complete file content
	EmbeddingModel  string          `json:"embeddingModel"`  // Find to use for embeddings

	// Optional runtime overrides (single-turn)
	ModelOverride string                 `json:"model,omitempty"` // llm model name
	ToolsAllowed  []string               `json:"tools,omitempty"` // allow-list for tools (empty = default)
	Context       map[string]interface{} `json:"context,omitempty"`

	// ElicitationMode controls how missing-input requests are handled.
	//   "user"   – always forward to end-user (current default)
	//   "agent"  – auto-fill using sub-agent; fail when unable
	//   "hybrid" – try agent first; fall back to real user when needed.
	ElicitationMode string `json:"elicitationMode,omitempty" yaml:"elicitationMode,omitempty"`
}

// QueryOutput represents the result of an agent knowledge query
type QueryOutput struct {
	ConversationID string            `json:"conversationId,omitempty"`
	Agent          *agentmdl.Agent   `json:"agent"`                 // Agent used for the query
	Content        string            `json:"content"`               // Generated content from the agent
	Elicitation    *plan.Elicitation `json:"elicitation,omitempty"` // structured missing input request
	Plan           *plan.Plan        `json:"plan,omitempty"`        // current execution plan (optional)
	Usage          *usage.Aggregator `json:"usage,omitempty"`
	Model          string            `json:"model,omitempty"`
}

func (s *Service) query(ctx context.Context, input interface{}, output interface{}) error {
	// 0. Coerce IO
	queryInput, ok := input.(*QueryInput)
	if !ok {
		return types.NewInvalidInputError(input)
	}
	queryOutput, ok := output.(*QueryOutput)
	if !ok {
		return types.NewInvalidOutputError(output)
	}
	return s.Query(ctx, queryInput, queryOutput)
}
