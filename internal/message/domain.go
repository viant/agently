package message

import (
	"time"
)

// Type represents a high-level message category in a conversation.
// Keep values lowercase for stable comparisons and storage.
type Type string

const (
	// TypeUser is a user-authored text message.
	TypeUser Type = "user"
	// TypeAssistant is an assistant-authored text message.
	TypeAssistant Type = "assistant"
	// TypeTool is a tool call or tool result message.
	TypeTool Type = "tool"
)

// Message is the canonical, internal representation of a conversation message
// used by internal/message tools. It is intentionally generic and applies to
// user, assistant, and tool messages. No persistence assumptions are made here.
type Message struct {
	ID              string    // unique message identifier
	ConversationID  string    // owning conversation
	ParentMessageID string    // optional parent (threading/turn)
	Type            Type      // user | assistant | tool
	Role            string    // optional role (e.g., "system", "assistant")
	ToolName        string    // if Type==tool, the tool's name
	CreatedAt       time.Time // creation timestamp

	// Content carries primary textual content. For binary/large payloads, use
	// an external payload reference in services that implement the store.
	Content string

	// Summary is a short retained summary when compacting/removing messages.
	// Kept concise (recommendation: <= 256 chars) and authored by model/user.
	Summary string

	// Removed flags a message as logically removed from the transcript while
	// retaining Summary. RemovedAt is when it was flagged.
	Removed   bool
	RemovedAt *time.Time

	// Tags are optional categorical labels (e.g., "duplicate", "large-output").
	Tags []string

	// Embedding holds the vector used for semantic search. Storage/backing is
	// left to the MessageStore implementation.
	Embedding []float32
}

// Preview contains a compact, human-readable excerpt used when presenting
// candidates (e.g., for removal) to the model or UI.
type Preview struct {
	Head string // first N lines/bytes
	Tail string // last N lines/bytes
}

// Candidate represents a message with sizing and preview metadata used when
// proposing removals/compaction to the LLM or UI.
type Candidate struct {
	Message   *Message
	ByteSize  int
	TokenSize int
	Preview   Preview
}

// Filter specifies common query criteria for listing messages.
type Filter struct {
	Types          []Type     // optional type filter
	OlderThan      *time.Time // optional upper bound on CreatedAt
	Limit          int        // optional limit (>0)
	IncludeRemoved bool       // include logically removed messages
}

// CandidateOptions control how candidate listings are produced.
type CandidateOptions struct {
	Filter
	// Future: strategies, scoring hints, etc.
}

// MessageStore abstracts persistence of messages and related metadata used by
// internal/message services. Implementations may back onto an in-memory store,
// SQL/NoSQL databases, or conversation clients already present in the system.
type MessageStore interface {
	// Get returns messages by ID within a conversation.
	Get(conversationID string, ids ...string) ([]*Message, error)

	// List returns messages matching the supplied filter in a conversation.
	List(conversationID string, filter Filter) ([]*Message, error)

	// Put inserts a new message or replaces an existing one.
	Put(msg *Message) error

	// UpdateSummary updates the short retained summary for a message.
	UpdateSummary(messageID string, summary string) error

	// MarkRemoved flags/unflags a message as removed and sets RemovedAt.
	MarkRemoved(messageID string, removed bool, removedAt *time.Time) error

	// UpdateEmbedding upserts the semantic embedding vector for a message.
	UpdateEmbedding(messageID string, embedding []float32) error

	// ListCandidates returns messages augmented with sizing and preview data
	// used when presenting removal/compaction options.
	ListCandidates(conversationID string, opts CandidateOptions) ([]Candidate, error)
}
