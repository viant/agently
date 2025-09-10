package memory

import (
	"context"
	"time"

	"github.com/viant/agently/genai/agent/plan"
)

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

// ModelMessageIDKey carries the message id to which the current model call should attach.
type modelMessageIDKey string

var ModelMessageIDKey = modelMessageIDKey("modelMessageID")

func ModelMessageIDFromContext(ctx context.Context) string {
	value := ctx.Value(ModelMessageIDKey)
	if value == nil {
		return ""
	}
	return value.(string)
}

// TurnMeta captures minimal per-turn context for downstream persistence.
// Prefer passing a single TurnMeta instead of scattering separate keys.
type TurnMeta struct {
	TurnID          string
	ConversationID  string
	ParentMessageID string // last user message id (or tool message when parenting final)
}

type turnMetaKeyT string

var turnMetaKey = turnMetaKeyT("turnMeta")

// WithTurnMeta stores TurnMeta on the context and also seeds individual keys
// for backward compatibility with existing readers.
func WithTurnMeta(ctx context.Context, meta TurnMeta) context.Context {

	if meta.ConversationID != "" {
		ctx = context.WithValue(ctx, ConversationIDKey, meta.ConversationID)
	}
	return context.WithValue(ctx, turnMetaKey, meta)
}

// TurnMetaFromContext returns a stored TurnMeta when present.
func TurnMetaFromContext(ctx context.Context) (TurnMeta, bool) {
	if ctx == nil {
		return TurnMeta{}, false
	}
	if v := ctx.Value(turnMetaKey); v != nil {
		if m, ok := v.(TurnMeta); ok {
			return m, true
		}
	}
	return TurnMeta{}, false
}

// Message represents a conversation message for memory storage.
type Message struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversationId"`
	ParentID       string  `json:"parentId,omitempty"`
	Role           string  `json:"role"`
	Actor          string  `json:"actor,omitempty" yaml:"actor,omitempty"`
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

	Interim *int `json:"interim,omitempty" yaml:"interim,omitempty"` // 1 for interim messages, nil or 0 otherwise
}

// ConversationMeta captures hierarchical metadata for a conversation. It is
// kept minimal so that additional fields can be added without breaking
// existing callers.
type ConversationMeta struct {
	ID         string    `json:"id"`
	ParentID   string    `json:"parentId,omitempty"`
	Title      string    `json:"title,omitempty"`
	Visibility string    `json:"visibility,omitempty"` // full|summary|none
	CreatedAt  time.Time `json:"createdAt"`

	// Model stores the last LLM model explicitly chosen by the user within
	// this conversation. When a subsequent turn omits the model override the
	// orchestration can fall back to this value so that the user does not
	// have to repeat the flag every time.
	Model string `json:"model,omitempty"`

	// Tools keeps the last explicit per-turn tool allow-list requested by the
	// user. When a subsequent turn sends an empty tools slice, orchestration
	// falls back to this stored list so the preference persists.
	Tools []string `json:"tools,omitempty"`

	// Agent records the last agent configuration reference (path or name)
	// explicitly used in the conversation so that subsequent requests can
	// omit the field and still continue the thread with the same agent.
	Agent string `json:"agent,omitempty"`

	// Context holds the latest accepted elicitation payload so that the user
	// does not have to resend the same data every turn when the same agent
	// schema still applies.
	Context map[string]interface{} `json:"context,omitempty"`
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
