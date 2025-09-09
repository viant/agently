package domain

import (
	"time"

	mcread "github.com/viant/agently/internal/dao/modelcall/read"
	pldaoRead "github.com/viant/agently/internal/dao/payload/read"
	tcread "github.com/viant/agently/internal/dao/toolcall/read"
)

// AggregatedTranscript is a self-contained transcript view that aggregates
// per-message related operations (model/tool calls) and optionally usage.
// Implementations should populate only the requested parts based on
// TranscriptAggOptions.
type AggregatedTranscript struct {
	Messages []*AggregatedMessage `json:"messages"`
}

// AggregatedMessage combines the base message with its related call aggregates
// so that callers can render or forward a complete view without additional
// repository roundtrips.
type AggregatedMessage struct {
	Message *TranscriptMessage `json:"message"`
	Model   *ModelCallTrace    `json:"model,omitempty"`
	Tool    *ToolCallTrace     `json:"tool,omitempty"`
}

// TranscriptMessage is a lightweight message DTO dedicated to aggregated
// transcript responses. It avoids duplicating storage models elsewhere.
type TranscriptMessage struct {
	ID             string
	ConversationID string
	TurnID         *string
	Sequence       *int
	CreatedAt      *time.Time
	Role           string
	Type           string
	Content        string
	Interim        *int
	ToolName       *string
}

// ModelCallTrace represents a model call attached to a message in aggregated transcript.
type ModelCallTrace struct {
	Call     *mcread.ModelCallView
	Request  *pldaoRead.PayloadView
	Response *pldaoRead.PayloadView
}

// ToolCallTrace represents a tool call attached to a message in aggregated transcript.
type ToolCallTrace struct {
	Call     *tcread.ToolCallView
	Request  *pldaoRead.PayloadView
	Response *pldaoRead.PayloadView
}

// (ModelCall and ToolCall domain duplicates removed; traces embed DAO read views.)

// Operation is a normalized view of a recorded model or tool call associated
// with a message or turn scope used by Operations interface.
type Operation struct {
	ID        string
	MessageID string
	TurnID    *string
	Model     *ModelCallTrace
	Tool      *ToolCallTrace
}
