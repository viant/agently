package operations

import (
	"context"
	"encoding/json"
	"fmt"

	daofactory "github.com/viant/agently/internal/dao/factory"
	mcread "github.com/viant/agently/internal/dao/modelcall/read"
	mcwrite "github.com/viant/agently/internal/dao/modelcall/write"
	pldaoRead "github.com/viant/agently/internal/dao/payload/read"
	tcread "github.com/viant/agently/internal/dao/toolcall/read"
	tcwrite "github.com/viant/agently/internal/dao/toolcall/write"
	d "github.com/viant/agently/internal/domain"
)

// Service adapts DAO model/tool-call repositories to the domain.Operations API.
type Service struct{ *daofactory.API }

// New constructs a Service instance from grouped DAO APIs.
func New(apis *daofactory.API) *Service { return &Service{API: apis} }

var _ d.Operations = (*Service)(nil)

func (a *Service) RecordModelCall(ctx context.Context, call *mcwrite.ModelCall, requestPayloadID, responsePayloadID string) error {
	if a == nil || a.API == nil || a.API.ModelCall == nil {
		return fmt.Errorf("modelcall API is not configured")
	}
	w := call
	if w == nil {
		w = &mcwrite.ModelCall{}
	}
	// If both times are present and latency not set, compute it.
	if w.StartedAt != nil && w.CompletedAt != nil && w.LatencyMS == nil {
		if ms := int(w.CompletedAt.Sub(*w.StartedAt).Milliseconds()); ms >= 0 {
			w.LatencyMS = &ms
			ensureMCHas(&w.Has)
			w.Has.LatencyMS = true
		}
	}
	// Reference payload ids when provided. Persisting payloads is handled by
	// higher level code; this adapter only wires IDs to the model_call row.
	if requestPayloadID != "" {
		w.RequestPayloadID = &requestPayloadID
		ensureMCHas(&w.Has)
		w.Has.RequestPayloadID = true
	}
	if responsePayloadID != "" {
		w.ResponsePayloadID = &responsePayloadID
		ensureMCHas(&w.Has)
		w.Has.ResponsePayloadID = true
	}
	_, err := a.API.ModelCall.Patch(ctx, w)
	return err
}

func (a *Service) RecordToolCall(ctx context.Context, call *tcwrite.ToolCall, requestPayloadID, responsePayloadID string) error {
	if a == nil || a.API == nil || a.API.ToolCall == nil {
		return fmt.Errorf("toolcall API is not configured")
	}
	w := call
	if w == nil {
		w = &tcwrite.ToolCall{}
	}
	// Derive latency if not already set
	if w.StartedAt != nil && w.CompletedAt != nil && w.LatencyMS == nil {
		if ms := int(w.CompletedAt.Sub(*w.StartedAt).Milliseconds()); ms >= 0 {
			w.LatencyMS = &ms
			if w.Has == nil {
				w.Has = &tcwrite.ToolCallHas{}
			}
			w.Has.LatencyMS = true
		}
	}
	// Persist payload IDs (not snapshots) similar to ModelCall
	if requestPayloadID != "" {
		w.RequestPayloadID = &requestPayloadID
		if w.Has == nil {
			w.Has = &tcwrite.ToolCallHas{}
		}
		w.Has.RequestPayloadID = true
	}
	if responsePayloadID != "" {
		w.ResponsePayloadID = &responsePayloadID
		if w.Has == nil {
			w.Has = &tcwrite.ToolCallHas{}
		}
		w.Has.ResponsePayloadID = true
	}
	_, err := a.API.ToolCall.Patch(ctx, w)
	return err
}

func ensureMCHas(h **mcwrite.ModelCallHas) {
	if *h == nil {
		*h = &mcwrite.ModelCallHas{}
	}
}

func (a *Service) GetByMessage(ctx context.Context, messageID string) ([]*d.Operation, error) {
	var out []*d.Operation
	if a == nil || a.API == nil {
		return nil, fmt.Errorf("operations service is not configured")
	}
	if a.API.ModelCall != nil {
		views, err := a.API.ModelCall.List(ctx, mcread.WithMessageID(messageID))
		if err != nil {
			return nil, err
		}
		for _, v := range views {
			op := &d.Operation{MessageID: v.MessageID}
			op.Model = &d.ModelCallTrace{Call: v}
			if a.API.Payload != nil {
				if v.RequestPayloadID != nil && *v.RequestPayloadID != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(*v.RequestPayloadID))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Model.Request = rows[0]
					}
				}
				if v.ResponsePayloadID != nil && *v.ResponsePayloadID != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(*v.ResponsePayloadID))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Model.Response = rows[0]
					}
				}
			}
			out = append(out, op)
		}
	}
	if a.API.ToolCall != nil {
		views, err := a.API.ToolCall.List(ctx, tcread.WithMessageID(messageID))
		if err != nil {
			return nil, err
		}
		for _, v := range views {
			op := &d.Operation{MessageID: v.MessageID}
			op.Tool = &d.ToolCallTrace{Call: v}
			if a.API.Payload != nil {
				if v.RequestPayloadID != nil && *v.RequestPayloadID != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(*v.RequestPayloadID))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Tool.Request = rows[0]
					}
				} else if id := payloadIDFromSnapshot(v.RequestSnapshot); id != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(id))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Tool.Request = rows[0]
					}
				}
				if v.ResponsePayloadID != nil && *v.ResponsePayloadID != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(*v.ResponsePayloadID))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Tool.Response = rows[0]
					}
				} else if id := payloadIDFromSnapshot(v.ResponseSnapshot); id != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(id))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Tool.Response = rows[0]
					}
				}
			}
			out = append(out, op)
		}
	}
	return out, nil
}

func (a *Service) GetByTurn(ctx context.Context, turnID string) ([]*d.Operation, error) {
	var out []*d.Operation
	if a == nil || a.API == nil {
		return nil, fmt.Errorf("operations service is not configured")
	}
	if a.API.ModelCall != nil {
		views, err := a.API.ModelCall.List(ctx, mcread.WithTurnID(turnID))
		if err != nil {
			return nil, err
		}
		for _, v := range views {
			op := &d.Operation{MessageID: v.MessageID, TurnID: v.TurnID}
			op.Model = &d.ModelCallTrace{Call: v}
			if a.API.Payload != nil {
				if v.RequestPayloadID != nil && *v.RequestPayloadID != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(*v.RequestPayloadID))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Model.Request = rows[0]
					}
				}
				if v.ResponsePayloadID != nil && *v.ResponsePayloadID != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(*v.ResponsePayloadID))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Model.Response = rows[0]
					}
				}
			}
			out = append(out, op)
		}
	}
	if a.API.ToolCall != nil {
		views, err := a.API.ToolCall.List(ctx, tcread.WithTurnID(turnID))
		if err != nil {
			return nil, err
		}
		for _, v := range views {
			op := &d.Operation{MessageID: v.MessageID, TurnID: v.TurnID}
			op.Tool = &d.ToolCallTrace{Call: v}
			if a.API.Payload != nil {
				if v.RequestPayloadID != nil && *v.RequestPayloadID != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(*v.RequestPayloadID))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Tool.Request = rows[0]
					}
				} else if id := payloadIDFromSnapshot(v.RequestSnapshot); id != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(id))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Tool.Request = rows[0]
					}
				}
				if v.ResponsePayloadID != nil && *v.ResponsePayloadID != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(*v.ResponsePayloadID))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Tool.Response = rows[0]
					}
				} else if id := payloadIDFromSnapshot(v.ResponseSnapshot); id != "" {
					rows, err := a.API.Payload.List(ctx, pldaoRead.WithID(id))
					if err != nil {
						return nil, err
					}
					if len(rows) > 0 {
						op.Tool.Response = rows[0]
					}
				}
			}
			out = append(out, op)
		}
	}
	return out, nil
}

// No internal payload writer here to avoid adapter import cycles.
// Helper to extract payloadId from snapshot JSON.
func payloadIDFromSnapshot(snapshot *string) string {
	if snapshot == nil || *snapshot == "" {
		return ""
	}
	var x struct {
		PayloadID string `json:"payloadId"`
	}
	if json.Unmarshal([]byte(*snapshot), &x) == nil {
		return x.PayloadID
	}
	return ""
}
