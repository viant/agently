package memory

import (
	"context"
	"github.com/viant/agently/genai/agent/plan"
	"time"
)

type messageID string

var MessageIDKey = messageID("messageID")

func MessageIDFromContext(ctx context.Context) string {
	value := ctx.Value(MessageIDKey)
	if value == nil {
		return ""
	}
	return value.(string)
}

// ConversationIDKey is used to propagate the current conversation identifier
// via context so that downstream services (e.g. tool-execution tracing) can
// associate side-effects with the correct conversation without changing every
// function signature.
type conversationID string

var ConversationIDKey = conversationID("conversationID")

func ConversationIDFromContext(ctx context.Context) string {
	value := ctx.Value(ConversationIDKey)
	if value == nil {
		return ""
	}
	return value.(string)
}

// Message represents a conversation message for memory storage.
type Message struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversationId"`
	ParentID       string  `json:"parentId,omitempty"`
	Role           string  `json:"role"`
	Content        string  `json:"content"`
	ToolName       *string `json:"toolName,omitempty"` // Optional tool name, can be nil
	// When messages include file uploads the Attachments slice describes each
	// uploaded asset (or generated/downloadable asset on assistant side).
	Attachments Attachments     `json:"attachments,omitempty" yaml:"attachments,omitempty" sqlx:"-"`
	Executions  []*plan.Outcome `json:"executions,omitempty" yaml:"executions,omitempty" sqlx:"-"`
	CreatedAt   time.Time       `json:"createdAt" yaml:"createdAt"`

	// Elicitation carries a structured schema-driven prompt when the assistant
	// needs additional user input. When non-nil the UI can render an
	// interactive form instead of plain text. It is omitted for all other
	// message kinds.
	Elicitation *plan.Elicitation `json:"elicitation,omitempty" yaml:"elicitation,omitempty"`

	// CallbackURL is set when the message requires a user action through a
	// dedicated REST callback (e.g. MCP elicitation). Empty for normal chat
	// messages.
	CallbackURL string `json:"callbackURL,omitempty" yaml:"callbackURL,omitempty"`

	// Status indicates the resolution state of interactive MCP prompts.
	// "open"   – waiting for user
	// "done"   – accepted and finished
	// "declined" – user declined
	Status string `json:"status,omitempty" yaml:"status,omitempty"`

	// Interaction contains details for an MCP user-interaction request (e.g.
	// "open the following URL and confirm when done"). When non-nil the UI
	// should render an approval card with a link and Accept/Decline buttons.
	Interaction *UserInteraction `json:"interaction,omitempty" yaml:"interaction,omitempty"`

	// PolicyApproval is non-nil when the system requires explicit user
	// approval before executing a potentially sensitive action (e.g. running
	// an external tool).  The UI should show the approval dialog when the
	// message role == "policyapproval" and Status == "open".
	PolicyApproval *PolicyApproval `json:"policyApproval,omitempty" yaml:"policyApproval,omitempty"`
}

// PolicyApproval captures the details of an approval request that needs an
// explicit Accept/Reject decision by the user.
type PolicyApproval struct {
	Tool   string                 `json:"tool" yaml:"tool"`                     // tool/function name such as "system.exec"
	Args   map[string]interface{} `json:"args,omitempty" yaml:"args,omitempty"` // flattened argument map
	Reason string                 `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// UserInteraction represents a structured prompt created via the MCP
// user-interaction feature.
type UserInteraction struct {
	URL         string `json:"url" yaml:"url"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type Attachments []Attachment

// Attachment describes a file linked to the message.
type Attachment struct {
	Name string `json:"name,omitempty" yaml:"name"`
	URL  string `json:"url,omitempty"  yaml:"url"`
	Size int64  `json:"size,omitempty" yaml:"size"` // bytes
	// MediaType allows UI to decide how to display or download.
	MediaType string `json:"mediaType,omitempty" yaml:"mediaType,omitempty"`
}

// EmbedFunc defines a function that creates embeddings for given texts.
// It should return one embedding per input text.
type EmbedFunc func(ctx context.Context, texts []string) ([][]float32, error)
