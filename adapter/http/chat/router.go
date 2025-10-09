package chat

import (
	"context"
	"fmt"
	"net/http"

	chat "github.com/viant/agently/client/chat"
	chstore "github.com/viant/agently/client/chat/store"
	agconv "github.com/viant/agently/pkg/agently/conversation"
	convw "github.com/viant/agently/pkg/agently/conversation/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository/contract"
	hstate "github.com/viant/xdatly/handler/state"
)

// Service provides access to chat store for router handlers.
type Service struct{ Store chstore.Client }

func Register(ctx context.Context, dao *datly.Service, router *datly.Router[Service]) error {
	// GET conversations list
	if err := router.Register(ctx, contract.NewPath(http.MethodGet, agconv.ConversationsPathURI), func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, _ ...datly.OperateOption) (interface{}, error) {
		in := agconv.ConversationInput{}
		if err := injector.Bind(ctx, &in); err != nil {
			return nil, err
		}
		out := &agconv.ConversationOutput{}
		if _, err := dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(agconv.ConversationsPathURI), datly.WithInput(&in)); err != nil {
			return nil, err
		}
		return out, nil
	}); err != nil {
		return err
	}

	// GET conversation by id
	if err := router.Register(ctx, contract.NewPath(http.MethodGet, agconv.ConversationPathURI), func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, _ ...datly.OperateOption) (interface{}, error) {
		in := agconv.ConversationInput{}
		if err := injector.Bind(ctx, &in); err != nil {
			return nil, err
		}
		out := &agconv.ConversationOutput{}
		uri := agconv.ConversationPathURI
		if _, err := dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in)); err != nil {
			return nil, err
		}
		return out, nil
	}); err != nil {
		return err
	}

	// PATCH conversation (create/update) via store client, marshal conv write Output
	if err := router.Register(ctx, contract.NewPath(http.MethodPatch, convw.PathURI), func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, _ ...datly.OperateOption) (interface{}, error) {
		return manageConversation(ctx, svc, injector)
	}); err != nil {
		return err
	}
	if err := router.Register(ctx, contract.NewPath(http.MethodPost, convw.PathURI), func(ctx context.Context, svc Service, r *http.Request, injector hstate.Injector, _ ...datly.OperateOption) (interface{}, error) {
		return manageConversation(ctx, svc, injector)
	}); err != nil {
		return err
	}
	return nil
}

func manageConversation(ctx context.Context, svc Service, injector hstate.Injector) (interface{}, error) {
	in := convw.Input{}

	envelope := datly.BodyEnvelope[*convw.Conversation]{}
	if err := injector.Bind(ctx, &envelope); err != nil {
		return nil, err
	}
	if envelope.Body != nil {
		in.Conversations = append(in.Conversations, envelope.Body)
	}

	if len(in.Conversations) == 0 {
		return nil, fmt.Errorf("conversations payload required")
	}

	// Apply each conversation change via store adapter
	for _, c := range in.Conversations {
		if c == nil {
			continue
		}
		if err := svc.Store.PatchConversations(ctx, (*chat.MutableConversation)(c)); err != nil {
			return nil, err
		}
	}
	// Return a standard write Output, echoing payload as Data
	return &convw.Output{Data: in.Conversations}, nil
}
