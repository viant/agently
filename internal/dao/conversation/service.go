package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/viant/agently/genai/memory"
	post2 "github.com/viant/agently/internal/dao/conversation/post"
	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
	"net/http"
	"strings"
)

type Service struct {
	dao *datly.Service
}

func (s *Service) AddMessage(ctx context.Context, convID string, msg memory.Message) error {

	input := post2.Input{
		Conversations: []*post2.Conversation{
			{
				Id: convID,
				Message: []*post2.Message{
					{
						Id:       uuid.New().String(),
						Role:     msg.Role,
						Content:  msg.Content,
						ToolName: msg.ToolName,
					},
				},
			},
		},
	}
	output := &post2.Output{}
	_, err := s.dao.Operate(ctx, datly.WithOutput(output), datly.WithInput(&input), datly.WithPath(contract.NewPath(http.MethodPost, post2.PathURI)))
	if err != nil {
		return err
	}
	if len(output.Violations) > 0 {
		data, _ := json.Marshal(output.Violations)
		return fmt.Errorf("failed to add message: %s", data)
	}
	return nil
}

func (s *Service) GetConversation(ctx context.Context, convID string) (*ConversationView, error) {
	output := &ConversationOutput{}
	URI := strings.ReplaceAll(ConversationPathURI, "{id}", convID)
	_, err := s.dao.Operate(ctx, datly.WithOutput(output),
		datly.WithURI(URI),
		datly.WithInput(&ConversationInput{Id: convID}))
	if err != nil {
		return nil, err
	}
	if len(output.Data) == 0 {
		return nil, nil // No conversation found
	}
	return output.Data[0], nil
}

func (s *Service) GetMessages(ctx context.Context, convID string) ([]memory.Message, error) {
	URI := strings.ReplaceAll(MessagePathURI, "{conversationId}", convID)
	var result = &MessageOutput{}
	_, err := s.dao.Operate(ctx, datly.WithOutput(result),
		datly.WithURI(URI),
		datly.WithInput(&MessageInput{ConvId: convID}))
	if err != nil {
		return nil, err
	}
	var messages []memory.Message
	for _, view := range result.Data {
		messages = append(messages, memory.Message{
			Role:    view.Role,
			Content: view.Content,
		})
	}
	return messages, err
}

func (s *Service) Retrieve(ctx context.Context, convID string, policy memory.Policy) ([]memory.Message, error) {
	messages, err := s.GetMessages(ctx, convID)
	if err != nil {
		return nil, err
	}
	if policy != nil {
		return policy.Apply(ctx, messages)
	}
	return messages, nil
}

func (s *Service) init(ctx context.Context) error {
	if err := DefineConversationComponent(ctx, s.dao); err != nil {
		return err
	}
	if err := DefineMessageComponent(ctx, s.dao); err != nil {
		return err
	}
	if _, err := post2.DefineComponent(ctx, s.dao); err != nil {
		return err
	}
	return nil
}

func New(ctx context.Context, connector *view.Connector, options ...repository.Option) (*Service, error) {
	dao, err := datly.New(ctx, options...)
	if err != nil {
		return nil, err
	}
	if err := dao.AddConnectors(ctx, connector); err != nil {
		return nil, err
	}
	ret := &Service{
		dao: dao,
	}
	err = ret.init(ctx)
	return ret, err
}
