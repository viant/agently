package message

import (
	"context"
	"fmt"

	daofactory "github.com/viant/agently/internal/dao/factory"
	"github.com/viant/agently/internal/domain"
	msgwrite "github.com/viant/agently/pkg/agently/message"
)

// Service adapts DAO message/modelcall/toolcall/payload to the domain.Messages API.
type Service struct{ *daofactory.API }

func New(apis *daofactory.API) *Service { return &Service{API: apis} }

var _ domain.Messages = (*Service)(nil)

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
