package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/viant/agently/genai/memory"
	post2 "github.com/viant/agently/internal/dao/conversation/post"
	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
)

type Service struct {
	dao   *datly.Service
	db    *sql.DB
	dbDSN string // underlying SQLite datasource path for direct SQL fallback
}

// getDBPath returns the DataSourceName used by the underlying repository.
// When the repository connector is not an SQLite file it returns an empty
// string so that the optional direct-SQL fallback is skipped.
func (s *Service) getDBPath() string {
	if s == nil || s.db == nil {
		return ""
	}
	return s.dbDSN
}

func (s *Service) AddMessage(ctx context.Context, convID string, msg memory.Message) error {

	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	input := post2.Input{
		Conversations: []*post2.Conversation{
			{
				Id: convID,
				Message: []*post2.Message{
					{
						Id:       msg.ID,
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
	// Use the URI approach first to try to get messages
	URI := strings.ReplaceAll(MessagePathURI, "{conversationId}", convID)
	var result = &MessageOutput{}
	input := &MessageInput{
		ConvId: convID,
		Has: &MessageInputHas{
			ConvId: true,
		},
	}

	_, err := s.dao.Operate(ctx, datly.WithOutput(result),
		datly.WithURI(URI),
		datly.WithInput(input))

	// If the datly approach works, use it
	if err == nil && len(result.Data) > 0 {
		var messages []memory.Message
		for _, view := range result.Data {
			messages = append(messages, memory.Message{
				ID:      view.Id,
				Role:    view.Role,
				Content: view.Content,
			})
		}
		return messages, nil
	}

	// If datly approach fails, fall back to direct SQL
	fmt.Printf("[DEBUG_LOG] Falling back to direct SQL query for conversation %s\n", convID)

	// Get the database connection
	db, err := sql.Open("sqlite3", s.getDBPath())

	// Query the database directly
	rows, err := db.QueryContext(ctx, "SELECT id, role, content, tool_name FROM message WHERE conversation_id = ?", convID)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messages []memory.Message
	for rows.Next() {
		var id, role, content string
		var toolName *string
		if err := rows.Scan(&id, &role, &content, &toolName); err != nil {
			return nil, fmt.Errorf("failed to scan message row: %w", err)
		}

		message := memory.Message{
			Role:    role,
			Content: content,
		}
		if toolName != nil {
			message.ToolName = toolName
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating message rows: %w", err)
	}

	fmt.Printf("[DEBUG_LOG] GetMessages: Found %d messages for conversation %s\n", len(messages), convID)
	return messages, nil
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
