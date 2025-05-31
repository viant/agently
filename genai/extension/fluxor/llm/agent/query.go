package agent

import (
    "github.com/tmc/langchaingo/schema"
    "github.com/viant/agently/genai/agent"
    "github.com/viant/agently/genai/agent/plan"
    "github.com/viant/agently/genai/usage"
)

// QueryInput represents the input for querying an agent's knowledge
type QueryInput struct {
	// ConversationID is an optional identifier for the conversation session.
	// If provided, conversation history will be tracked and reused.
	ConversationID  string       `json:"conversationId,omitempty"`
	Location        string       `json:"location"`        // Path to the agent configuration
	Agent           *agent.Agent `json:"agent"`           // Agent to use (alternative to Location)
	Query           string       `json:"query"`           // The query to submit
	MaxResponseSize int          `json:"maxResponseSize"` // Maximum size of the response in bytes
	MaxDocuments    int          `json:"maxDocuments"`    // Maximum number of documents to retrieve
	IncludeFile     bool         `json:"includeFile"`     // Whether to include complete file content
	EmbeddingModel  string       `json:"embeddingModel"`  // Find to use for embeddings
}

// QueryOutput represents the result of an agent knowledge query
type QueryOutput struct {
	Agent         *agent.Agent      `json:"agent"`                 // Agent used for the query
	Content       string            `json:"content"`               // Generated content from the agent
	Documents     []schema.Document `json:"documents"`             // List of relevant documents
	DocumentsSize int               `json:"documentsSize"`         // Total size of retrieved documents
	Elicitation   *plan.Elicitation `json:"elicitation,omitempty"` // structured missing input request
	Usage         *usage.Aggregator `json:"usage,omitempty"`
}
