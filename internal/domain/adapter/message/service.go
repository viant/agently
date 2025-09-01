package message

import (
	"context"
	"fmt"

	daofactory "github.com/viant/agently/internal/dao/factory"
	msgread "github.com/viant/agently/internal/dao/message/read"
	msgwrite "github.com/viant/agently/internal/dao/message/write"
	mcread "github.com/viant/agently/internal/dao/modelcall/read"
	pldaoRead "github.com/viant/agently/internal/dao/payload/read"
	tcread "github.com/viant/agently/internal/dao/toolcall/read"
	d "github.com/viant/agently/internal/domain"
)

// Service adapts DAO message/modelcall/toolcall/payload to the domain.Messages API.
type Service struct{ *daofactory.API }

func New(apis *daofactory.API) *Service { return &Service{API: apis} }

var _ d.Messages = (*Service)(nil)

// Append using DAO write model (convenience outside interface); returns Id.
func (a *Service) Append(ctx context.Context, m *msgwrite.Message) (string, error) {
	if a == nil || a.API == nil || a.API.Message == nil {
		return "", fmt.Errorf("message service is not configured")
	}
	if m == nil {
		return "", fmt.Errorf("nil message")
	}
	out, err := a.API.Message.Patch(ctx, m)
	if err != nil {
		return "", err
	}
	if out != nil && len(out.Data) > 0 && out.Data[0] != nil && out.Data[0].Id != "" {
		return out.Data[0].Id, nil
	}
	return m.Id, nil
}

func ensureMsgHas(h **msgwrite.MessageHas) {
	if *h == nil {
		*h = &msgwrite.MessageHas{}
	}
}

// Patch implements domain.Messages.Patch by delegating to DAO.
func (a *Service) Patch(ctx context.Context, messages ...*msgwrite.Message) (*msgwrite.Output, error) {
	if a == nil || a.API == nil || a.API.Message == nil {
		return nil, fmt.Errorf("message service is not configured")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages to patch")
	}
	return a.API.Message.Patch(ctx, messages...)
}

// List implements domain.Messages.List (DAO read view).
func (a *Service) List(ctx context.Context, opts ...msgread.InputOption) ([]*msgread.MessageView, error) {
	if a == nil || a.API == nil || a.API.Message == nil {
		return nil, fmt.Errorf("message service is not configured")
	}
	return a.API.Message.List(ctx, opts...)
}

// GetTranscript implements domain.Messages.GetTranscript (DAO read view).
func (a *Service) GetTranscript(ctx context.Context, conversationID, turnID string, opts ...msgread.InputOption) ([]*msgread.MessageView, error) {
	if a == nil || a.API == nil || a.API.Message == nil {
		return nil, fmt.Errorf("message service is not configured")
	}
	return a.API.Message.GetTranscript(ctx, conversationID, turnID, opts...)
}

// Legacy helpers removed in favour of DAO-level read methods.

func (a *Service) GetTranscriptAggregated(ctx context.Context, conversationID, turnID string, opts d.TranscriptAggOptions) (*d.AggregatedTranscript, error) {
	if a == nil || a.API == nil || a.API.Message == nil {
		return nil, fmt.Errorf("message service is not configured")
	}
	if opts.IncludeModelCalls && a.API.ModelCall == nil {
		return nil, fmt.Errorf("modelcall API is not configured")
	}
	if opts.IncludeToolCalls && a.API.ToolCall == nil {
		return nil, fmt.Errorf("toolcall API is not configured")
	}
	in := []msgread.InputOption{}
	if opts.ExcludeInterim {
		in = append(in, msgread.WithInterim(0))
	}
	if opts.Since != nil {
		in = append(in, msgread.WithSince(*opts.Since))
	}
	views, err := a.API.Message.GetTranscript(ctx, conversationID, turnID, in...)
	if err != nil {
		return nil, err
	}
	agg := &d.AggregatedTranscript{Messages: make([]*d.AggregatedMessage, 0, len(views))}
	for _, v := range views {
		if !opts.IncludeTools && v.Role == "tool" {
			continue
		}
		msg := mapMessage(v)
		am := &d.AggregatedMessage{Message: msg}

		if opts.IncludeModelCalls {
			mc := v.ModelCall
			if mc == nil && a.API != nil && a.API.ModelCall != nil {
				if rows, _ := a.API.ModelCall.List(ctx, mcread.WithMessageID(v.Id)); len(rows) > 0 {
					row := rows[0]
					am.Model = &d.ModelCallTrace{Call: row}
					if row.RequestPayloadID != nil {
						if a.API == nil || a.API.Payload == nil {
							return nil, fmt.Errorf("payload API is not configured (model request payload)")
						}
						if p, _ := a.API.Payload.List(ctx, pldaoRead.WithID(*row.RequestPayloadID)); len(p) > 0 {
							am.Model.Request = mapPayloadView(p[0], opts)
						}
					}
					if row.ResponsePayloadID != nil {
						if a.API == nil || a.API.Payload == nil {
							return nil, fmt.Errorf("payload API is not configured (model response payload)")
						}
						if p, _ := a.API.Payload.List(ctx, pldaoRead.WithID(*row.ResponsePayloadID)); len(p) > 0 {
							am.Model.Response = mapPayloadView(p[0], opts)
						}
					}
				}
			}
			if mc != nil && am.Model == nil {
				// Map message-embedded view to modelcall/read view
				mv := &mcread.ModelCallView{
					MessageID:         mc.MessageID,
					Provider:          mc.Provider,
					Model:             mc.Model,
					ModelKind:         mc.ModelKind,
					PromptTokens:      mc.PromptTokens,
					CompletionTokens:  mc.CompletionTokens,
					TotalTokens:       mc.TotalTokens,
					FinishReason:      mc.FinishReason,
					CacheHit:          mc.CacheHit,
					StartedAt:         mc.StartedAt,
					CompletedAt:       mc.CompletedAt,
					LatencyMS:         mc.LatencyMS,
					Cost:              mc.Cost,
					TraceID:           mc.TraceID,
					SpanID:            mc.SpanID,
					RequestPayloadID:  mc.RequestPayloadID,
					ResponsePayloadID: mc.ResponsePayloadID,
				}
				am.Model = &d.ModelCallTrace{Call: mv}
				if mc.RequestPayloadID != nil {
					if a.API == nil || a.API.Payload == nil {
						return nil, fmt.Errorf("payload API is not configured (model request payload)")
					}
					if p, _ := a.API.Payload.List(ctx, pldaoRead.WithID(*mc.RequestPayloadID)); len(p) > 0 {
						am.Model.Request = mapPayloadView(p[0], opts)
					}
				}
				if mc.ResponsePayloadID != nil {
					if a.API == nil || a.API.Payload == nil {
						return nil, fmt.Errorf("payload API is not configured (model response payload)")
					}
					if p, _ := a.API.Payload.List(ctx, pldaoRead.WithID(*mc.ResponsePayloadID)); len(p) > 0 {
						am.Model.Response = mapPayloadView(p[0], opts)
					}
				}
			}
		}

		if opts.IncludeToolCalls {
			tc := v.ToolCall
			if tc == nil && a.API != nil && a.API.ToolCall != nil {
				if rows, _ := a.API.ToolCall.List(ctx, tcread.WithMessageID(v.Id)); len(rows) > 0 {
					row := rows[0]
					am.Tool = &d.ToolCallTrace{Call: row}
					if opts.PayloadLevel != d.PayloadNone && opts.PayloadLevel != d.PayloadFull {
						if row.RequestSnapshot != nil {
							am.Tool.Request = &pldaoRead.PayloadView{Kind: "tool_request", MimeType: "application/json", SizeBytes: len(*row.RequestSnapshot), Storage: "inline", Preview: row.RequestSnapshot}
						}
						if row.ResponseSnapshot != nil {
							am.Tool.Response = &pldaoRead.PayloadView{Kind: "tool_response", MimeType: "application/json", SizeBytes: len(*row.ResponseSnapshot), Storage: "inline", Preview: row.ResponseSnapshot}
						}
					}
				}
			}
			if tc != nil && am.Tool == nil {
				tv := &tcread.ToolCallView{
					MessageID:        tc.MessageID,
					TurnID:           v.TurnID,
					OpID:             tc.OpID,
					Attempt:          tc.Attempt,
					ToolName:         tc.ToolName,
					ToolKind:         tc.ToolKind,
					CapabilityTags:   tc.CapabilityTags,
					ResourceURIs:     tc.ResourceURIs,
					Status:           tc.Status,
					RequestSnapshot:  tc.RequestSnapshot,
					RequestHash:      tc.RequestHash,
					ResponseSnapshot: tc.ResponseSnapshot,
					ErrorCode:        tc.ErrorCode,
					ErrorMessage:     tc.ErrorMessage,
					Retriable:        tc.Retriable,
					StartedAt:        tc.StartedAt,
					CompletedAt:      tc.CompletedAt,
					LatencyMS:        tc.LatencyMS,
					Cost:             tc.Cost,
					TraceID:          tc.TraceID,
					SpanID:           tc.SpanID,
				}
				am.Tool = &d.ToolCallTrace{Call: tv}
				if opts.PayloadLevel != d.PayloadNone && opts.PayloadLevel != d.PayloadFull {
					if tc.RequestSnapshot != nil {
						am.Tool.Request = &pldaoRead.PayloadView{Kind: "tool_request", MimeType: "application/json", SizeBytes: len(*tc.RequestSnapshot), Storage: "inline", Preview: tc.RequestSnapshot}
					}
					if tc.ResponseSnapshot != nil {
						am.Tool.Response = &pldaoRead.PayloadView{Kind: "tool_response", MimeType: "application/json", SizeBytes: len(*tc.ResponseSnapshot), Storage: "inline", Preview: tc.ResponseSnapshot}
					}
				}
			}
		}
		agg.Messages = append(agg.Messages, am)
	}
	return agg, nil
}

func mapMessage(v *msgread.MessageView) *d.TranscriptMessage {
	return &d.TranscriptMessage{ID: v.Id, ConversationID: v.ConversationID, TurnID: v.TurnID, Sequence: v.Sequence, CreatedAt: v.CreatedAt,
		Role: v.Role, Type: v.Type, Content: v.Content, Interim: v.Interim, ToolName: v.ToolName,
		ModelCallPresent: v.ModelCallPresent, ToolCallPresent: v.ToolCallPresent,
	}
}

func mapPayloadView(v *pldaoRead.PayloadView, opts d.TranscriptAggOptions) *pldaoRead.PayloadView {
	if v == nil {
		return nil
	}
	// Work on a shallow copy to avoid mutating DAO row used elsewhere
	out := *v
	switch opts.PayloadLevel {
	case d.PayloadNone:
		return nil
	case d.PayloadFull:
		// keep InlineBody as-is
	case d.PayloadInlineIfSmall:
		if out.InlineBody != nil && opts.PayloadInlineMaxB > 0 && len(*out.InlineBody) > opts.PayloadInlineMaxB {
			out.InlineBody = nil
		}
	case d.PayloadPreview:
		out.InlineBody = nil
	}
	if opts.RedactSensitive {
		if out.Redacted != nil && *out.Redacted == 1 {
			out.InlineBody = nil
		}
	}
	return &out
}

// Append is a convenience helper that accepts a domain.Message, converts it to
// DAO write model and delegates to Patch. It is not part of the domain.Messages
// interface but kept for transition where callers still build domain.Message.
// (duplicate removed; method declared earlier)
