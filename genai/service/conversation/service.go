package messages

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	plan "github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/stage"
	msgread "github.com/viant/agently/internal/dao/message/read"
	d "github.com/viant/agently/internal/domain"
)

// Service exposes message retrieval independent of HTTP concerns.
type Service struct {
	store d.Store
}

func NewService(store d.Store) *Service { return &Service{store: store} }

// GetRequest defines inputs to fetch messages.
type GetRequest struct {
	ConversationID string
	SinceID        string // optional: inclusive slice starting from this message id
}

// GetResponse carries fetched messages, inferred stage and progress flag.
type GetResponse struct {
	Messages   []memory.Message
	Stage      *stage.Stage
	InProgress bool // true when SinceID set and no new messages yet
}

// Get fetches messages according to request and computes conversation stage.
func (s *Service) Get(ctx context.Context, req GetRequest) (*GetResponse, error) {
	opts := []msgread.InputOption{msgread.WithInterim(0), msgread.WithConversationID(req.ConversationID)}
	if id := strings.TrimSpace(req.SinceID); id != "" {
		// Some callers may pass synthetic suffixes (e.g. "/form"). Strip anything after first '/'.
		if idx := strings.IndexByte(id, '/'); idx > 0 {
			id = id[:idx]
		}
		opts = append(opts, msgread.WithSinceID(id))
	}

	views, err := s.store.Messages().GetTranscript(ctx, req.ConversationID, opts...)
	if err != nil {
		return nil, err
	}

	msgs := make([]memory.Message, 0, len(views))
	for _, v := range views {
		if v == nil || v.IsInterim() {
			continue
		}
		mm := memory.Message{ID: v.Id, ConversationID: v.ConversationID, Role: v.Role, Content: v.Content}
		if v.ParentID != nil {
			mm.ParentID = *v.ParentID
		}
		if v.ToolName != nil {
			mm.ToolName = v.ToolName
		}
		if v.CreatedAt != nil {
			mm.CreatedAt = *v.CreatedAt
		} else {
			mm.CreatedAt = time.Now()
		}
		if v.Elicitation != nil {
			mm.Elicitation = v.Elicitation
		}
		// Inline a single-step outcome for tool messages, including embedded payloads when available.
		if strings.EqualFold(strings.TrimSpace(v.Role), "tool") && v.ToolCall != nil {
			st := &plan.StepOutcome{
				ID:                v.ToolCall.OpID,
				Name:              v.ToolCall.ToolName,
				Reason:            v.Content,
				Success:           strings.EqualFold(strings.TrimSpace(v.ToolCall.Status), "completed"),
				Error:             derefStr(v.ToolCall.ErrorMessage),
				StartedAt:         v.ToolCall.StartedAt,
				EndedAt:           v.ToolCall.CompletedAt,
				RequestPayloadID:  v.ToolCall.RequestPayloadID,
				ResponsePayloadID: v.ToolCall.ResponsePayloadID,
			}
			if v.ToolCall.StartedAt != nil && v.ToolCall.CompletedAt != nil {
				st.Elapsed = v.ToolCall.CompletedAt.Sub(*v.ToolCall.StartedAt).Round(time.Millisecond).String()
			}
			if err := s.inlinePayload(ctx, v.ToolCall.RequestPayloadID, &st.Request); err != nil {
				return nil, err
			}
			if err := s.inlinePayload(ctx, v.ToolCall.ResponsePayloadID, &st.Response); err != nil {
				return nil, err
			}
			mm.Executions = []*plan.Outcome{{Steps: []*plan.StepOutcome{st}}}
		}
		msgs = append(msgs, mm)
	}

	stg := s.currentStage(ctx, req.ConversationID)
	inProgress := strings.TrimSpace(req.SinceID) != "" && len(msgs) == 0
	return &GetResponse{Messages: msgs, Stage: stg, InProgress: inProgress}, nil
}

func (s *Service) inlinePayload(ctx context.Context, id *string, out *json.RawMessage) error {
	if id == nil || strings.TrimSpace(*id) == "" {
		return nil
	}
	pv, err := s.store.Payloads().Get(ctx, *id)
	if err != nil {
		return err
	}
	if pv != nil && pv.InlineBody != nil {
		*out = json.RawMessage(*pv.InlineBody)
	}
	return nil
}

// currentStage infers the live phase of a conversation using transcript signals.
func (s *Service) currentStage(ctx context.Context, convID string) *stage.Stage {
	st := &stage.Stage{Phase: stage.StageWaiting}
	if s == nil || s.store == nil || strings.TrimSpace(convID) == "" {
		return st
	}
	views, err := s.store.Messages().GetTranscript(ctx, convID)
	if err != nil || len(views) == 0 {
		return st
	}
	lastRole := ""
	lastAssistantElic := false
	lastToolRunning := false
	lastToolFailed := false
	lastModelRunning := false
	for i := len(views) - 1; i >= 0; i-- {
		v := views[i]
		if v == nil || v.IsInterim() {
			continue
		}
		r := strings.ToLower(strings.TrimSpace(v.Role))
		if lastRole == "" {
			lastRole = r
		}
		if v.ToolCall != nil {
			status := strings.ToLower(strings.TrimSpace(v.ToolCall.Status))
			if status == "running" || v.ToolCall.CompletedAt == nil {
				lastToolRunning = true
				break
			}
			if status == "failed" {
				lastToolFailed = true
			}
		}
		if v.ModelCall != nil {
			mstatus := strings.ToLower(strings.TrimSpace(v.ModelCall.Status))
			if mstatus == "running" || v.ModelCall.CompletedAt == nil {
				lastModelRunning = true
				break
			}
		}
		if r == "assistant" && v.Elicitation != nil {
			lastAssistantElic = true
			break
		}
	}

	switch {
	case lastToolRunning:
		st.Phase = stage.StageExecuting
	case lastAssistantElic:
		st.Phase = stage.StageEliciting
	case lastModelRunning:
		st.Phase = stage.StageThinking
	case lastRole == "user":
		st.Phase = stage.StageThinking
	case lastToolFailed:
		st.Phase = stage.StageError
	default:
		st.Phase = stage.StageDone
	}
	return st
}

func derefStr(p *string) string {
	if p != nil {
		return *p
	}
	return ""
}
