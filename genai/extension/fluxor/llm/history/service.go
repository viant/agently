package history

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/extension/fluxor/llm/core"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/summary"
	"github.com/viant/fluxor/model/types"
)

const serviceName = "llm/history"

// Service exposes helpers for interacting with conversation history inside
// Fluxor workflows. It is intentionally lightweight so other packages (agent,
// run, etc.) remain focused on their primary concern.
type Service struct {
	hist memory.History
	llm  *core.Service
}

func New(hist memory.History, llm *core.Service) *Service { return &Service{hist: hist, llm: llm} }

// ------------------------------------------------------------------
// Executable input/output structs
// ------------------------------------------------------------------

type AddMessageInput struct {
	ConversationID string             `json:"conversationId,omitempty"`
	ParentID       string             `json:"parentId,omitempty"`
	Role           string             `json:"role"`
	Actor          string             `json:"actor,omitempty"`
	Content        string             `json:"content"`
	Attachments    memory.Attachments `json:"attachments,omitempty"`
}

type AddMessageOutput struct {
	ID string `json:"id"`
}

type MessagesInput struct {
	ConversationID   string `json:"conversationId,omitempty"`
	SinceID          string `json:"sinceId,omitempty"`
	IncludeSummaries bool   `json:"includeSummaries,omitempty"`
}

type MessagesOutput struct {
	Messages []memory.Message `json:"messages"`
}

type LastNInput struct {
	ConversationID   string `json:"conversationId,omitempty"`
	N                int    `json:"n"`
	IncludeSummaries bool   `json:"includeSummaries,omitempty"`
}

type LastNOutput struct {
	Messages []memory.Message `json:"messages"`
}

// -------------------- Compact -------------------------------------------

type CompactInput struct {
	ConversationID string `json:"conversationId,omitempty"`
	Threshold      int    `json:"threshold"` // minimum messages before compaction
	LastN          int    `json:"lastN"`     // messages to keep intact at tail
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
}

type CompactOutput struct {
	SummaryID string `json:"summaryId,omitempty"`
}

// --------------------- Clear -------------------------------------------

type ClearInput struct {
	ConversationID string `json:"conversationId,omitempty"`
}

type ClearOutput struct{}

type SummarizeInput struct {
	ConversationID string `json:"conversationId,omitempty"`
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
	LastN          int    `json:"lastN,omitempty"`
}

type SummarizeOutput struct {
	Summary string `json:"summary"`
}

// ------------------------------------------------------------------
// Fluxor service interface implementation
// ------------------------------------------------------------------

func (s *Service) Name() string { return serviceName }

func (s *Service) Methods() types.Signatures {
	return []types.Signature{
		{
			Name:   "addMessage",
			Input:  reflect.TypeOf(&AddMessageInput{}),
			Output: reflect.TypeOf(&AddMessageOutput{}),
		},
		{
			Name:   "messages",
			Input:  reflect.TypeOf(&MessagesInput{}),
			Output: reflect.TypeOf(&MessagesOutput{}),
		},
		{
			Name:   "lastN",
			Input:  reflect.TypeOf(&LastNInput{}),
			Output: reflect.TypeOf(&LastNOutput{}),
		},
		{
			Name:   "summarize",
			Input:  reflect.TypeOf(&SummarizeInput{}),
			Output: reflect.TypeOf(&SummarizeOutput{}),
		},
		{
			Name:   "compact",
			Input:  reflect.TypeOf(&CompactInput{}),
			Output: reflect.TypeOf(&CompactOutput{}),
		},
		{
			Name:   "clear",
			Input:  reflect.TypeOf(&ClearInput{}),
			Output: reflect.TypeOf(&ClearOutput{}),
		},
	}
}

func (s *Service) Method(name string) (types.Executable, error) {
	switch strings.ToLower(name) {
	case "addmessage":
		return s.addMessage, nil
	case "messages":
		return s.messages, nil
	case "lastn":
		return s.lastN, nil
	case "summarize":
		return s.summarize, nil
	case "compact":
		return s.compact, nil
	case "clear":
		return s.clear, nil
	}
	return nil, types.NewMethodNotFoundError(name)
}

// ------------------------------------------------------------------
// Executable implementations
// ------------------------------------------------------------------

func (s *Service) addMessage(ctx context.Context, in, out interface{}) error {
	if s == nil || s.hist == nil {
		return nil
	}
	arg := in.(*AddMessageInput)
	res := out.(*AddMessageOutput)

	convID := arg.ConversationID
	if strings.TrimSpace(convID) == "" {
		convID = memory.ConversationIDFromContext(ctx)
	}
	if convID == "" {
		return types.NewInvalidInputError("conversationId is required")
	}

	msg := memory.Message{
		ID:             uuid.New().String(),
		ConversationID: convID,
		ParentID:       arg.ParentID,
		Role:           arg.Role,
		Actor:          arg.Actor,
		Content:        arg.Content,
		Attachments:    arg.Attachments,
	}
	if err := s.hist.AddMessage(ctx, msg); err != nil {
		return err
	}
	res.ID = msg.ID
	return nil
}

func filterSummaries(msgs []memory.Message) []memory.Message {
	var out []memory.Message
	for _, m := range msgs {
		if m.Status == "summary" || m.Status == "summarized" {
			continue
		}
		out = append(out, m)
	}
	return out
}

// compact performs summarisation + flagging of older messages.
func (s *Service) compact(ctx context.Context, in, out interface{}) error {
	arg := in.(*CompactInput)
	res := out.(*CompactOutput)

	if s.llm == nil {
		return types.NewInvalidInputError("llm core not configured")
	}

	convID := strings.TrimSpace(arg.ConversationID)
	if convID == "" {
		convID = memory.ConversationIDFromContext(ctx)
	}
	if convID == "" {
		return types.NewInvalidInputError("conversationId is required")
	}

	msgs, err := s.hist.GetMessages(ctx, convID)
	if err != nil {
		return err
	}
	if arg.Threshold <= 0 || len(msgs) <= arg.Threshold {
		return nil // nothing to do
	}

	keepN := arg.LastN
	if keepN <= 0 {
		keepN = 10
	}
	if keepN > len(msgs) {
		keepN = len(msgs)
	}

	// identify initial boundaries
	keepStart := len(msgs) - keepN
	if keepStart < 0 {
		keepStart = 0
	}

	// ensure at least one message with attachment survives
	hasAttachment := func(items []memory.Message) bool {
		for _, m := range items {
			if len(m.Attachments) > 0 {
				return true
			}
		}
		return false
	}

	if !hasAttachment(msgs[keepStart:]) {
		// search summarized part backwards for a message with attachment
		for i := keepStart - 1; i >= 0; i-- {
			if len(msgs[i].Attachments) > 0 {
				keepStart = i // include from this message onwards
				break
			}
		}
	}

	summarized := msgs[:keepStart]

	// generate summary
	summaryText, err := summary.Summarize(ctx, s.hist, s.llm, arg.Model, convID, len(summarized), arg.Prompt)
	if err != nil {
		return err
	}

	// Add summary message
	summaryMsg := memory.Message{
		ID:             uuid.New().String(),
		ConversationID: convID,
		Role:           "system",
		Status:         "summary",
		Content:        summaryText,
	}
	if err := s.hist.AddMessage(ctx, summaryMsg); err != nil {
		return err
	}
	res.SummaryID = summaryMsg.ID

	// mark old messages
	for _, m := range summarized {
		mid := m.ID
		_ = s.hist.UpdateMessage(ctx, mid, func(mm *memory.Message) {
			mm.Status = "summarized"
		})
	}
	return nil
}

func (s *Service) clear(ctx context.Context, in, out interface{}) error {
	arg := in.(*ClearInput)
	convID := strings.TrimSpace(arg.ConversationID)
	if convID == "" {
		convID = memory.ConversationIDFromContext(ctx)
	}
	if convID == "" {
		return types.NewInvalidInputError("conversationId is required")
	}
	if deleter, ok := s.hist.(interface {
		Delete(context.Context, string) error
	}); ok {
		return deleter.Delete(ctx, convID)
	}
	return fmt.Errorf("history store does not support delete")
}

func (s *Service) messages(ctx context.Context, in, out interface{}) error {
	arg := in.(*MessagesInput)
	res := out.(*MessagesOutput)

	convID := arg.ConversationID
	if strings.TrimSpace(convID) == "" {
		convID = memory.ConversationIDFromContext(ctx)
	}
	if convID == "" {
		return types.NewInvalidInputError("conversationId is required")
	}

	var msgs []memory.Message
	var err error
	msgs, err = s.hist.GetMessages(ctx, convID)
	if err == nil && arg.SinceID != "" {
		// emulate MessagesSince when implementation does not provide it
		start := -1
		for i := range msgs {
			if msgs[i].ID == arg.SinceID {
				start = i
				break
			}
		}
		if start >= 0 {
			msgs = msgs[start:]
		} else {
			msgs = []memory.Message{}
		}
	}
	if err != nil {
		return err
	}
	if !arg.IncludeSummaries {
		msgs = filterSummaries(msgs)
	}
	res.Messages = msgs
	return nil
}

func (s *Service) lastN(ctx context.Context, in, out interface{}) error {
	arg := in.(*LastNInput)
	res := out.(*LastNOutput)

	convID := arg.ConversationID
	if strings.TrimSpace(convID) == "" {
		convID = memory.ConversationIDFromContext(ctx)
	}
	if convID == "" {
		return types.NewInvalidInputError("conversationId is required")
	}

	policy := memory.NewLastNPolicy(arg.N)
	msgs, err := s.hist.Retrieve(ctx, convID, policy)
	if err != nil {
		return err
	}
	if !arg.IncludeSummaries {
		msgs = filterSummaries(msgs)
	}
	res.Messages = msgs
	return nil
}

func (s *Service) summarize(ctx context.Context, in, out interface{}) error {
	arg := in.(*SummarizeInput)
	res := out.(*SummarizeOutput)

	convID := strings.TrimSpace(arg.ConversationID)
	if convID == "" {
		convID = memory.ConversationIDFromContext(ctx)
	}
	if convID == "" {
		return types.NewInvalidInputError("conversationId is required")
	}
	if s.llm == nil {
		return types.NewInvalidInputError("llm core not configured")
	}

	lastN := arg.LastN
	if lastN <= 0 {
		lastN = 10
	}

	summaryText, err := summary.Summarize(ctx, s.hist, s.llm, arg.Model, convID, lastN, arg.Prompt)
	if err != nil {
		return err
	}
	res.Summary = summaryText
	return nil
}
