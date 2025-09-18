package impl

import (
	"context"
	"os"
	"strings"
	"unsafe"

	agconv "github.com/viant/agently/pkg/agently/conversation"
	api "github.com/viant/agently/sdk/conversation"
	"github.com/viant/datly"
	"github.com/viant/datly/view"
)

type Service struct{ dao *datly.Service }

// NewFromEnv constructs a Service using AGENTLY_DB_* environment settings and
// registers the rich conversation component.
func NewFromEnv(ctx context.Context) (*Service, error) {
	dao, err := datly.New(ctx)

	if err != nil {
		return nil, err
	}
	driver := strings.TrimSpace(os.Getenv("AGENTLY_DB_DRIVER"))
	dsn := strings.TrimSpace(os.Getenv("AGENTLY_DB_DSN"))
	if driver == "" || dsn == "" {
		// Return service anyway; Operate will fail if used. Caller can check env earlier.
		// Keeping behavior simple: attach only when configured.
		return &Service{dao: dao}, nil
	}
	_ = dao.AddConnectors(ctx, view.NewConnector("agently", driver, dsn))

	if err := agconv.DefineConversationComponent(ctx, dao); err != nil {
		return nil, err
	}
	if err := agconv.DefineConversationsComponent(ctx, dao); err != nil {
		return nil, err
	}

	return &Service{dao: dao}, nil
}

// GetConversations implements conversation.API using the generated component and returns SDK Conversation.
func (s *Service) GetConversations(ctx context.Context) ([]*api.Conversation, error) {
	inSDK := api.Input{IncludeTranscript: false, Has: &agconv.ConversationInputHas{IncludeTranscript: true}}
	// Map SDK input to generated input
	in := agconv.ConversationInput(inSDK)
	out := &agconv.ConversationOutput{}
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(agconv.ConversationsPathURI), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	result := *(*[]*api.Conversation)(unsafe.Pointer(&out.Data))
	return result, nil
}

// GetConversation implements conversation.API using the generated component and returns SDK Conversation.
func (s *Service) GetConversation(ctx context.Context, id string, options ...api.Option) (*api.Conversation, error) {
	if s == nil || s.dao == nil {
		return nil, nil
	}
	// Build SDK input via options
	inSDK := api.Input{Id: id, Has: &agconv.ConversationInputHas{Id: true}}
	for _, opt := range options {
		if opt != nil {
			opt(&inSDK)
		}
	}
	// Map SDK input to generated input
	in := agconv.ConversationInput(inSDK)

	out := &agconv.ConversationOutput{}
	uri := strings.ReplaceAll(agconv.ConversationPathURI, "{id}", id)
	if _, err := s.dao.Operate(ctx, datly.WithOutput(out), datly.WithURI(uri), datly.WithInput(&in)); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, nil
	}
	// Cast generated to SDK type
	conv := api.Conversation(*out.Data[0])
	return &conv, nil
}
