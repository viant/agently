package agent

import (
	"context"
	"strings"

	"github.com/viant/agently/client/conversation"
	agentmdl "github.com/viant/agently/genai/agent"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/prompt"
	svc "github.com/viant/agently/genai/tool/service"
	"github.com/viant/agently/genai/usage"
)

// QueryInput represents the input for querying an agent's knowledge
type QueryInput struct {
	// ConversationID is an optional identifier for the conversation session.
	// If provided, conversation history will be tracked and reused.
	ConversationID       string `json:"conversationId,omitempty"`
	ParentConversationID string `json:"parentConversationId,omitempty"`
	// Optional client-supplied identifier for the user message. When empty the
	// service will generate a UUID.
	MessageID   string               `json:"messageId,omitempty"`
	AgentID     string               `json:"agentId"` // Agent ID to use
	UserId      string               `json:"userId"`
	Agent       *agentmdl.Agent      `json:"agent"` // Agent to use (alternative to agentId)
	Query       string               `json:"query"` // The query to submit
	Attachments []*prompt.Attachment `json:"attachments,omitempty"`

	MaxResponseSize int    `json:"maxResponseSize"` // Maximum size of the response in bytes
	MaxDocuments    int    `json:"maxDocuments"`    // Maximum number of documents to retrieve
	IncludeFile     bool   `json:"includeFile"`     // Whether to include complete file content
	EmbeddingModel  string `json:"embeddingModel"`  // Find to use for embeddings

	// Optional runtime overrides (single-turn)
	ModelOverride string                 `json:"model,omitempty"` // llm model name
	ToolsAllowed  []string               `json:"tools,omitempty"` // allow-list for tools (empty = default)
	Context       map[string]interface{} `json:"context,omitempty"`

	Transcript conversation.Transcript `json:"transcript,omitempty"`

	// ElicitationMode controls how missing-input requests are handled.
	ElicitationMode string `json:"elicitationMode,omitempty" yaml:"elicitationMode,omitempty"`

	AutoSummarize *bool `json:"autoSummarize,omitempty"`

	AllowedChains []string `json:"allowedChains,omitempty"` //

	DisableChains bool `json:"disableChains,omitempty"`

	ToolCallExposure *agentmdl.ToolCallExposure `json:"toolCallExposure,omitempty"`

	// ToolResultPreviewLimit (single-turn override). When >0 it takes precedence
	// over agent, model and service defaults for tool result preview trimming.
	ToolResultPreviewLimit *int `json:"toolResultPreviewLimit,omitempty"`

	// ReasoningEffort optionally overrides agent-level Reasoning.Effort for this turn.
	// Valid values (OpenAI o-series): low | medium | high.
	ReasoningEffort *string `json:"reasoningEffort,omitempty"`
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
	MessageID      string            `json:"messageId,omitempty"`
	Warnings       []string          `json:"warnings,omitempty"`
}

func (s *Service) query(ctx context.Context, input interface{}, output interface{}) error {
	// 0. Coerce IO
	queryInput, ok := input.(*QueryInput)
	if !ok {
		return svc.NewInvalidInputError(input)
	}
	queryOutput, ok := output.(*QueryOutput)
	if !ok {
		return svc.NewInvalidOutputError(output)
	}
	return s.Query(ctx, queryInput, queryOutput)
}

func (i *QueryInput) Actor() string {
	actor := ""
	if i != nil && i.Agent != nil && strings.TrimSpace(i.Agent.ID) != "" {
		actor = strings.TrimSpace(i.Agent.ID)
	} else if i != nil && strings.TrimSpace(i.AgentID) != "" {
		actor = strings.TrimSpace(i.AgentID)
	}
	return actor
}

func (i *QueryInput) ShallAutoSummarize() bool {
	if i.Agent.HasAutoSummarizeDefinition() {
		if !i.Agent.ShallAutoSummarize() {
			return false
		}
	}
	if i.AutoSummarize == nil {
		return i.Agent.ShallAutoSummarize()
	}
	autoSummarize := false
	if i.AutoSummarize != nil {
		autoSummarize = *i.AutoSummarize
	}
	return autoSummarize
}
